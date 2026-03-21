package kernel

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var requiredModules = []string{
	"br_netfilter",
	"overlay",
	"ip_vs",
	"ip_vs_rr",
	"ip_vs_wrr",
	"ip_vs_sh",
	"nf_conntrack",
}

var sysctlSettings = map[string]string{
	"net.bridge.bridge-nf-call-iptables":  "1",
	"net.bridge.bridge-nf-call-ip6tables": "1",
	"net.ipv4.ip_forward":                 "1",
	"net.ipv4.conf.all.forwarding":        "1",
	"net.ipv6.conf.all.forwarding":        "1",
	"net.ipv4.tcp_tw_reuse":               "1",
	"net.ipv4.tcp_fin_timeout":            "30",
	"net.ipv4.tcp_keepalive_time":         "600",
	"net.ipv4.tcp_keepalive_probes":       "5",
	"net.ipv4.tcp_keepalive_intvl":        "15",
	"net.core.somaxconn":                  "32768",
	"net.ipv4.tcp_max_syn_backlog":        "8192",
	"net.core.netdev_max_backlog":         "16384",
	"vm.max_map_count":                    "262144",
	"fs.inotify.max_user_instances":       "8192",
	"fs.inotify.max_user_watches":         "524288",
	"fs.file-max":                         "2097152",
	"vm.swappiness":                       "10",
}

func LoadModules() error {
	fmt.Println("  Loading kernel modules ...")

	// Write modules to /etc/modules-load.d for persistence
	var moduleLines []string
	for _, mod := range requiredModules {
		moduleLines = append(moduleLines, mod)
	}
	content := strings.Join(moduleLines, "\n") + "\n"
	if err := os.MkdirAll("/etc/modules-load.d", 0755); err != nil {
		return fmt.Errorf("create modules-load.d: %w", err)
	}
	if err := os.WriteFile("/etc/modules-load.d/packalares.conf", []byte(content), 0644); err != nil {
		return fmt.Errorf("write modules config: %w", err)
	}

	// Load modules now
	for _, mod := range requiredModules {
		cmd := exec.Command("modprobe", mod)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("  Warning: could not load module %s: %s\n", mod, strings.TrimSpace(string(out)))
			// Not fatal — some modules may not be available on all kernels
		}
	}

	return nil
}

func ApplySysctl() error {
	fmt.Println("  Applying sysctl settings ...")

	var lines []string
	for k, v := range sysctlSettings {
		lines = append(lines, fmt.Sprintf("%s = %s", k, v))
	}
	content := "# Packalares sysctl tuning\n" + strings.Join(lines, "\n") + "\n"

	if err := os.MkdirAll("/etc/sysctl.d", 0755); err != nil {
		return fmt.Errorf("create sysctl.d: %w", err)
	}
	if err := os.WriteFile("/etc/sysctl.d/99-packalares.conf", []byte(content), 0644); err != nil {
		return fmt.Errorf("write sysctl config: %w", err)
	}

	cmd := exec.Command("sysctl", "--system")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply sysctl: %s\n%w", string(out), err)
	}

	return nil
}
