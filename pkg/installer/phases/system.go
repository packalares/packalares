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

// configureStaticIP writes a netplan config with the current IP as static.
func configureStaticIP(opts *InstallOptions, w io.Writer) error {
	iface, ip, gateway, err := detectNetworkInfo()
	if err != nil {
		return err
	}

	dns := detectDNS()
	fmt.Fprintf(w, "  Configuring static IP: %s on %s (gateway: %s)\n", ip, iface, gateway)

	// Determine if this is a WiFi or Ethernet interface
	isWifi := opts.NetworkType == "wifi"

	var config string
	if isWifi && opts.WifiSSID != "" {
		config = fmt.Sprintf(`network:
  version: 2
  renderer: networkd
  wifis:
    %s:
      dhcp4: false
      addresses:
        - %s/24
      routes:
        - to: default
          via: %s
      nameservers:
        addresses: [%s]
      access-points:
        "%s":
          password: "%s"
`, iface, ip, gateway, dns, opts.WifiSSID, opts.WifiPassword)
	} else {
		config = fmt.Sprintf(`network:
  version: 2
  renderer: networkd
  ethernets:
    %s:
      dhcp4: false
      addresses:
        - %s/24
      routes:
        - to: default
          via: %s
      nameservers:
        addresses: [%s]
`, iface, ip, gateway, dns)
	}

	if err := os.WriteFile("/etc/netplan/99-packalares-static.yaml", []byte(config), 0600); err != nil {
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

// ConnectWifi writes a netplan WiFi config and applies it.
func ConnectWifi(ssid, password string, w io.Writer) error {
	iface := getWifiInterface()
	if iface == "" {
		return fmt.Errorf("no wifi interface found")
	}

	fmt.Fprintf(w, "  Configuring WiFi: %s on %s ...\n", ssid, iface)

	config := fmt.Sprintf(`network:
  version: 2
  renderer: networkd
  wifis:
    %s:
      dhcp4: true
      access-points:
        "%s":
          password: "%s"
`, iface, ssid, password)

	if err := os.WriteFile("/etc/netplan/99-packalares-wifi.yaml", []byte(config), 0600); err != nil {
		return fmt.Errorf("write netplan: %w", err)
	}

	out, err := exec.Command("netplan", "apply").CombinedOutput()
	if err != nil {
		return fmt.Errorf("netplan apply: %s %w", string(out), err)
	}

	// Wait for WiFi to get an IP
	fmt.Fprintln(w, "  Waiting for WiFi connection ...")
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)
		ipOut, err := exec.Command("ip", "-4", "addr", "show", iface).Output()
		if err == nil {
			for _, line := range strings.Split(string(ipOut), "\n") {
				if strings.Contains(line, "inet ") {
					fields := strings.Fields(strings.TrimSpace(line))
					if len(fields) >= 2 {
						ip := strings.Split(fields[1], "/")[0]
						fmt.Fprintf(w, "  WiFi connected: %s (%s)\n", ssid, ip)
						return nil
					}
				}
			}
		}
	}

	return fmt.Errorf("WiFi connected but no IP received after 30s")
}

// GetCurrentIP returns the IP of the default route interface.
func GetCurrentIP() string {
	_, ip, _, err := detectNetworkInfo()
	if err != nil {
		return ""
	}
	return ip
}

// CreateLoginHook writes the profile.d script for auto-resume after reboot.
func CreateLoginHook() error {
	hook := `#!/bin/bash
if [ -f /etc/packalares/install-state.json ] && [ "$(id -u)" = "0" ]; then
    echo ""
    echo "  Resuming Packalares installation..."
    echo ""
    packalares install
fi
`
	return os.WriteFile("/etc/profile.d/packalares-resume.sh", []byte(hook), 0755)
}
