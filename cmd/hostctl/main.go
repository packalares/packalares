package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// nsenterPrefix is the command prefix to execute in the host's PID 1 namespace.
var nsenterPrefix = []string{"/usr/bin/nsenter", "-t", "1", "-m", "-u", "-n", "-i", "--"}

// cmdTimeout is the maximum duration for any nsenter command.
const cmdTimeout = 30 * time.Second

// SSHStatus represents the current SSH daemon state.
type SSHStatus struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

func main() {
	token := os.Getenv("HOSTCTL_TOKEN")
	if token == "" {
		log.Fatal("HOSTCTL_TOKEN environment variable is required")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ssh/status", withAuth(token, handleSSHStatus))
	mux.HandleFunc("/ssh/config", withAuth(token, handleSSHConfig))
	mux.HandleFunc("/wireguard/enable", withAuth(token, handleWGEnable))
	mux.HandleFunc("/wireguard/disable", withAuth(token, handleWGDisable))
	mux.HandleFunc("/wireguard/status", withAuth(token, handleWGStatus))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:         ":9199",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received signal %v, shutting down", sig)
		cancel()
	}()

	log.Printf("hostctl listening on %s", srv.Addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func withAuth(token string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			respondErr(w, http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}
		if strings.TrimPrefix(auth, "Bearer ") != token {
			respondErr(w, http.StatusForbidden, "invalid token")
			return
		}
		handler(w, r)
	}
}

// ---------------------------------------------------------------------------
// GET /ssh/status
// ---------------------------------------------------------------------------

func handleSSHStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := getSSHStatus()
	respondJSON(w, http.StatusOK, status)
}

// sshServiceName returns "ssh" or "sshd" depending on what the host has.
func sshServiceName() string {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, nsenterPrefix[0], append(nsenterPrefix[1:], "systemctl", "list-unit-files", "ssh.service")...).CombinedOutput()
	if strings.Contains(string(out), "ssh.service") {
		return "ssh"
	}
	return "sshd"
}

func getSSHStatus() SSHStatus {
	status := SSHStatus{Port: 22, Enabled: false}

	svcName := sshServiceName()

	// Check if ssh/sshd is active (also check ssh.socket for socket-activated SSH)
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, nsenterPrefix[0], append(nsenterPrefix[1:], "systemctl", "is-active", svcName)...).CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		status.Enabled = true
	}
	if !status.Enabled {
		ctx3, cancel3 := context.WithTimeout(context.Background(), cmdTimeout)
		defer cancel3()
		out3, _ := exec.CommandContext(ctx3, nsenterPrefix[0], append(nsenterPrefix[1:], "systemctl", "is-active", "ssh.socket")...).CombinedOutput()
		if strings.TrimSpace(string(out3)) == "active" {
			status.Enabled = true
		}
	}

	// Parse port from sshd_config
	ctx2, cancel2 := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel2()
	cfgOut, err := exec.CommandContext(ctx2, nsenterPrefix[0], append(nsenterPrefix[1:], "cat", "/etc/ssh/sshd_config")...).CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(cfgOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "Port ") || strings.HasPrefix(line, "Port\t") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if p, err := strconv.Atoi(fields[1]); err == nil {
						status.Port = p
					}
				}
			}
		}
	}

	return status
}

// ---------------------------------------------------------------------------
// POST /ssh/config
// ---------------------------------------------------------------------------

func handleSSHConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
		Port    int  `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Validate port
	if req.Port != 22 && (req.Port < 1024 || req.Port > 65535) {
		respondErr(w, http.StatusBadRequest, "port must be 22 or in range 1024-65535")
		return
	}

	log.Printf("[ssh-config] request: enabled=%v port=%d at %s", req.Enabled, req.Port, time.Now().UTC().Format(time.RFC3339))

	// Update port in sshd_config
	if err := setSSHPort(req.Port); err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to set SSH port: %v", err))
		return
	}

	// Enable or disable ssh (handle both service and socket)
	svcName := sshServiceName()
	if req.Enabled {
		// Enable and start the service
		_ = nsenterRun("systemctl", "enable", svcName)
		if err := nsenterRun("systemctl", "restart", svcName); err != nil {
			respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to restart %s: %v", svcName, err))
			return
		}
		log.Printf("[ssh-config] %s enabled on port %d at %s", svcName, req.Port, time.Now().UTC().Format(time.RFC3339))
	} else {
		// Stop and disable both service and socket (socket-activated SSH stays alive without this)
		_ = nsenterRun("systemctl", "stop", "ssh.socket")
		_ = nsenterRun("systemctl", "disable", "ssh.socket")
		_ = nsenterRun("systemctl", "stop", svcName)
		_ = nsenterRun("systemctl", "disable", svcName)
		log.Printf("[ssh-config] %s + ssh.socket disabled at %s", svcName, time.Now().UTC().Format(time.RFC3339))
	}

	// Return new status
	status := getSSHStatus()
	respondJSON(w, http.StatusOK, status)
}

func setSSHPort(port int) error {
	// Use sed via nsenter to update the Port line in sshd_config.
	// This handles both "Port NNN" and "#Port NNN" lines.
	sedExpr := fmt.Sprintf("s/^#*Port .*/Port %d/", port)
	return nsenterRun("sed", "-i", sedExpr, "/etc/ssh/sshd_config")
}

func nsenterRun(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmdArgs := append(nsenterPrefix[1:], args...)
	cmd := exec.CommandContext(ctx, nsenterPrefix[0], cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func respondJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func respondErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// nsenterWrite writes content to a file on the host via nsenter + tee.
func nsenterWrite(path, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmdArgs := append(nsenterPrefix[1:], "tee", path)
	cmd := exec.CommandContext(ctx, nsenterPrefix[0], cmdArgs...)
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write %s: %s (%w)", path, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func nsenterOutput(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmdArgs := append(nsenterPrefix[1:], args...)
	out, err := exec.CommandContext(ctx, nsenterPrefix[0], cmdArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ---------------------------------------------------------------------------
// WireGuard handlers
// ---------------------------------------------------------------------------

type WGEnableRequest struct {
	Config     string `json:"config"`
	KillSwitch bool   `json:"killSwitch"`
}

type WGStatusResponse struct {
	Active    bool   `json:"active"`
	IP        string `json:"ip"`
	PublicKey string `json:"publicKey"`
	Endpoint  string `json:"endpoint"`
	Handshake string `json:"latestHandshake"`
	Transfer  string `json:"transfer"`
	KillSwitch bool  `json:"killSwitch"`
}

// parseWGAddress extracts the Address (IP) from a WireGuard config.
func parseWGAddress(config string) string {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "address") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				addr := strings.TrimSpace(parts[1])
				// Strip CIDR suffix
				if idx := strings.Index(addr, "/"); idx > 0 {
					addr = addr[:idx]
				}
				return addr
			}
		}
	}
	return ""
}

// parseWGDNS extracts the DNS server from a WireGuard config.
func parseWGDNS(config string) string {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "dns") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				dns := strings.TrimSpace(parts[1])
				// Take first DNS if multiple
				if idx := strings.Index(dns, ","); idx > 0 {
					dns = strings.TrimSpace(dns[:idx])
				}
				return dns
			}
		}
	}
	return ""
}

// parseWGEndpoint extracts the Endpoint IP and port from a WireGuard config.
func parseWGEndpoint(config string) (ip string, port string) {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "endpoint") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				ep := strings.TrimSpace(parts[1])
				if colonIdx := strings.LastIndex(ep, ":"); colonIdx > 0 {
					return ep[:colonIdx], ep[colonIdx+1:]
				}
				return ep, "51820"
			}
		}
	}
	return "", ""
}

// rewriteAllowedIPs replaces "0.0.0.0/0" in AllowedIPs with split ranges
// that exclude K8s cluster CIDRs. This prevents WG routing table from
// capturing cluster traffic.
func rewriteAllowedIPs(config string) string {
	clusterCIDRs := detectClusterCIDRs()
	if len(clusterCIDRs) == 0 {
		return config
	}

	// Build exclusion: 0.0.0.0/0 minus cluster CIDRs
	// For simplicity, we use 0.0.0.0/1 + 128.0.0.0/1 (covers all non-10.x)
	// plus specific 10.x ranges that exclude the cluster
	// Actually simplest: just add Table = off and manage routing manually
	// Or: replace AllowedIPs = 0.0.0.0/0 with 0.0.0.0/0 but add Table = off
	// and handle routing in PostUp

	// Check if config already has Table = off
	if strings.Contains(strings.ToLower(config), "table =") {
		return config
	}

	// Add Table = off to [Interface] section to prevent wg-quick from creating routing rules
	lines := strings.Split(config, "\n")
	var result []string
	for _, line := range lines {
		result = append(result, line)
		if strings.TrimSpace(line) == "[Interface]" {
			result = append(result, "Table = off")
		}
	}
	return strings.Join(result, "\n")
}

// buildWGConfig processes the user's WG config:
// - Always adds Table = off (routing managed by PostUp)
// - When kill-switch is enabled, injects PostUp/PostDown scripts
func buildWGConfig(userConfig string, killSwitch bool) string {
	// Always rewrite to prevent wg-quick from creating routing rules
	// We manage routing ourselves to exclude cluster CIDRs
	config := rewriteAllowedIPs(userConfig)

	if !killSwitch {
		// Even without kill-switch, we need PostUp for routing + DNS
		clusterCIDRs := detectClusterCIDRs()
		clusterDNS := detectClusterDNS()
		dns := parseWGDNS(userConfig)

		upScript := fmt.Sprintf(`#!/bin/bash
# Routing: send everything through WG except cluster traffic
WG_TABLE=51820
ip route add default dev wg0 table $WG_TABLE
ip rule add not fwmark 0xca6c table $WG_TABLE
ip rule add table main suppress_prefixlength 0
# Exclude cluster CIDRs from WG table
%s
# DNS
cp -f /etc/resolv.conf /etc/resolv.conf.pre-wg 2>/dev/null
echo "nameserver %s" > /etc/resolv.conf
[ -n "%s" ] && echo "nameserver %s" >> /etc/resolv.conf
`,
			func() string {
				var s string
				for _, cidr := range clusterCIDRs {
					s += fmt.Sprintf("ip route add throw %s table $WG_TABLE 2>/dev/null\n", cidr)
				}
				return s
			}(),
			dns, clusterDNS, clusterDNS)

		downScript := `#!/bin/bash
WG_TABLE=51820
ip rule del not fwmark 0xca6c table $WG_TABLE 2>/dev/null
ip rule del table main suppress_prefixlength 0 2>/dev/null
ip route flush table $WG_TABLE 2>/dev/null
[ -f /etc/resolv.conf.pre-wg ] && mv /etc/resolv.conf.pre-wg /etc/resolv.conf
`
		_ = nsenterWrite("/usr/local/bin/wg0-up.sh", upScript)
		_ = nsenterWrite("/usr/local/bin/wg0-down.sh", downScript)
		_ = nsenterRun("chmod", "+x", "/usr/local/bin/wg0-up.sh", "/usr/local/bin/wg0-down.sh")

		// Inject PostUp/PostDown before [Peer]
		lines := strings.Split(config, "\n")
		var result []string
		injected := false
		for _, line := range lines {
			if strings.TrimSpace(line) == "[Peer]" && !injected {
				result = append(result, "PostUp = /usr/local/bin/wg0-up.sh")
				result = append(result, "PostDown = /usr/local/bin/wg0-down.sh")
				result = append(result, "")
				injected = true
			}
			result = append(result, line)
		}
		return strings.Join(result, "\n")
	}

	endpointIP, endpointPort := parseWGEndpoint(userConfig)
	dns := parseWGDNS(userConfig)
	clusterCIDRs := detectClusterCIDRs()
	clusterDNS := detectClusterDNS()

	// Build PostUp: routing + DNS + kill-switch
	upScript := fmt.Sprintf(`#!/bin/bash
WG_TABLE=51820
# Routing: send everything through WG except cluster traffic
ip route add default dev wg0 table $WG_TABLE
ip rule add not fwmark 0xca6c table $WG_TABLE
ip rule add table main suppress_prefixlength 0
# Exclude cluster CIDRs from WG table
%s
# DNS
cp -f /etc/resolv.conf /etc/resolv.conf.pre-wg 2>/dev/null
echo "nameserver %s" > /etc/resolv.conf
[ -n "%s" ] && echo "nameserver %s" >> /etc/resolv.conf
# Kill-switch
iptables -N WG_OUT 2>/dev/null; iptables -F WG_OUT
iptables -A WG_OUT -o lo -j ACCEPT
iptables -A WG_OUT -d 10.0.0.0/8 -j ACCEPT
iptables -A WG_OUT -d 172.16.0.0/12 -j ACCEPT
iptables -A WG_OUT -d 192.168.0.0/16 -j ACCEPT
iptables -A WG_OUT -o wg0 -j ACCEPT
[ -n "%s" ] && iptables -A WG_OUT -d %s -p udp --dport %s -j ACCEPT
iptables -A WG_OUT -j REJECT
iptables -C OUTPUT -j WG_OUT 2>/dev/null || iptables -I OUTPUT -j WG_OUT
# DNS redirect
if [ -n "%s" ]; then
  iptables -t nat -N WG_DNS_REDIRECT 2>/dev/null; iptables -t nat -F WG_DNS_REDIRECT
  iptables -t nat -A WG_DNS_REDIRECT -d %s -j RETURN
  iptables -t nat -A WG_DNS_REDIRECT -p udp --dport 53 -j DNAT --to-destination %s:53
  iptables -t nat -A WG_DNS_REDIRECT -p tcp --dport 53 -j DNAT --to-destination %s:53
  iptables -t nat -C OUTPUT -j WG_DNS_REDIRECT 2>/dev/null || iptables -t nat -I OUTPUT -j WG_DNS_REDIRECT
fi
`,
		func() string {
			var s string
			for _, cidr := range clusterCIDRs {
				s += fmt.Sprintf("ip route add throw %s table $WG_TABLE 2>/dev/null\n", cidr)
			}
			return s
		}(),
		dns, clusterDNS, clusterDNS,
		endpointIP, endpointIP, endpointPort,
		dns, dns, dns, dns)

	// PostDown: remove routing + DNS redirect, keep kill-switch (safe)
	downScript := `#!/bin/bash
WG_TABLE=51820
ip rule del not fwmark 0xca6c table $WG_TABLE 2>/dev/null
ip rule del table main suppress_prefixlength 0 2>/dev/null
ip route flush table $WG_TABLE 2>/dev/null
iptables -t nat -D OUTPUT -j WG_DNS_REDIRECT 2>/dev/null
iptables -t nat -F WG_DNS_REDIRECT 2>/dev/null
iptables -t nat -X WG_DNS_REDIRECT 2>/dev/null
[ -f /etc/resolv.conf.pre-wg ] && mv /etc/resolv.conf.pre-wg /etc/resolv.conf
`

	_ = nsenterWrite("/usr/local/bin/wg0-up.sh", upScript)
	_ = nsenterWrite("/usr/local/bin/wg0-down.sh", downScript)
	_ = nsenterRun("chmod", "+x", "/usr/local/bin/wg0-up.sh", "/usr/local/bin/wg0-down.sh")

	// Inject PostUp/PostDown before the first [Peer] section
	lines := strings.Split(userConfig, "\n")
	var result []string
	injected := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[Peer]" && !injected {
			result = append(result, "PostUp = /usr/local/bin/wg0-up.sh")
			result = append(result, "PostDown = /usr/local/bin/wg0-down.sh")
			result = append(result, "")
			injected = true
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func handleWGEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req WGEnableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if !strings.Contains(req.Config, "[Interface]") || !strings.Contains(req.Config, "[Peer]") {
		respondErr(w, http.StatusBadRequest, "config must contain [Interface] and [Peer] sections")
		return
	}

	ip := parseWGAddress(req.Config)
	if ip == "" {
		respondErr(w, http.StatusBadRequest, "could not parse Address from config")
		return
	}

	log.Printf("[wg] enabling WireGuard with IP %s, killSwitch=%v", ip, req.KillSwitch)

	// Ensure wireguard-tools is installed
	if _, err := nsenterOutput("which", "wg"); err != nil {
		log.Printf("[wg] wireguard-tools not found, installing...")
		if err := nsenterRun("apt-get", "install", "-y", "wireguard-tools"); err != nil {
			respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to install wireguard-tools: %v", err))
			return
		}
	}

	// Stop existing WG interface if active
	_ = nsenterRun("wg-quick", "down", "wg0")

	// Build final config (injects PostUp/PostDown if kill-switch enabled)
	finalConfig := buildWGConfig(req.Config, req.KillSwitch)

	// Write config to /etc/wireguard/wg0.conf
	if err := nsenterRun("mkdir", "-p", "/etc/wireguard"); err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to create /etc/wireguard: %v", err))
		return
	}
	if err := nsenterWrite("/etc/wireguard/wg0.conf", finalConfig); err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to write wg0.conf: %v", err))
		return
	}
	if err := nsenterRun("chmod", "600", "/etc/wireguard/wg0.conf"); err != nil {
		log.Printf("[wg] warning: chmod failed: %v", err)
	}

	// Bring up WireGuard (PostUp script handles kill-switch + DNS)
	if out, err := nsenterOutput("wg-quick", "up", "wg0"); err != nil {
		respondErr(w, http.StatusInternalServerError, fmt.Sprintf("wg-quick up failed: %s (%v)", out, err))
		return
	}

	// PostUp handles routing, DNS, and optionally kill-switch

	// Enable boot persistence
	_ = nsenterRun("systemctl", "enable", "wg-quick@wg0")

	log.Printf("[wg] WireGuard enabled on %s", ip)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"ip":      ip,
	})
}

func handleWGDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	log.Printf("[wg] disabling WireGuard")

	// Bring down WireGuard (PostDown handles routing + DNS cleanup)
	_ = nsenterRun("wg-quick", "down", "wg0")

	// Full cleanup: remove kill-switch (PostDown leaves it for safety)
	removeWGKillSwitch()

	// Ensure DNS is restored (in case PostDown failed)
	_ = nsenterRun("bash", "-c", "[ -f /etc/resolv.conf.pre-wg ] && mv /etc/resolv.conf.pre-wg /etc/resolv.conf || ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf")
	_ = nsenterRun("systemctl", "restart", "systemd-resolved")

	// Disable boot persistence
	_ = nsenterRun("systemctl", "disable", "wg-quick@wg0")

	// Remove config and helper scripts
	_ = nsenterRun("rm", "-f", "/etc/wireguard/wg0.conf", "/etc/wireguard/wg0.env")
	_ = nsenterRun("rm", "-f", "/usr/local/bin/wg0-up.sh", "/usr/local/bin/wg0-down.sh")

	log.Printf("[wg] WireGuard disabled and fully cleaned")
	respondJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func handleWGStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := WGStatusResponse{}

	// Check if wg0 interface exists
	out, err := nsenterOutput("wg", "show", "wg0")
	if err != nil {
		respondJSON(w, http.StatusOK, status)
		return
	}

	status.Active = true

	// Parse wg show output
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "public key:") {
			status.PublicKey = strings.TrimSpace(strings.TrimPrefix(line, "public key:"))
		} else if strings.HasPrefix(line, "endpoint:") {
			status.Endpoint = strings.TrimSpace(strings.TrimPrefix(line, "endpoint:"))
		} else if strings.HasPrefix(line, "latest handshake:") {
			status.Handshake = strings.TrimSpace(strings.TrimPrefix(line, "latest handshake:"))
		} else if strings.HasPrefix(line, "transfer:") {
			status.Transfer = strings.TrimSpace(strings.TrimPrefix(line, "transfer:"))
		}
	}

	// Get IP from interface
	ipOut, err := nsenterOutput("ip", "-4", "addr", "show", "wg0")
	if err == nil {
		for _, line := range strings.Split(ipOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "inet ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					addr := parts[1]
					if idx := strings.Index(addr, "/"); idx > 0 {
						addr = addr[:idx]
					}
					status.IP = addr
				}
			}
		}
	}

	// Check if kill-switch is active
	iptOut, _ := nsenterOutput("iptables", "-L", "WG_OUT", "-n")
	status.KillSwitch = strings.Contains(iptOut, "REJECT")

	respondJSON(w, http.StatusOK, status)
}

// detectClusterCIDRs finds K8s cluster CIDRs by reading the K3s service-cidr
// and cluster-cidr from the K3s config, falling back to route table inspection.
func detectClusterCIDRs() []string {
	cidrs := make(map[string]bool)

	// Method 1: Read K3s config for explicit CIDRs
	cfgOut, err := nsenterOutput("cat", "/etc/rancher/k3s/config.yaml")
	if err == nil {
		for _, line := range strings.Split(cfgOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "service-cidr:") || strings.HasPrefix(line, "cluster-cidr:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					cidr := strings.TrimSpace(strings.Trim(parts[1], "\"'"))
					if cidr != "" {
						cidrs[cidr] = true
					}
				}
			}
		}
	}

	// Method 2: Find blackhole routes (pod CIDRs) and service ClusterIP range
	routeOut, _ := nsenterOutput("ip", "route", "show", "table", "main")
	for _, line := range strings.Split(routeOut, "\n") {
		if strings.HasPrefix(line, "blackhole ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cidrs[parts[1]] = true
			}
		}
	}

	// Method 3: Check iptables for KUBE-SERVICES destination ranges
	iptOut, _ := nsenterOutput("iptables", "-t", "nat", "-L", "KUBE-SERVICES", "-n")
	for _, line := range strings.Split(iptOut, "\n") {
		if strings.Contains(line, "cluster IP") {
			fields := strings.Fields(line)
			for _, f := range fields {
				if strings.Contains(f, ".") && strings.Contains(f, "/") {
					cidrs[f] = true
				}
			}
		}
	}

	// If we found specific CIDRs, also cover the parent /16
	// to catch any services we missed
	for cidr := range cidrs {
		parts := strings.Split(strings.Split(cidr, "/")[0], ".")
		if len(parts) >= 2 {
			parent := parts[0] + "." + parts[1] + ".0.0/16"
			cidrs[parent] = true
		}
	}

	result := make([]string, 0, len(cidrs))
	for cidr := range cidrs {
		result = append(result, cidr)
	}

	if len(result) == 0 {
		log.Printf("[wg] WARNING: could not detect cluster CIDRs, K8s traffic may route through tunnel")
	}
	return result
}

// detectClusterDNS finds the cluster DNS IP from kubelet config or resolv.conf.
func detectClusterDNS() string {
	// Read from kubelet's resolv.conf (K3s sets cluster-dns in kubelet args)
	out, _ := nsenterOutput("bash", "-c", "cat /var/lib/rancher/k3s/agent/etc/resolv.conf 2>/dev/null | grep nameserver | head -1 | awk '{print $2}'")
	if out != "" && strings.Contains(out, ".") {
		return out
	}
	// Fallback: check iptables for kube-dns service IP
	iptOut, _ := nsenterOutput("bash", "-c", "iptables -t nat -L KUBE-SERVICES -n 2>/dev/null | grep 'kube-system/kube-dns:dns ' | awk '{print $5}'")
	if iptOut != "" && strings.Contains(iptOut, ".") {
		return iptOut
	}
	log.Printf("[wg] WARNING: could not detect cluster DNS")
	return ""
}

// removeWGKillSwitch fully removes all iptables kill-switch and DNS redirect rules.
// Called only on explicit disable — PostDown only removes DNS (kill-switch stays for safety).
func removeWGKillSwitch() {
	_ = nsenterRun("iptables", "-D", "OUTPUT", "-j", "WG_OUT")
	_ = nsenterRun("iptables", "-F", "WG_OUT")
	_ = nsenterRun("iptables", "-X", "WG_OUT")
	_ = nsenterRun("iptables", "-t", "nat", "-D", "OUTPUT", "-j", "WG_DNS_REDIRECT")
	_ = nsenterRun("iptables", "-t", "nat", "-F", "WG_DNS_REDIRECT")
	_ = nsenterRun("iptables", "-t", "nat", "-X", "WG_DNS_REDIRECT")
	log.Printf("[wg] kill-switch removed")
}
