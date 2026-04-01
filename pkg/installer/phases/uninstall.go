package phases

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type uninstallStep struct {
	Name string
	Fn   func() error
}

func RunUninstall() error {
	// Confirm with user
	fmt.Println()
	fmt.Print("  This will remove ALL Packalares components. Are you sure? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(input)) != "y" {
		fmt.Println("  Aborted.")
		return nil
	}
	fmt.Println()

	steps := []uninstallStep{
		// Stop all services first
		{"Stop K3s", stopK3s},
		{"Stop etcd", stopEtcd},
		{"Stop containerd", stopContainerd},

		// Remove K3s (uses its own uninstall script which cleans most K3s state)
		{"Remove K3s", removeK3s},

		// Remove etcd data and service
		{"Remove etcd", removeEtcd},

		// Remove containerd (standalone, if installed alongside K3s)
		{"Remove containerd", removeContainerd},

		// Clean all K8s/CNI state (iptables, interfaces, dirs)
		{"Clean networking", cleanNetworking},

		// Remove NVIDIA GPU drivers and toolkit (if installed)
		{"Remove GPU drivers", removeGPUDrivers},

		// Remove all data directories
		{"Remove data", removeData},

		// Remove binaries
		{"Remove binaries", removeBinaries},

		// Remove systemd units
		{"Remove systemd units", removeSystemdUnits},

		// Remove kernel modules config and sysctl
		{"Remove kernel config", removeKernelConfig},

		// Remove install state and login hook
		{"Remove install state", removeInstallFiles},

		// Clean logs
		{"Clean logs", cleanLogs},

		// Clean apt cache
		{"Clean apt cache", cleanAptCache},
	}

	total := len(steps)
	for i, s := range steps {
		fmt.Printf("[%d/%d] %s ...\n", i+1, total, s.Name)
		if err := s.Fn(); err != nil {
			fmt.Printf("  Warning: %v (continuing)\n", err)
		}
	}

	fmt.Println()
	fmt.Println("  Packalares has been removed.")
	fmt.Println()
	fmt.Print("  Press Enter to reboot (or Ctrl+C to skip): ")
	reader.ReadString('\n')
	exec.Command("reboot").Run()

	return nil
}

// --- Stop services ---

func stopK3s() error {
	// K3s has its own kill script that stops all containers
	if _, err := os.Stat("/usr/local/bin/k3s-killall.sh"); err == nil {
		exec.Command("/usr/local/bin/k3s-killall.sh").Run()
	}
	exec.Command("systemctl", "stop", "k3s").Run()
	exec.Command("systemctl", "disable", "k3s").Run()
	return nil
}

func stopEtcd() error {
	exec.Command("systemctl", "stop", "etcd").Run()
	exec.Command("systemctl", "disable", "etcd").Run()
	return nil
}

func stopContainerd() error {
	exec.Command("systemctl", "stop", "containerd").Run()
	exec.Command("systemctl", "disable", "containerd").Run()
	return nil
}

// --- Remove components ---

func removeK3s() error {
	// K3s provides its own uninstall script
	if _, err := os.Stat("/usr/local/bin/k3s-uninstall.sh"); err == nil {
		exec.Command("/usr/local/bin/k3s-uninstall.sh").Run()
	}

	// Remove any remaining K3s files
	for _, f := range []string{
		"/usr/local/bin/k3s",
		"/usr/local/bin/kubectl",
		"/usr/local/bin/crictl",
		"/usr/local/bin/ctr",
		"/usr/local/bin/k3s-killall.sh",
		"/usr/local/bin/k3s-uninstall.sh",
	} {
		os.Remove(f)
	}

	for _, d := range []string{
		"/etc/rancher",
		"/var/lib/rancher",
		"/run/k3s",
	} {
		os.RemoveAll(d)
	}

	os.Remove("/etc/crictl.yaml")
	return nil
}

func removeEtcd() error {
	os.Remove(filepath.Join(SystemdDir, "etcd.service"))

	for _, d := range []string{
		"/var/lib/etcd",
		"/var/backups/kube_etcd",
		ETCDCertDir,
	} {
		os.RemoveAll(d)
	}

	for _, f := range []string{
		"/usr/local/bin/etcd",
		"/usr/local/bin/etcdctl",
		"/usr/local/bin/etcdutl",
		"/usr/local/bin/kube-scripts/etcd-backup.sh",
	} {
		os.Remove(f)
	}
	os.RemoveAll("/usr/local/bin/kube-scripts")

	return nil
}

func removeContainerd() error {
	for _, d := range []string{
		"/var/lib/containerd",
		"/run/containerd",
		ContainerdCfgDir,
	} {
		os.RemoveAll(d)
	}
	os.Remove(filepath.Join(SystemdDir, "containerd.service"))
	return nil
}

// --- Networking ---

func cleanNetworking() error {
	// Flush iptables rules
	for _, cmd := range []string{"iptables", "ip6tables"} {
		exec.Command(cmd, "-F").Run()
		exec.Command(cmd, "-t", "nat", "-F").Run()
		exec.Command(cmd, "-t", "mangle", "-F").Run()
		exec.Command(cmd, "-X").Run()
	}

	// Remove CNI/overlay interfaces
	for _, iface := range []string{
		"cni0", "flannel.1", "vxlan.calico", "tunl0",
		"cali+", "veth+", "docker0",
	} {
		exec.Command("ip", "link", "delete", iface).Run()
	}

	// Remove CNI state
	for _, d := range []string{
		"/var/lib/cni",
		"/etc/cni",
		"/run/flannel",
		"/var/lib/calico",
		"/var/run/calico",
	} {
		os.RemoveAll(d)
	}

	return nil
}

// --- GPU ---

func removeGPUDrivers() error {
	// Check if NVIDIA is installed
	if _, err := exec.Command("dpkg", "-l", "nvidia-open").CombinedOutput(); err != nil {
		// Also check ubuntu-drivers variant
		if _, err := exec.Command("dpkg", "-l", "nvidia-driver-*").CombinedOutput(); err != nil {
			fmt.Println("  No GPU drivers found")
			return nil
		}
	}

	fmt.Println("  Removing NVIDIA drivers and toolkit ...")

	// Remove all NVIDIA and CUDA packages
	exec.Command("apt-get", "remove", "--purge", "--autoremove", "-y",
		"nvidia-open", "cuda-keyring", "nvidia-container-toolkit",
	).Run()

	// Remove wildcard packages
	cmd := exec.Command("bash", "-c", "dpkg -l | grep -E 'nvidia|cuda|libnvidia' | awk '{print $2}' | xargs -r apt-get remove --purge -y")
	cmd.Run()

	// Remove NVIDIA repos and keys
	removeGlob("/etc/apt/sources.list.d/cuda*")
	removeGlob("/etc/apt/sources.list.d/nvidia*")
	removeGlob("/usr/share/keyrings/nvidia-*")

	// Remove NVIDIA container runtime config
	os.RemoveAll("/etc/containerd/conf.d")
	os.Remove("/var/lib/rancher/k3s/agent/etc/containerd/config.toml.tmpl")

	// Remove HAMi if installed
	exec.Command("helm", "uninstall", "hami", "-n", "hami-system").Run()

	exec.Command("apt-get", "update", "-qq").Run()

	return nil
}

// --- Data ---

func removeData() error {
	// Read base dir from release file
	baseDir := DefaultBaseDir
	if data, err := os.ReadFile(ReleaseFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PACKALARES_BASE_DIR=") {
				baseDir = strings.TrimPrefix(line, "PACKALARES_BASE_DIR=")
				break
			}
		}
	}

	dirs := []string{
		baseDir,                          // /opt/packalares
		"/etc/packalares",                // config, certs, state
		"/var/lib/packalares",            // app data, kvrocks, tailscale state
		"/var/lib/kubelet",               // kubelet state
		"/var/lib/openebs",               // OpenEBS local PVs
		"/var/openebs",                   // OpenEBS data
		KubeConfigDir,                    // /etc/kubernetes
		"/root/.kube",                    // kubectl config cache
		"/root/.helm",                    // helm cache
		"/root/.config/helm",             // helm config
		"/root/.cache/helm",              // helm cache
	}

	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			fmt.Printf("  Warning: could not remove %s: %v\n", d, err)
		}
	}

	return nil
}

// --- Binaries ---

func removeBinaries() error {
	binaries := []string{
		"/usr/local/bin/helm",
		"/usr/local/bin/packalares",
	}

	for _, f := range binaries {
		os.Remove(f)
	}

	return nil
}

// --- Systemd ---

func removeSystemdUnits() error {
	// Remove any packalares/olares related units
	entries, err := os.ReadDir(SystemdDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, "packalares") ||
			strings.Contains(name, "olares") ||
			strings.Contains(name, "k3s") ||
			strings.Contains(name, "etcd") {
			os.Remove(filepath.Join(SystemdDir, name))
		}
	}

	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "reset-failed").Run()
	return nil
}

// --- Kernel config ---

func removeKernelConfig() error {
	// Remove kernel module configs
	removeGlob("/etc/modules-load.d/packalares*")
	removeGlob("/etc/modules-load.d/k8s*")

	// Remove sysctl configs
	removeGlob("/etc/sysctl.d/99-packalares*")
	removeGlob("/etc/sysctl.d/99-k8s*")

	// Reload sysctl
	exec.Command("sysctl", "--system").Run()

	return nil
}

// --- Install state and hook ---

func removeInstallFiles() error {
	os.Remove(StateFilePath)
	os.Remove(ReleaseFile)
	os.Remove("/etc/profile.d/packalares-resume.sh")
	return nil
}

// --- Logs ---

func cleanLogs() error {
	// Truncate journal to last day
	exec.Command("journalctl", "--vacuum-time=1d").Run()

	// Remove rotated log files
	removeGlob("/var/log/*.gz")
	removeGlob("/var/log/*.1")
	removeGlob("/var/log/*.old")
	removeGlob("/var/log/pods/*")
	removeGlob("/var/log/containers/*")

	return nil
}

// --- Apt cache ---

func cleanAptCache() error {
	exec.Command("apt-get", "clean").Run()
	exec.Command("apt-get", "autoremove", "-y").Run()
	return nil
}

// --- Upgrade (stub) ---

func RunUpgrade(targetVersion string) error {
	return fmt.Errorf("upgrade not yet implemented for version %s — download from https://github.com/packalares/packalares/releases", targetVersion)
}

// --- Helpers ---

func removeGlob(pattern string) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, m := range matches {
		os.RemoveAll(m)
	}
}
