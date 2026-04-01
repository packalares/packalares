package phases

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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
		if err := configureStaticIP(w); err != nil {
			fmt.Fprintf(w, "  Warning: static IP config failed: %v\n", err)
		}
	}

	return nil
}

// configureStaticIP writes a netplan config with the current IP as static.
func configureStaticIP(w io.Writer) error {
	// Get current default interface and IP
	iface, ip, gateway, err := detectNetworkInfo()
	if err != nil {
		return err
	}

	dns := detectDNS()

	fmt.Fprintf(w, "  Configuring static IP: %s on %s (gateway: %s)\n", ip, iface, gateway)

	config := fmt.Sprintf(`network:
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

	// Write to netplan
	if err := os.WriteFile("/etc/netplan/99-packalares-static.yaml", []byte(config), 0600); err != nil {
		return fmt.Errorf("write netplan: %w", err)
	}

	// Apply
	if out, err := exec.Command("netplan", "apply").CombinedOutput(); err != nil {
		return fmt.Errorf("netplan apply: %s %w", string(out), err)
	}

	return nil
}

// detectNetworkInfo returns the default interface, IP, and gateway.
func detectNetworkInfo() (iface, ip, gateway string, err error) {
	// Default route interface
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", "", fmt.Errorf("ip route: %w", err)
	}
	parts := strings.Fields(string(out))
	// "default via 192.168.1.1 dev eth0 ..."
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

	// IP of that interface
	out, err = exec.Command("ip", "-4", "addr", "show", iface).Output()
	if err != nil {
		return "", "", "", fmt.Errorf("ip addr: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// "inet 192.168.1.10/24 ..."
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
		// Parse "Link 2 (eth0): 192.168.1.1 8.8.8.8"
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
	// Fallback
	return "1.1.1.1, 8.8.8.8"
}

// ConnectWifi connects to a WiFi network using nmcli.
func ConnectWifi(ssid, password string, w io.Writer) error {
	fmt.Fprintf(w, "  Connecting to %s ...\n", ssid)
	cmd := exec.Command("nmcli", "device", "wifi", "connect", ssid, "password", password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wifi connect: %s %w", string(out), err)
	}
	fmt.Fprintf(w, "  %s\n", strings.TrimSpace(string(out)))
	return nil
}

// ScanWifiNetworks returns a list of available WiFi networks.
func ScanWifiNetworks() ([]WifiNetwork, error) {
	out, err := exec.Command("nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY", "device", "wifi", "list").Output()
	if err != nil {
		return nil, fmt.Errorf("wifi scan: %w", err)
	}

	seen := make(map[string]bool)
	var networks []WifiNetwork
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		ssid := parts[0]
		if seen[ssid] {
			continue
		}
		seen[ssid] = true
		networks = append(networks, WifiNetwork{
			SSID:     ssid,
			Signal:   parts[1],
			Security: parts[2],
		})
	}
	return networks, nil
}

// WifiNetwork represents a detected WiFi network.
type WifiNetwork struct {
	SSID     string
	Signal   string
	Security string
}

// DetectWifi checks if a WiFi interface exists.
func DetectWifi() bool {
	out, err := exec.Command("nmcli", "-t", "-f", "TYPE", "device").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "wifi")
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
