package phases

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func RunUninstall() error {
	steps := []phase{
		{"Stop K3s", stopK3s},
		{"Stop etcd", stopEtcd},
		{"Stop containerd", stopContainerd},
		{"Stop Redis", stopRedis},
		{"Remove K3s", removeK3s},
		{"Remove etcd", removeEtcd},
		{"Remove containerd", removeContainerd},
		{"Remove Redis service", removeRedis},
		{"Remove systemd units", removeSystemdUnits},
		{"Clean Kubernetes state", cleanKubeState},
		{"Remove binaries", removeBinaries},
		{"Remove release file", removeReleaseFile},
	}

	total := len(steps)
	for i, s := range steps {
		fmt.Printf("[%d/%d] %s ...\n", i+1, total, s.Name)
		if err := s.Fn(); err != nil {
			fmt.Printf("  Warning: %v (continuing)\n", err)
		}
	}

	return nil
}

func stopService(name string) error {
	if _, err := os.Stat(filepath.Join(SystemdDir, name+".service")); os.IsNotExist(err) {
		return nil
	}
	return exec.Command("systemctl", "stop", name).Run()
}

func disableService(name string) error {
	exec.Command("systemctl", "disable", name).Run()
	return nil
}

func stopK3s() error {
	// K3s has its own kill script
	if _, err := os.Stat("/usr/local/bin/k3s-killall.sh"); err == nil {
		exec.Command("/usr/local/bin/k3s-killall.sh").Run()
	}
	return stopService("k3s")
}

func stopEtcd() error {
	return stopService("etcd")
}

func stopContainerd() error {
	return stopService("containerd")
}

func stopRedis() error {
	return stopService("redis-server")
}

func removeK3s() error {
	// K3s provides its own uninstall script
	if _, err := os.Stat("/usr/local/bin/k3s-uninstall.sh"); err == nil {
		exec.Command("/usr/local/bin/k3s-uninstall.sh").Run()
	}
	disableService("k3s")

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
	} {
		os.RemoveAll(d)
	}

	return nil
}

func removeEtcd() error {
	disableService("etcd")
	os.Remove(filepath.Join(SystemdDir, "etcd.service"))

	for _, d := range []string{
		"/var/lib/etcd",
		ETCDCertDir,
	} {
		os.RemoveAll(d)
	}

	os.Remove("/usr/local/bin/etcd")
	os.Remove("/usr/local/bin/etcdctl")
	os.Remove("/usr/local/bin/etcdutl")

	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func removeContainerd() error {
	disableService("containerd")
	os.Remove(filepath.Join(SystemdDir, "containerd.service"))
	os.RemoveAll(ContainerdCfgDir)
	os.Remove("/usr/local/bin/containerd")
	os.Remove("/usr/local/bin/containerd-shim")
	os.Remove("/usr/local/bin/containerd-shim-runc-v2")
	os.Remove("/usr/local/bin/runc")

	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func removeRedis() error {
	disableService("redis-server")
	os.Remove(filepath.Join(SystemdDir, "redis-server.service"))
	os.RemoveAll("/var/lib/redis")
	os.Remove("/etc/redis/redis.conf")

	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func removeSystemdUnits() error {
	// Remove any remaining packalares-related units
	entries, err := os.ReadDir(SystemdDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "packalares") || strings.Contains(e.Name(), "olaresd") {
			os.Remove(filepath.Join(SystemdDir, e.Name()))
		}
	}
	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func cleanKubeState() error {
	for _, d := range []string{
		"/var/lib/kubelet",
		KubeConfigDir,
		"/var/lib/cni",
		"/etc/cni",
		"/run/flannel",
	} {
		os.RemoveAll(d)
	}

	// Clean iptables rules
	for _, cmd := range []string{"iptables", "ip6tables"} {
		exec.Command(cmd, "-F").Run()
		exec.Command(cmd, "-t", "nat", "-F").Run()
		exec.Command(cmd, "-t", "mangle", "-F").Run()
		exec.Command(cmd, "-X").Run()
	}

	// Remove CNI interfaces
	for _, iface := range []string{"cni0", "flannel.1", "vxlan.calico", "tunl0"} {
		exec.Command("ip", "link", "delete", iface).Run()
	}

	return nil
}

func removeBinaries() error {
	for _, f := range []string{
		"/usr/local/bin/helm",
	} {
		os.Remove(f)
	}

	// Remove base dir (keep this last)
	baseDir := DefaultBaseDir
	if data, err := os.ReadFile(ReleaseFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PACKALARES_BASE_DIR=") {
				baseDir = strings.TrimPrefix(line, "PACKALARES_BASE_DIR=")
				break
			}
		}
	}

	os.RemoveAll(baseDir)
	return nil
}

func removeReleaseFile() error {
	os.Remove(ReleaseFile)
	os.RemoveAll("/etc/packalares")
	return nil
}

// RunUpgrade re-downloads the target version and redeploys
func RunUpgrade(targetVersion string) error {
	fmt.Printf("Upgrading to version %s ...\n", targetVersion)

	steps := []phase{
		{"Download new version", func() error {
			return fmt.Errorf("upgrade not yet implemented for version %s — download from https://github.com/packalares/packalares/releases", targetVersion)
		}},
	}

	// Read current installation info
	data, err := os.ReadFile(ReleaseFile)
	if err != nil {
		return fmt.Errorf("cannot read release file %s: %w — is Packalares installed?", ReleaseFile, err)
	}

	var currentVersion string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PACKALARES_VERSION=") {
			currentVersion = strings.TrimPrefix(line, "PACKALARES_VERSION=")
		}
	}

	if currentVersion == targetVersion {
		return fmt.Errorf("already running version %s", currentVersion)
	}

	fmt.Printf("Current version: %s -> Target version: %s\n", currentVersion, targetVersion)

	_ = time.Now() // suppress unused import
	for _, s := range steps {
		fmt.Printf("  %s ...\n", s.Name)
		if err := s.Fn(); err != nil {
			return err
		}
	}

	return nil
}
