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
	Mode       string `json:"mode"` // "full" (default) or "dns-only"
	KillSwitch bool   `json:"killSwitch"`
}

type WGStatusResponse struct {
	Active     bool   `json:"active"`
	IP         string `json:"ip"`
	PublicKey  string `json:"publicKey"`
	Endpoint   string `json:"endpoint"`
	Handshake  string `json:"latestHandshake"`
	Transfer   string `json:"transfer"`
	Mode       string `json:"mode"`
	KillSwitch bool   `json:"killSwitch"`
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

// ensureInterfaceField inserts `key = value` into the [Interface] section of a
// WG config if no line for that key is already present there. Used to pin
// ListenPort so the kernel doesn't pick a fresh ephemeral port on every restart
// (which would invalidate the home router's NAT mapping and leave the relay's
// stored peer endpoint pointing at a dead port until it re-learns).
func ensureInterfaceField(config, key, value string) string {
	inInterface := false
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[Interface]" {
			inInterface = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inInterface = false
			continue
		}
		if !inInterface {
			continue
		}
		lower := strings.ToLower(trimmed)
		k := strings.ToLower(key)
		if strings.HasPrefix(lower, k+" ") || strings.HasPrefix(lower, k+"=") {
			return config // key already present, respect user setting
		}
	}
	lines := strings.Split(config, "\n")
	var result []string
	injected := false
	for _, line := range lines {
		result = append(result, line)
		if !injected && strings.TrimSpace(line) == "[Interface]" {
			result = append(result, fmt.Sprintf("%s = %s", key, value))
			injected = true
		}
	}
	return strings.Join(result, "\n")
}

// rewriteAllowedIPs replaces every AllowedIPs line in [Peer] sections with newAllowed.
// Used to override AllowedIPs when mode requires it (e.g. dns-only mode forces peer-mesh only).
func rewriteAllowedIPs(config, newAllowed string) string {
	lines := strings.Split(config, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "AllowedIPs") {
			lines[i] = "AllowedIPs = " + newAllowed
		}
	}
	return strings.Join(lines, "\n")
}

// buildWGConfig injects PostUp/PostDown into the user's WG config.
// wg-quick handles routing normally (creates its own table + fwmark + nft rules).
// PostUp adds throw routes for cluster CIDRs + optional DNS + optional kill-switch.
//
// Modes:
//   - "" / "full": user's AllowedIPs honored.
//     killSwitch=true → iptables WG_OUT chain blocks all non-LAN/non-WG egress.
//   - "dns-only": AllowedIPs forced to 10.8.0.0/24 (peer mesh) — internet stays direct.
//     DNS pinned via iptables NAT redirect (catches apps that bypass /etc/resolv.conf).
//     killSwitch=true → strict resolv.conf (only 10.8.0.1, no public fallback).
//     killSwitch=false → resolv.conf has 10.8.0.1 + 1.1.1.1 + 8.8.8.8 fallback.
func buildWGConfig(userConfig, mode string, killSwitch bool) string {
	// Pin ListenPort so NAT mappings survive reboots. User can override by
	// setting their own ListenPort in [Interface].
	userConfig = ensureInterfaceField(userConfig, "ListenPort", "51820")

	dns := parseWGDNS(userConfig)

	// dns-only mode: only peer mesh through WG. killSwitch is reinterpreted here
	// to control DNS strictness rather than gating an iptables egress chain.
	if mode == "dns-only" {
		userConfig = rewriteAllowedIPs(userConfig, "10.8.0.0/24")
	}

	clusterCIDRs := detectClusterCIDRs()

	// Throw routes so cluster traffic bypasses the WG tunnel.
	var throwRoutes string
	for _, cidr := range clusterCIDRs {
		throwRoutes += fmt.Sprintf("ip route add throw %s table $WG_TABLE 2>/dev/null\n", cidr)
	}

	// DNS resolv.conf rewrite.
	// We include cluster DNS (10.233.0.10) so hostNetwork pods (e.g. the proxy
	// nginx) can resolve *.cluster.local names — without it, the tunnel resolver
	// (e.g. AdGuard) returns NXDOMAIN for cluster names and auth subrequests
	// die with 502, breaking the admin panel entirely.
	// The historic "NEVER list cluster DNS" caveat was about CoreDNS-loop risk:
	// if CoreDNS reads host resolv.conf as its upstream and finds 10.233.0.10
	// listed, it forwards to itself and the loop plugin trips. The k3s/CoreDNS
	// in this deployment uses an explicit upstream in the Corefile (not host
	// resolv.conf) so this is safe. If you fork to a deployment whose CoreDNS
	// reads /etc/resolv.conf, drop the cluster-DNS line from the templates below.
	// Break-the-symlink dance: on systemd-resolved hosts, /etc/resolv.conf is a
	// symlink to /run/systemd/resolve/stub-resolv.conf. Writing through the
	// symlink (cat >) modifies the stub file, which systemd-resolved owns and
	// regenerates on its own — losing our changes within minutes. We snapshot
	// the original (preserving symlink type), remove it, and write a real file.
	// PostDown reverses this.
	snapshotResolv := `# Snapshot original /etc/resolv.conf (preserve symlink target if any)
if [ -L /etc/resolv.conf ]; then
  readlink /etc/resolv.conf > /etc/resolv.conf.pre-wg-link 2>/dev/null
else
  cp -f /etc/resolv.conf /etc/resolv.conf.pre-wg 2>/dev/null
fi
rm -f /etc/resolv.conf
`
	dnsBlock := ""
	if dns != "" {
		switch {
		case mode == "dns-only" && killSwitch:
			// Strict: only the WG-side resolver + cluster DNS. AdGuard down → no external DNS.
			dnsBlock = fmt.Sprintf(`# DNS: strict (tunnel resolver + cluster DNS only — killswitch ON in dns-only mode)
%scat > /etc/resolv.conf <<'EOF'
nameserver %s
nameserver 10.233.0.10
EOF
`, snapshotResolv, dns)
		default:
			// Soft: tunnel DNS first, then public fallback, with cluster DNS for *.cluster.local.
			dnsBlock = fmt.Sprintf(`# DNS: tunnel DNS + public fallback + cluster DNS for *.cluster.local
%scat > /etc/resolv.conf <<'EOF'
nameserver %s
nameserver 10.233.0.10
nameserver 1.1.1.1
nameserver 8.8.8.8
EOF
`, snapshotResolv, dns)
		}
	}

	// DNS NAT pin — dns-only mode only.
	// Forces every port-53 packet through the WG-side resolver regardless of
	// /etc/resolv.conf, so apps that bake their own resolver (browsers with DoH off,
	// stub libc, dnsmasq forwarders) still hit the filtered DNS.
	//
	// Exemption: traffic destined for the cluster service CIDR (10.233.0.0/16)
	// skips the redirect so kube-proxy's KUBE-SERVICES DNAT can translate the
	// service VIP (e.g. 10.233.0.10:53) to the actual CoreDNS pod. Without this
	// the host (and any hostNetwork pod, like the proxy nginx) cannot resolve
	// *.cluster.local names — every query gets hijacked to AdGuard, which
	// returns NXDOMAIN, and the admin panel auth subrequest dies with 502.
	dnsNATBlock := ""
	if mode == "dns-only" {
		dnsTarget := dns
		if dnsTarget == "" {
			dnsTarget = "10.8.0.1"
		}
		dnsNATBlock = fmt.Sprintf(`# DNS NAT pin: redirect all :53 to the WG-side filtered resolver
# (cluster CIDR 10.233.0.0/16 exempted so kube-proxy can DNAT the cluster DNS VIP)
iptables -t nat -N WG_DNS_REDIRECT 2>/dev/null; iptables -t nat -F WG_DNS_REDIRECT
iptables -t nat -A WG_DNS_REDIRECT -p udp --dport 53 -j DNAT --to-destination %s:53
iptables -t nat -A WG_DNS_REDIRECT -p tcp --dport 53 -j DNAT --to-destination %s:53
iptables -t nat -C OUTPUT -p udp --dport 53 ! -d 10.233.0.0/16 -j WG_DNS_REDIRECT 2>/dev/null || iptables -t nat -I OUTPUT 1 -p udp --dport 53 ! -d 10.233.0.0/16 -j WG_DNS_REDIRECT
iptables -t nat -C OUTPUT -p tcp --dport 53 ! -d 10.233.0.0/16 -j WG_DNS_REDIRECT 2>/dev/null || iptables -t nat -I OUTPUT 1 -p tcp --dport 53 ! -d 10.233.0.0/16 -j WG_DNS_REDIRECT
`, dnsTarget, dnsTarget)
	}

	// Kill-switch iptables egress chain — full mode only.
	// In dns-only mode, killSwitch controls resolv.conf strictness instead (above).
	killSwitchBlock := ""
	if killSwitch && mode != "dns-only" {
		endpointIP, endpointPort := parseWGEndpoint(userConfig)
		// Build the endpoint-accept line conditionally in Go. Doing it in bash
		// via `[ -n "%s" ] && ...` after fmt.Sprintf substitutes the literal
		// makes the test always-true — the conditional was effectively dead.
		endpointAccept := ""
		if endpointIP != "" {
			endpointAccept = fmt.Sprintf("iptables -A WG_OUT -d %s -p udp --dport %s -j ACCEPT\n",
				endpointIP, endpointPort)
		}
		killSwitchBlock = fmt.Sprintf(`# Kill-switch (iptables)
iptables -N WG_OUT 2>/dev/null; iptables -F WG_OUT
iptables -A WG_OUT -o lo -j ACCEPT
iptables -A WG_OUT -d 10.0.0.0/8 -j ACCEPT
iptables -A WG_OUT -d 172.16.0.0/12 -j ACCEPT
iptables -A WG_OUT -d 192.168.0.0/16 -j ACCEPT
iptables -A WG_OUT -o wg0 -j ACCEPT
%siptables -A WG_OUT -j REJECT
iptables -C OUTPUT -j WG_OUT 2>/dev/null || iptables -I OUTPUT -j WG_OUT
`, endpointAccept)
	}

	// Mode + killswitch markers so handleWGStatus can report state without
	// having to guess from iptables (which only reflects full-mode killswitch).
	modeValue := mode
	if modeValue == "" {
		modeValue = "full"
	}
	killSwitchValue := "0"
	if killSwitch {
		killSwitchValue = "1"
	}
	modeMarker := fmt.Sprintf("echo %s > /etc/wireguard/wg0.mode\necho %s > /etc/wireguard/wg0.killswitch\n", modeValue, killSwitchValue)

	upScript := fmt.Sprintf(`#!/bin/bash
# Get wg-quick's routing table from the fwmark it set
WG_TABLE=$(wg show wg0 fwmark 2>/dev/null)
[ -z "$WG_TABLE" ] && WG_TABLE=51820
%s# Add throw routes so cluster traffic bypasses WG tunnel
%s
%s%s%s`, modeMarker, throwRoutes, dnsBlock, dnsNATBlock, killSwitchBlock)

	// PostDown restores DNS only; iptables rules (if any) stay for safety on crash.
	// Full cleanup happens in handleWGDisable → removeWGKillSwitch.
	//
	// Resolv.conf restore handles both cases: original was a symlink (-link snapshot)
	// or a real file (.pre-wg snapshot). PostUp replaced /etc/resolv.conf with a real
	// file, so we always rm it first before restoring.
	downScript := `#!/bin/bash
rm -f /etc/resolv.conf
if [ -f /etc/resolv.conf.pre-wg-link ]; then
  ln -sf "$(cat /etc/resolv.conf.pre-wg-link)" /etc/resolv.conf
  rm -f /etc/resolv.conf.pre-wg-link
elif [ -f /etc/resolv.conf.pre-wg ]; then
  mv /etc/resolv.conf.pre-wg /etc/resolv.conf
else
  # Safety net: re-link to systemd-resolved stub if neither snapshot exists.
  ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
fi
`

	_ = nsenterWrite("/usr/local/bin/wg0-up.sh", upScript)
	_ = nsenterWrite("/usr/local/bin/wg0-down.sh", downScript)
	_ = nsenterRun("chmod", "+x", "/usr/local/bin/wg0-up.sh", "/usr/local/bin/wg0-down.sh")

	// Inject PostUp/PostDown before the first [Peer] section.
	lines := strings.Split(userConfig, "\n")
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

	log.Printf("[wg] enabling WireGuard with IP %s, mode=%q, killSwitch=%v", ip, req.Mode, req.KillSwitch)

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
	finalConfig := buildWGConfig(req.Config, req.Mode, req.KillSwitch)

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
	_ = nsenterRun("rm", "-f", "/etc/wireguard/wg0.conf", "/etc/wireguard/wg0.env", "/etc/wireguard/wg0.mode", "/etc/wireguard/wg0.killswitch")
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

	// Read mode marker (written by PostUp). Defaults to "full" for backwards compat.
	if modeOut, err := nsenterOutput("cat", "/etc/wireguard/wg0.mode"); err == nil {
		status.Mode = strings.TrimSpace(modeOut)
	}
	if status.Mode == "" {
		status.Mode = "full"
	}

	// Kill-switch: prefer the marker file (covers both modes' semantics).
	// Fall back to iptables REJECT detection for older WG sessions that
	// predate the marker.
	if ksOut, err := nsenterOutput("cat", "/etc/wireguard/wg0.killswitch"); err == nil {
		status.KillSwitch = strings.TrimSpace(ksOut) == "1"
	} else {
		iptOut, _ := nsenterOutput("iptables", "-L", "WG_OUT", "-n")
		status.KillSwitch = strings.Contains(iptOut, "REJECT")
	}

	respondJSON(w, http.StatusOK, status)
}

// detectClusterCIDRs finds K8s cluster CIDRs by reading the K3s service-cidr
// and cluster-cidr from the K3s config, falling back to route table inspection.
// isValidCIDR checks if a string looks like a valid CIDR (x.x.x.x/N, not 0.0.0.0/0).
func isValidCIDR(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") || !strings.Contains(s, ".") {
		return false
	}
	// Reject 0.0.0.0/0 and similar catch-all
	ip := strings.Split(s, "/")[0]
	if ip == "0.0.0.0" || ip == "" {
		return false
	}
	// Must start with a digit
	if len(ip) == 0 || ip[0] < '0' || ip[0] > '9' {
		return false
	}
	return true
}

func detectClusterCIDRs() []string {
	seen := make(map[string]bool)

	// Method 1: Read K3s config for service-cidr and cluster-cidr
	cfgOut, err := nsenterOutput("cat", "/etc/rancher/k3s/config.yaml")
	if err == nil {
		for _, line := range strings.Split(cfgOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "service-cidr:") || strings.HasPrefix(line, "cluster-cidr:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					cidr := strings.TrimSpace(parts[1])
					cidr = strings.Trim(cidr, "\"' ")
					if isValidCIDR(cidr) {
						seen[cidr] = true
					}
				}
			}
		}
	}

	// Method 2: Find blackhole routes (pod CIDRs assigned to this node)
	routeOut, _ := nsenterOutput("ip", "route", "show", "table", "main")
	for _, line := range strings.Split(routeOut, "\n") {
		if strings.HasPrefix(line, "blackhole ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && isValidCIDR(parts[1]) {
				seen[parts[1]] = true
			}
		}
	}

	// If we found specific CIDRs, also add the parent /16 to cover the full range
	for cidr := range seen {
		ip := strings.Split(cidr, "/")[0]
		octets := strings.Split(ip, ".")
		if len(octets) >= 2 {
			parent := octets[0] + "." + octets[1] + ".0.0/16"
			if isValidCIDR(parent) {
				seen[parent] = true
			}
		}
	}

	result := make([]string, 0, len(seen))
	for cidr := range seen {
		result = append(result, cidr)
	}

	if len(result) == 0 {
		log.Printf("[wg] WARNING: could not detect cluster CIDRs")
	} else {
		log.Printf("[wg] detected cluster CIDRs: %v", result)
	}
	return result
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
