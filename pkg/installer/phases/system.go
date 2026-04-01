package phases

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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

// --- Netplan handling ---

// findNetplanFile returns the path to the existing netplan config file.
func findNetplanFile() string {
	matches, _ := filepath.Glob("/etc/netplan/*.yaml")
	if len(matches) == 0 {
		matches, _ = filepath.Glob("/etc/netplan/*.yml")
	}
	if len(matches) > 0 {
		return matches[0]
	}
	return "/etc/netplan/01-netcfg.yaml"
}

// readNetplan reads and parses the existing netplan config.
func readNetplan() (map[string]interface{}, string) {
	path := findNetplanFile()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{
			"network": map[string]interface{}{
				"version": 2,
			},
		}, path
	}
	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return map[string]interface{}{
			"network": map[string]interface{}{
				"version": 2,
			},
		}, path
	}
	return config, path
}

// writeNetplan writes the config back and applies it.
func writeNetplan(config map[string]interface{}, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal netplan: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write netplan: %w", err)
	}
	out, err := exec.Command("netplan", "apply").CombinedOutput()
	if err != nil {
		return fmt.Errorf("netplan apply: %s %w", string(out), err)
	}
	return nil
}

// getNetworkSection returns or creates a section in the netplan config.
func getNetworkSection(config map[string]interface{}, section string) map[string]interface{} {
	network, ok := config["network"].(map[string]interface{})
	if !ok {
		network = map[string]interface{}{"version": 2}
		config["network"] = network
	}
	s, ok := network[section].(map[string]interface{})
	if !ok {
		s = map[string]interface{}{}
		network[section] = s
	}
	return s
}

// ConnectWifi adds WiFi config to the existing netplan file and applies it.
func ConnectWifi(ssid, password string, w io.Writer) error {
	wifiIface := getWifiInterface()
	if wifiIface == "" {
		return fmt.Errorf("no wifi interface found")
	}

	fmt.Fprintf(w, "  Configuring WiFi: %s on %s ...\n", ssid, wifiIface)

	config, path := readNetplan()
	wifis := getNetworkSection(config, "wifis")

	wifis[wifiIface] = map[string]interface{}{
		"dhcp4": true,
		"access-points": map[string]interface{}{
			ssid: map[string]interface{}{
				"password": password,
			},
		},
	}

	// Remove old packalares netplan file if it exists (from previous attempt)
	os.Remove("/etc/netplan/99-packalares.yaml")

	if err := writeNetplan(config, path); err != nil {
		return err
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

// configureStaticIP modifies the existing netplan config to use a static IP.
func configureStaticIP(opts *InstallOptions, w io.Writer) error {
	iface, ip, gateway, err := detectNetworkInfo()
	if err != nil {
		return err
	}
	dns := detectDNS()

	fmt.Fprintf(w, "  Configuring static IP: %s on %s (gateway: %s)\n", ip, iface, gateway)

	config, path := readNetplan()

	staticConfig := map[string]interface{}{
		"dhcp4": false,
		"addresses": []string{
			ip + "/24",
		},
		"routes": []interface{}{
			map[string]interface{}{
				"to":  "default",
				"via": gateway,
			},
		},
		"nameservers": map[string]interface{}{
			"addresses": strings.Split(dns, ", "),
		},
	}

	// Determine if the active interface is WiFi or Ethernet
	wifiIface := getWifiInterface()
	if opts.NetworkType == "wifi" && iface == wifiIface {
		// WiFi interface — preserve access-points
		wifis := getNetworkSection(config, "wifis")
		existing, ok := wifis[iface].(map[string]interface{})
		if ok {
			if ap, exists := existing["access-points"]; exists {
				staticConfig["access-points"] = ap
			}
		}
		wifis[iface] = staticConfig
	} else {
		// Ethernet interface
		ethernets := getNetworkSection(config, "ethernets")
		ethernets[iface] = staticConfig
	}

	return writeNetplan(config, path)
}

// --- Network detection ---

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

// --- WiFi detection ---

func DetectWifi() bool {
	matches, _ := filepath.Glob("/sys/class/net/*/wireless")
	return len(matches) > 0
}

func getWifiInterface() string {
	matches, _ := filepath.Glob("/sys/class/net/*/wireless")
	if len(matches) == 0 {
		return ""
	}
	dir := filepath.Dir(matches[0])
	return filepath.Base(dir)
}

type WifiNetwork struct {
	SSID     string
	Signal   string
	Security string
}

func InstallWifiDeps(w io.Writer) error {
	fmt.Fprintln(w, "  Installing WiFi dependencies (iw, wpasupplicant) ...")
	cmd := exec.Command("apt-get", "install", "-y", "-qq", "iw", "wpasupplicant")
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

func ScanWifiNetworks() ([]WifiNetwork, error) {
	iface := getWifiInterface()
	if iface == "" {
		return nil, fmt.Errorf("no wifi interface found")
	}

	exec.Command("ip", "link", "set", iface, "up").Run()
	time.Sleep(2 * time.Second)

	out, err := exec.Command("iw", "dev", iface, "scan").Output()
	if err != nil {
		return nil, fmt.Errorf("wifi scan failed: %w", err)
	}

	var networks []WifiNetwork
	seen := make(map[string]bool)
	var current WifiNetwork

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "BSS ") {
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
			sig := strings.TrimPrefix(line, "signal: ")
			sig = strings.TrimSuffix(sig, " dBm")
			current.Signal = sig
		}
		if strings.Contains(line, "WPA") || strings.Contains(line, "RSN") {
			current.Security = "WPA"
		}
	}
	if current.SSID != "" && !seen[current.SSID] {
		networks = append(networks, current)
	}

	return networks, nil
}

// --- IP helpers ---

func GetCurrentIP() string {
	_, ip, _, err := detectNetworkInfo()
	if err != nil {
		return ""
	}
	return ip
}

// GetWifiIP returns the IP of the WiFi interface specifically.
func GetWifiIP() string {
	iface := getWifiInterface()
	if iface == "" {
		return ""
	}
	out, err := exec.Command("ip", "-4", "addr", "show", iface).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strings.Split(fields[1], "/")[0]
			}
		}
	}
	return ""
}

// --- Login hook ---

func removeLoginHook() {
	data, err := os.ReadFile("/root/.bashrc")
	if err != nil {
		return
	}
	marker := "# packalares-install-resume"
	if !strings.Contains(string(data), marker) {
		return
	}
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
	os.Remove("/etc/profile.d/packalares-resume.sh")
}

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
