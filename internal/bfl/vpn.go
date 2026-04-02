package bfl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// VPN type constants
const (
	VPNNone      = "none"
	VPNTailscale = "tailscale"
	VPNWireGuard = "wireguard"

	vpnConfigMapName = "packalares-network"
)

// VPNStatusResponse is the unified VPN status response.
type VPNStatusResponse struct {
	Type      string                   `json:"type"`
	IP        string                   `json:"ip"`
	Connected bool                     `json:"connected"`
	Tailscale *TailscaleStatusResponse `json:"tailscale,omitempty"`
	WireGuard *WGStatusDetail          `json:"wireguard,omitempty"`
}

// WGStatusDetail is the WireGuard-specific status from hostctl.
type WGStatusDetail struct {
	Active     bool   `json:"active"`
	IP         string `json:"ip"`
	PublicKey  string `json:"publicKey"`
	Endpoint   string `json:"endpoint"`
	Handshake  string `json:"latestHandshake"`
	Transfer   string `json:"transfer"`
	KillSwitch bool   `json:"killSwitch"`
}

// WGEnableRequest is the request body for enabling WireGuard.
type WGEnableRequest struct {
	Config     string `json:"config"`
	KillSwitch bool   `json:"killSwitch"`
}

// ---------------------------------------------------------------------------
// VPN state management (ConfigMap-backed)
// ---------------------------------------------------------------------------

func (s *Server) getVPNState(ctx context.Context) (vpnType, vpnIP string) {
	ns := config.FrameworkNamespace()
	cm, err := s.K8s.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, vpnConfigMapName, metav1.GetOptions{})
	if err != nil {
		return VPNNone, ""
	}
	t := cm.Data["vpn_type"]
	if t == "" {
		t = VPNNone
	}
	return t, cm.Data["vpn_ip"]
}

func (s *Server) setVPNState(ctx context.Context, vpnType, vpnIP string) error {
	ns := config.FrameworkNamespace()
	cm, err := s.K8s.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, vpnConfigMapName, metav1.GetOptions{})
	if err != nil {
		// Create if not exists
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: vpnConfigMapName, Namespace: ns},
			Data:       map[string]string{"vpn_type": vpnType, "vpn_ip": vpnIP},
		}
		_, err = s.K8s.Clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data["vpn_type"] = vpnType
	cm.Data["vpn_ip"] = vpnIP
	_, err = s.K8s.Clientset.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// getActiveVPNIP returns the active VPN IP (Tailscale or WireGuard) or "".
// For Tailscale, it checks the live pod first, falling back to stored state.
func (s *Server) getActiveVPNIP(ctx context.Context) string {
	vpnType, vpnIP := s.getVPNState(ctx)
	switch vpnType {
	case VPNTailscale:
		// Try live Tailscale IP first
		if live := s.getTailscaleIP(ctx); live != "" {
			return live
		}
		return vpnIP
	case VPNWireGuard:
		return vpnIP
	default:
		// Legacy: check if Tailscale is running without VPN state
		if ip := s.getTailscaleIP(ctx); ip != "" {
			return ip
		}
		return ""
	}
}

// ---------------------------------------------------------------------------
// GET /api/settings/vpn/status
// ---------------------------------------------------------------------------

func (s *Server) handleVPNStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()
	vpnType, vpnIP := s.getVPNState(ctx)

	resp := VPNStatusResponse{Type: vpnType, IP: vpnIP}

	switch vpnType {
	case VPNTailscale:
		ts := s.getTailscaleStatusJSON(ctx)
		if ts != nil {
			resp.Connected = true
			resp.IP = ts.IP
			resp.Tailscale = ts
		}
	case VPNWireGuard:
		wg := s.getWireGuardStatus(ctx)
		if wg != nil && wg.Active {
			resp.Connected = true
			resp.IP = wg.IP
			resp.WireGuard = wg
		}
	default:
		// Check if Tailscale is running without state (legacy)
		if ip := s.getTailscaleIP(ctx); ip != "" {
			resp.Type = VPNTailscale
			resp.IP = ip
			resp.Connected = true
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// POST /api/settings/vpn/wireguard/enable
// ---------------------------------------------------------------------------

func (s *Server) handleWireGuardEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	var req WGEnableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Mutual exclusion: disable Tailscale first if active
	vpnType, _ := s.getVPNState(ctx)
	if vpnType == VPNTailscale {
		klog.Infof("VPN switch: disabling Tailscale before enabling WireGuard")
		s.disableTailscale(ctx)
	}

	// Call hostctl to enable WireGuard
	body, _ := json.Marshal(req)
	resp, err := s.hostctlRequest(ctx, http.MethodPost, "/wireguard/enable", body)
	if err != nil {
		respondError(w, fmt.Sprintf("hostctl wireguard enable: %v", err))
		return
	}

	var result struct {
		Success bool   `json:"success"`
		IP      string `json:"ip"`
		Error   string `json:"error"`
	}
	json.Unmarshal(resp, &result)
	if !result.Success && result.Error != "" {
		respondError(w, result.Error)
		return
	}

	// Store VPN state and regenerate certs
	go s.afterVPNEnabled(context.Background(), VPNWireGuard, result.IP)

	respondJSON(w, http.StatusOK, map[string]interface{}{"status": "OK", "ip": result.IP})
}

// ---------------------------------------------------------------------------
// POST /api/settings/vpn/wireguard/disable
// ---------------------------------------------------------------------------

func (s *Server) handleWireGuardDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	_, err := s.hostctlRequest(ctx, http.MethodPost, "/wireguard/disable", nil)
	if err != nil {
		klog.Warningf("hostctl wireguard disable: %v", err)
	}

	go s.afterVPNDisabled(context.Background())

	respondJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

// ---------------------------------------------------------------------------
// POST /api/settings/vpn/tailscale/disable
// ---------------------------------------------------------------------------

func (s *Server) handleTailscaleDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed")
		return
	}
	ctx := r.Context()

	s.disableTailscale(ctx)
	go s.afterVPNDisabled(context.Background())

	respondJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

// ---------------------------------------------------------------------------
// VPN lifecycle helpers
// ---------------------------------------------------------------------------

// afterVPNEnabled stores VPN state and regenerates TLS cert + nginx.
func (s *Server) afterVPNEnabled(ctx context.Context, vpnType, vpnIP string) {
	if err := s.setVPNState(ctx, vpnType, vpnIP); err != nil {
		klog.Errorf("set VPN state: %v", err)
	}

	serverIP := s.getNodeIP(ctx)
	zone := ""
	user, err := s.K8s.GetUser(ctx, "")
	if err == nil {
		zone = GetUserZone(user)
	}
	if zone == "" {
		klog.Warning("zone not set, skipping post-VPN cert regen")
		return
	}
	customDomain := s.getCustomDomain(ctx)

	if err := s.regenerateTLSCert(ctx, serverIP, vpnIP, zone, customDomain); err != nil {
		klog.Errorf("post-VPN cert regen: %v", err)
		return
	}
	if err := s.regenerateNginxConfig(ctx); err != nil {
		klog.Errorf("post-VPN nginx regen: %v", err)
	}
	s.restartProxy(ctx)
	klog.Infof("cert and proxy updated after VPN enable (type=%s, ip=%s)", vpnType, vpnIP)
}

// afterVPNDisabled clears VPN state and regenerates TLS cert + nginx without VPN IP.
func (s *Server) afterVPNDisabled(ctx context.Context) {
	if err := s.setVPNState(ctx, VPNNone, ""); err != nil {
		klog.Errorf("clear VPN state: %v", err)
	}

	serverIP := s.getNodeIP(ctx)
	zone := ""
	user, err := s.K8s.GetUser(ctx, "")
	if err == nil {
		zone = GetUserZone(user)
	}
	if zone == "" {
		klog.Warning("zone not set, skipping post-VPN-disable cert regen")
		return
	}
	customDomain := s.getCustomDomain(ctx)

	if err := s.regenerateTLSCert(ctx, serverIP, "", zone, customDomain); err != nil {
		klog.Errorf("post-VPN-disable cert regen: %v", err)
		return
	}
	if err := s.regenerateNginxConfig(ctx); err != nil {
		klog.Errorf("post-VPN-disable nginx regen: %v", err)
	}
	s.restartProxy(ctx)
	klog.Infof("cert and proxy updated after VPN disable")
}

// disableTailscale deletes the Tailscale deployment and waits for it to terminate.
func (s *Server) disableTailscale(ctx context.Context) {
	ns := config.FrameworkNamespace()
	err := s.K8s.Clientset.AppsV1().Deployments(ns).Delete(ctx, "tailscale", metav1.DeleteOptions{})
	if err != nil {
		klog.Warningf("delete tailscale deployment: %v", err)
		return
	}
	// Wait for pod to terminate (up to 15s)
	for i := 0; i < 15; i++ {
		pods, _ := s.K8s.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
			LabelSelector: "app=tailscale",
		})
		if pods == nil || len(pods.Items) == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}
	klog.Infof("Tailscale deployment deleted")
}

// getWireGuardStatus calls hostctl to get WireGuard status.
func (s *Server) getWireGuardStatus(ctx context.Context) *WGStatusDetail {
	resp, err := s.hostctlRequest(ctx, http.MethodGet, "/wireguard/status", nil)
	if err != nil {
		return nil
	}
	var status WGStatusDetail
	if err := json.Unmarshal(resp, &status); err != nil {
		return nil
	}
	return &status
}

// getTailscaleStatusJSON returns a structured Tailscale status for the VPN status response.
func (s *Server) getTailscaleStatusJSON(ctx context.Context) *TailscaleStatusResponse {
	ts := s.getTailscaleStatus(ctx)
	if ts == nil {
		return nil
	}
	return ts
}

// hostctlRequest makes an authenticated HTTP request to the hostctl service.
func (s *Server) hostctlRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	url := "http://hostctl-svc." + config.FrameworkNamespace() + ".svc.cluster.local:9199" + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.hostctlToken())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return data, fmt.Errorf("hostctl returned %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// hostctlToken returns the hostctl bearer token.
func (s *Server) hostctlToken() string {
	ctx := context.Background()
	ns := config.FrameworkNamespace()
	secret, err := s.K8s.Clientset.CoreV1().Secrets(ns).Get(ctx, "hostctl-secret", metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return string(secret.Data["token"])
}
