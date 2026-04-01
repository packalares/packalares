package phases

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// configureSystem applies hostname, timezone, and static IP if requested.
func configureSystem(opts *InstallOptions, w io.Writer) error {
	if opts.Hostname != "" {
		current, _ := os.Hostname()
		if opts.Hostname != current {
			fmt.Fprintf(w, "  Setting hostname to %s ...\n", opts.Hostname)
			if err := exec.Command("hostnamectl", "set-hostname", opts.Hostname).Run(); err != nil {
				return fmt.Errorf("set hostname: %w", err)
			}
		} else {
			fmt.Fprintf(w, "  Hostname already set to %s\n", opts.Hostname)
		}
	}

	if opts.Timezone != "" {
		fmt.Fprintf(w, "  Setting timezone to %s ...\n", opts.Timezone)
		if err := exec.Command("timedatectl", "set-timezone", opts.Timezone).Run(); err != nil {
			return fmt.Errorf("set timezone: %w", err)
		}
	}

	if opts.StaticIP {
		if err := configureStaticIP(opts, w); err != nil {
			fmt.Fprintf(w, "  Warning: static IP config failed: %v\n", err)
		}
	}

	return nil
}

// configureStaticIP converts the current DHCP config to static on the active interface.
func configureStaticIP(opts *InstallOptions, w io.Writer) error {
	iface, ip, gateway, err := detectNetworkInfo()
	if err != nil {
		return err
	}

	dns := detectDNS()
	fmt.Fprintf(w, "  Configuring static IP: %s on %s (gateway: %s)\n", ip, iface, gateway)

	// Update the existing packalares netplan file
	config := buildNetplanConfig(opts, iface, ip, gateway, dns)

	if err := os.WriteFile("/etc/netplan/99-packalares.yaml", []byte(config), 0600); err != nil {
		return fmt.Errorf("write netplan: %w", err)
	}

	if out, err := exec.Command("netplan", "apply").CombinedOutput(); err != nil {
		return fmt.Errorf("netplan apply: %s %w", string(out), err)
	}

	return nil
}

// detectNetworkInfo returns the default interface, IP, and gateway.
func detectNetworkInfo() (iface, ip, gateway string, err error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", "", fmt.Errorf("ip route: %w", err)
	}
	parts := strings.Fields(string(out))
	for i, p := range parts {
		if p == "via" && i+1 < len(parts) {
			gateway = parts[i+1]
		}
		if p == "dev" && i+1 < len(parts) {
			iface = parts[i+1]
		}
	}
	if iface == "" {
		return "", "", "", fmt.Errorf("no default interface found")
	}

	out, err = exec.Command("ip", "-4", "addr", "show", iface).Output()
	if err != nil {
		return "", "", "", fmt.Errorf("ip addr: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ip = strings.Split(fields[1], "/")[0]
				break
			}
		}
	}
	if ip == "" {
		return "", "", "", fmt.Errorf("no IP found on %s", iface)
	}

	return iface, ip, gateway, nil
}

// detectDNS returns current DNS servers as a comma-separated string.
func detectDNS() string {
	out, _ := exec.Command("resolvectl", "dns").Output()
	if len(out) > 0 {
		for _, line := range strings.Split(string(out), "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				servers := strings.TrimSpace(parts[1])
				if servers != "" {
					return strings.ReplaceAll(servers, " ", ", ")
				}
			}
		}
	}
	return "1.1.1.1, 8.8.8.8"
}

// DetectWifi checks if a WiFi interface exists via sysfs (no dependencies needed).
func DetectWifi() bool {
	matches, _ := filepath.Glob("/sys/class/net/*/wireless")
	return len(matches) > 0
}

// getWifiInterface returns the first WiFi interface name.
func getWifiInterface() string {
	matches, _ := filepath.Glob("/sys/class/net/*/wireless")
	if len(matches) == 0 {
		return ""
	}
	// path is /sys/class/net/<iface>/wireless
	dir := filepath.Dir(matches[0])
	return filepath.Base(dir)
}

// WifiNetwork represents a detected WiFi network.
type WifiNetwork struct {
	SSID     string
	Signal   string
	Security string
}

// InstallWifiDeps installs iw and wpasupplicant needed for WiFi.
func InstallWifiDeps(w io.Writer) error {
	fmt.Fprintln(w, "  Installing WiFi dependencies (iw, wpasupplicant) ...")
	cmd := exec.Command("apt-get", "install", "-y", "-qq", "iw", "wpasupplicant")
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// ScanWifiNetworks brings the interface up, scans, and returns available networks.
func ScanWifiNetworks() ([]WifiNetwork, error) {
	iface := getWifiInterface()
	if iface == "" {
		return nil, fmt.Errorf("no wifi interface found")
	}

	// Bring interface up
	exec.Command("ip", "link", "set", iface, "up").Run()
	time.Sleep(2 * time.Second)

	// Scan
	out, err := exec.Command("iw", "dev", iface, "scan").Output()
	if err != nil {
		return nil, fmt.Errorf("wifi scan failed: %w", err)
	}

	// Parse iw scan output
	var networks []WifiNetwork
	seen := make(map[string]bool)
	var current WifiNetwork

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "BSS ") {
			// New network entry — save previous if valid
			if current.SSID != "" && !seen[current.SSID] {
				seen[current.SSID] = true
				networks = append(networks, current)
			}
			current = WifiNetwork{}
		}
		if strings.HasPrefix(line, "SSID: ") {
			current.SSID = strings.TrimPrefix(line, "SSID: ")
		}
		if strings.HasPrefix(line, "signal: ") {
			// "signal: -45.00 dBm" → extract number, convert to percentage
			sig := strings.TrimPrefix(line, "signal: ")
			sig = strings.TrimSuffix(sig, " dBm")
			current.Signal = sig
		}
		if strings.Contains(line, "WPA") || strings.Contains(line, "RSN") {
			current.Security = "WPA"
		}
	}
	// Don't forget the last entry
	if current.SSID != "" && !seen[current.SSID] {
		networks = append(networks, current)
	}

	return networks, nil
}

// ConnectWifi writes a unified netplan config that sets WiFi as primary
// and disables the default route on Ethernet, then applies it.
func ConnectWifi(ssid, password string, w io.Writer) error {
	wifiIface := getWifiInterface()
	if wifiIface == "" {
		return fmt.Errorf("no wifi interface found")
	}

	// Detect current Ethernet interface
	ethIface, _, _, _ := detectNetworkInfo()

	fmt.Fprintf(w, "  Configuring WiFi: %s on %s ...\n", ssid, wifiIface)

	// Build netplan: WiFi with DHCP as primary, Ethernet with high metric (fallback)
	var sb strings.Builder
	sb.WriteString("network:\n  version: 2\n  renderer: networkd\n")

	// Ethernet: keep DHCP but with high route metric so WiFi is preferred
	if ethIface != "" {
		sb.WriteString(fmt.Sprintf("  ethernets:\n    %s:\n      dhcp4: true\n      dhcp4-overrides:\n        route-metric: 200\n", ethIface))
	}

	// WiFi: DHCP with low metric (preferred)
	sb.WriteString(fmt.Sprintf("  wifis:\n    %s:\n      dhcp4: true\n      dhcp4-overrides:\n        route-metric: 100\n      access-points:\n        \"%s\":\n          password: \"%s\"\n", wifiIface, ssid, password))

	if err := os.WriteFile("/etc/netplan/99-packalares.yaml", []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("write netplan: %w", err)
	}

	out, err := exec.Command("netplan", "apply").CombinedOutput()
	if err != nil {
		return fmt.Errorf("netplan apply: %s %w", string(out), err)
	}

	// Wait for WiFi to get an IP
	maxWait := 30
	for i := 0; i < maxWait; i++ {
		fmt.Fprintf(w, "\r  Waiting for WiFi connection ... %ds/%ds", i+1, maxWait)
		time.Sleep(1 * time.Second)
		ipOut, err := exec.Command("ip", "-4", "addr", "show", wifiIface).Output()
		if err == nil {
			for _, line := range strings.Split(string(ipOut), "\n") {
				if strings.Contains(line, "inet ") {
					fields := strings.Fields(strings.TrimSpace(line))
					if len(fields) >= 2 {
						ip := strings.Split(fields[1], "/")[0]
						fmt.Fprintf(w, "\r  WiFi connected: %s (%s)                \n", ssid, ip)
						return nil
					}
				}
			}
		}
	}
	fmt.Fprintln(w)

	return fmt.Errorf("no IP received after %ds", maxWait)
}

// buildNetplanConfig builds a unified netplan config based on current options.
func buildNetplanConfig(opts *InstallOptions, iface, ip, gateway, dns string) string {
	var sb strings.Builder
	sb.WriteString("network:\n  version: 2\n  renderer: networkd\n")

	if opts.NetworkType == "wifi" && opts.WifiSSID != "" {
		wifiIface := getWifiInterface()
		ethIface, _, _, _ := detectNetworkInfo()

		// Ethernet: DHCP fallback with high metric
		if ethIface != "" && ethIface != wifiIface {
			sb.WriteString(fmt.Sprintf("  ethernets:\n    %s:\n      dhcp4: true\n      dhcp4-overrides:\n        route-metric: 200\n", ethIface))
		}

		// WiFi: static or DHCP
		sb.WriteString(fmt.Sprintf("  wifis:\n    %s:\n", wifiIface))
		if opts.StaticIP {
			sb.WriteString(fmt.Sprintf("      dhcp4: false\n      addresses:\n        - %s/24\n      routes:\n        - to: default\n          via: %s\n      nameservers:\n        addresses: [%s]\n", ip, gateway, dns))
		} else {
			sb.WriteString("      dhcp4: true\n      dhcp4-overrides:\n        route-metric: 100\n")
		}
		sb.WriteString(fmt.Sprintf("      access-points:\n        \"%s\":\n          password: \"%s\"\n", opts.WifiSSID, opts.WifiPassword))
	} else {
		// Ethernet: static
		sb.WriteString(fmt.Sprintf("  ethernets:\n    %s:\n      dhcp4: false\n      addresses:\n        - %s/24\n      routes:\n        - to: default\n          via: %s\n      nameservers:\n        addresses: [%s]\n", iface, ip, gateway, dns))
	}

	return sb.String()
}

// GetCurrentIP returns the IP of the default route interface.
func GetCurrentIP() string {
	_, ip, _, err := detectNetworkInfo()
	if err != nil {
		return ""
	}
	return ip
}

// removeLoginHook removes the resume hook from /root/.bashrc.
func removeLoginHook() {
	data, err := os.ReadFile("/root/.bashrc")
	if err != nil {
		return
	}
	marker := "# packalares-install-resume"
	if !strings.Contains(string(data), marker) {
		return
	}
	// Remove everything between marker lines
	var lines []string
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, marker) {
			inBlock = !inBlock
			continue
		}
		if !inBlock {
			lines = append(lines, line)
		}
	}
	os.WriteFile("/root/.bashrc", []byte(strings.Join(lines, "\n")), 0644)
	// Also remove old profile.d hook if present
	os.Remove("/etc/profile.d/packalares-resume.sh")
}

// CreateLoginHook appends a resume check to /root/.bashrc.
func CreateLoginHook() error {
	marker := "# packalares-install-resume"
	hook := `
# packalares-install-resume
if [ -f /etc/packalares/install-state.json ]; then
    echo ""
    echo "  Resuming Packalares installation..."
    echo ""
    packalares install
    # Remove hook after install completes or fails
    sed -i '/# packalares-install-resume/,/sed.*packalares-install-resume/d' /root/.bashrc
fi
`
	// Check if already added
	existing, _ := os.ReadFile("/root/.bashrc")
	if strings.Contains(string(existing), marker) {
		return nil
	}

	f, err := os.OpenFile("/root/.bashrc", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open .bashrc: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(hook)
	return err
}
