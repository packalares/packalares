package bfl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// ---------------------------------------------------------------------------
// Tailscale status JSON parsing (from `tailscale status --json`)
// ---------------------------------------------------------------------------

// tsStatusJSON mirrors the relevant fields from `tailscale status --json`.
type tsStatusJSON struct {
	Self struct {
		TailscaleIPs []string `json:"TailscaleIPs"`
		HostName     string   `json:"HostName"`
		Online       bool     `json:"Online"`
	} `json:"Self"`
	Peer map[string]struct {
		HostName     string   `json:"HostName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
		Online       bool     `json:"Online"`
		LastSeen     string   `json:"LastSeen"`
	} `json:"Peer"`
	CurrentTailnet *struct {
		Name string `json:"Name"`
	} `json:"CurrentTailnet"`
	BackendState string `json:"BackendState"`
}

// parseTailscaleStatus decodes the JSON output of `tailscale status --json`.
func parseTailscaleStatus(data []byte) *TailscaleStatusResponse {
	var ts tsStatusJSON
	if err := json.Unmarshal(data, &ts); err != nil {
		klog.V(2).Infof("parse tailscale status: %v", err)
		return nil
	}

	ip := ""
	if len(ts.Self.TailscaleIPs) > 0 {
		ip = ts.Self.TailscaleIPs[0]
	}

	connected := ts.BackendState == "Running"

	var peers []TailscalePeer
	for _, p := range ts.Peer {
		peerIP := ""
		if len(p.TailscaleIPs) > 0 {
			peerIP = p.TailscaleIPs[0]
		}
		peers = append(peers, TailscalePeer{
			Name:     p.HostName,
			IP:       peerIP,
			Online:   p.Online,
			LastSeen: p.LastSeen,
		})
	}
	if peers == nil {
		peers = []TailscalePeer{}
	}

	return &TailscaleStatusResponse{
		Enabled:      true,
		Connected:    connected,
		IP:           ip,
		Hostname:     ts.Self.HostName,
		Peers:        peers,
		AcceptRoutes: false, // not directly available in status JSON; default false
	}
}

// ---------------------------------------------------------------------------
// handleTailscaleStatus — GET /bfl/backend/v1/tailscale/status
// ---------------------------------------------------------------------------

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}

	status := s.getTailscaleStatus(r.Context())
	if status == nil {
		respondJSON(w, http.StatusOK, &TailscaleStatusResponse{
			Enabled:   false,
			Connected: false,
			Peers:     []TailscalePeer{},
		})
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// ---------------------------------------------------------------------------
// handleNetworkInfo — GET /bfl/backend/v1/network/info
// ---------------------------------------------------------------------------

func (s *Server) handleNetworkInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	serverIP := s.getNodeIP(ctx)
	tailscaleIP := s.getTailscaleIP(ctx)
	customDomain := s.getCustomDomain(ctx)

	zone := ""
	user, err := s.K8s.GetUser(ctx, "")
	if err == nil {
		zone = GetUserZone(user)
	}

	certSANs, certExpiry := s.getCertInfo(ctx)
	if certSANs == nil {
		certSANs = []string{}
	}

	respondJSON(w, http.StatusOK, &NetworkInfoResponse{
		ServerIP:     serverIP,
		TailscaleIP:  tailscaleIP,
		Zone:         zone,
		CustomDomain: customDomain,
		CertSANs:     certSANs,
		CertExpiry:   certExpiry,
	})
}

// ---------------------------------------------------------------------------
// handleCustomDomain — POST /bfl/backend/v1/network/domain
// ---------------------------------------------------------------------------

func (s *Server) handleCustomDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	var req CustomDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Sanitize domain — trim whitespace and trailing dots
	req.Domain = strings.TrimSpace(req.Domain)
	req.Domain = strings.TrimSuffix(req.Domain, ".")

	// Store the domain
	if err := s.setCustomDomain(ctx, req.Domain); err != nil {
		respondError(w, fmt.Sprintf("save custom domain: %v", err))
		return
	}

	// Gather current network state for cert regeneration
	serverIP := s.getNodeIP(ctx)
	tailscaleIP := s.getTailscaleIP(ctx)

	zone := ""
	user, err := s.K8s.GetUser(ctx, "")
	if err == nil {
		zone = GetUserZone(user)
	}
	if zone == "" {
		respondError(w, "zone not configured")
		return
	}

	// Regenerate TLS cert
	if err := s.regenerateTLSCert(ctx, serverIP, tailscaleIP, zone, req.Domain); err != nil {
		respondError(w, fmt.Sprintf("regenerate TLS cert: %v", err))
		return
	}

	// Update nginx server_name in the IP block
	names := buildServerNames(serverIP, tailscaleIP, req.Domain)
	if err := s.updateNginxServerName(ctx, names); err != nil {
		klog.Warningf("update nginx server_name: %v", err)
		// Non-fatal — cert is already updated
	}

	// Restart proxy to pick up new cert and config
	go s.restartProxy(context.Background())

	klog.Infof("custom domain set to %q, cert regenerated", req.Domain)
	respondSuccess(w)
}

// buildServerNames returns the list of server_name values for the IP server block.
func buildServerNames(serverIP, tailscaleIP, customDomain string) []string {
	var names []string
	if serverIP != "" {
		names = append(names, serverIP)
	}
	if tailscaleIP != "" {
		names = append(names, tailscaleIP)
	}
	if customDomain != "" {
		names = append(names, customDomain, "*."+customDomain)
	}
	if len(names) == 0 {
		names = append(names, "_")
	}
	return names
}

// ---------------------------------------------------------------------------
// Tailscale enable/disable hooks for cert regeneration
// ---------------------------------------------------------------------------

// afterTailscaleEnabled should be called after the Tailscale deployment is created
// or restarted. It waits briefly for the Tailscale IP, then regenerates the cert
// and updates nginx.
func (s *Server) afterTailscaleEnabled(ctx context.Context) {
	// Poll for Tailscale IP (up to 30 seconds)
	var tsIP string
	for i := 0; i < 15; i++ {
		tsIP = s.getTailscaleIP(ctx)
		if tsIP != "" {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}

	if tsIP == "" {
		klog.Warning("Tailscale pod IP not available after 30s, skipping cert regen")
		return
	}

	serverIP := s.getNodeIP(ctx)
	zone := ""
	user, err := s.K8s.GetUser(ctx, "")
	if err == nil {
		zone = GetUserZone(user)
	}
	if zone == "" {
		klog.Warning("zone not set, skipping post-tailscale cert regen")
		return
	}

	customDomain := s.getCustomDomain(ctx)

	if err := s.regenerateTLSCert(ctx, serverIP, tsIP, zone, customDomain); err != nil {
		klog.Errorf("post-tailscale cert regen: %v", err)
		return
	}

	names := buildServerNames(serverIP, tsIP, customDomain)
	if err := s.updateNginxServerName(ctx, names); err != nil {
		klog.Errorf("post-tailscale nginx update: %v", err)
	}

	s.restartProxy(ctx)
	klog.Infof("cert and proxy updated after Tailscale enable (tsIP=%s)", tsIP)
}

// afterTailscaleDisabled should be called after the Tailscale deployment is removed.
// It regenerates the cert without the Tailscale IP and updates nginx.
func (s *Server) afterTailscaleDisabled(ctx context.Context) {
	serverIP := s.getNodeIP(ctx)
	zone := ""
	user, err := s.K8s.GetUser(ctx, "")
	if err == nil {
		zone = GetUserZone(user)
	}
	if zone == "" {
		klog.Warning("zone not set, skipping post-tailscale-disable cert regen")
		return
	}

	customDomain := s.getCustomDomain(ctx)

	if err := s.regenerateTLSCert(ctx, serverIP, "", zone, customDomain); err != nil {
		klog.Errorf("post-tailscale-disable cert regen: %v", err)
		return
	}

	names := buildServerNames(serverIP, "", customDomain)
	if err := s.updateNginxServerName(ctx, names); err != nil {
		klog.Errorf("post-tailscale-disable nginx update: %v", err)
	}

	s.restartProxy(ctx)
	klog.Infof("cert and proxy updated after Tailscale disable")
}

