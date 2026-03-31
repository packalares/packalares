package phases

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DetectGPU checks if NVIDIA GPU is present via lspci.
func DetectGPU() bool {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "nvidia")
}

// GetGPUName returns the NVIDIA GPU model name from lspci, or "Unknown NVIDIA GPU".
func GetGPUName() string {
	// Update PCI ID database so new GPUs (e.g. RTX 5090) are recognized
	exec.Command("update-pciids").Run()

	out, err := exec.Command("lspci").Output()
	if err != nil {
		return "Unknown NVIDIA GPU"
	}
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "nvidia") && (strings.Contains(lower, "vga") || strings.Contains(lower, "3d controller")) {
			parts := strings.SplitN(line, ": ", 3)
			if len(parts) >= 3 {
				return strings.TrimSpace(parts[2])
			}
		}
	}
	return "Unknown NVIDIA GPU"
}

// InstallGPU installs NVIDIA driver, container toolkit, and deploys HAMi.
func InstallGPU(opts *InstallOptions, w io.Writer) error {
	if !DetectGPU() {
		fmt.Fprintln(w, "  No NVIDIA GPU detected, skipping")
		return nil
	}

	fmt.Fprintf(w, "  NVIDIA GPU detected: %s\n", GetGPUName())

	// Step 1: Install NVIDIA driver
	if err := installNVIDIADriver(opts.GPUMethod, w); err != nil {
		return fmt.Errorf("install nvidia driver: %w", err)
	}

	// Step 2: Install NVIDIA container toolkit
	if err := installContainerToolkit(w); err != nil {
		return fmt.Errorf("install container toolkit: %w", err)
	}

	// Step 3: Configure K3s containerd to use nvidia runtime
	if err := configureContainerdNvidia(w); err != nil {
		return fmt.Errorf("configure containerd: %w", err)
	}

	// Step 4: Deploy HAMi via official Helm chart
	if err := deployHAMi(w); err != nil {
		return fmt.Errorf("deploy hami: %w", err)
	}

	// Step 5: Deploy DCGM exporter for GPU metrics
	if err := applyManifestFile("deploy/framework/dcgm-exporter.yaml", opts); err != nil {
		fmt.Fprintf(w, "  Warning: DCGM exporter failed: %v (non-fatal)\n", err)
	}

	fmt.Fprintln(w, "  GPU setup complete")
	return nil
}

func installNVIDIADriver(method string, w io.Writer) error {
	// Already installed?
	if _, err := exec.Command("nvidia-smi").Output(); err == nil {
		fmt.Fprintln(w, "  NVIDIA driver already installed")
		return nil
	}

	switch method {
	case GPUMethodUbuntu:
		return installDriverUbuntu(w)
	default:
		return installDriverCUDA(w)
	}
}

// ubuntuVersionCode returns the Ubuntu version without dots, e.g. "2404" for 24.04.
func ubuntuVersionCode() string {
	out, err := exec.Command("bash", "-c", `. /etc/os-release && echo "$VERSION_ID"`).Output()
	if err != nil {
		return "2404" // fallback
	}
	return strings.TrimSpace(strings.ReplaceAll(string(out), ".", ""))
}

// installDriverCUDA installs via the NVIDIA CUDA repository (nvidia-open package).
func installDriverCUDA(w io.Writer) error {
	fmt.Fprintln(w, "  Installing NVIDIA driver via CUDA repo (nvidia-open) ...")

	ver := ubuntuVersionCode()
	// NVIDIA repo uses x86_64/sbsa, not amd64/arm64
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "sbsa"
	}
	url := fmt.Sprintf("https://developer.download.nvidia.com/compute/cuda/repos/ubuntu%s/%s/cuda-keyring_1.1-1_all.deb", ver, arch)

	cmds := [][]string{
		{"bash", "-c", fmt.Sprintf("wget -q %s -O /tmp/cuda-keyring.deb", url)},
		{"dpkg", "-i", "/tmp/cuda-keyring.deb"},
		{"apt-get", "update", "-qq"},
		{"apt-get", "install", "-y", "-qq", "nvidia-open"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = w
		cmd.Stderr = w
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", args[0], err)
		}
	}

	os.Remove("/tmp/cuda-keyring.deb")
	return verifyDriverOrReboot(w)
}

// installDriverUbuntu installs via ubuntu-drivers autoinstall.
func installDriverUbuntu(w io.Writer) error {
	fmt.Fprintln(w, "  Installing NVIDIA driver via ubuntu-drivers autoinstall ...")

	cmds := [][]string{
		{"apt-get", "update", "-qq"},
		{"apt-get", "install", "-y", "-qq", "ubuntu-drivers-common"},
		{"ubuntu-drivers", "autoinstall"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = w
		cmd.Stderr = w
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", args[0], err)
		}
	}

	return verifyDriverOrReboot(w)
}

// verifyDriverOrReboot checks if nvidia-smi works. If not, signals a reboot is needed.
func verifyDriverOrReboot(w io.Writer) error {
	out, err := exec.Command("nvidia-smi").Output()
	if err == nil {
		// Working — print the driver/GPU line
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Driver Version") {
				fmt.Fprintf(w, "  %s\n", strings.TrimSpace(line))
				break
			}
		}
		return nil
	}

	// nvidia-smi failed — driver installed but kernel module not loaded (needs reboot)
	fmt.Fprintln(w, "  GPU driver installed but kernel module not loaded.")
	fmt.Fprintln(w, "  A reboot is required before GPU setup can continue.")
	return ErrRebootRequired
}

func installContainerToolkit(w io.Writer) error {
	if _, err := exec.LookPath("nvidia-container-runtime"); err == nil {
		fmt.Fprintln(w, "  NVIDIA container toolkit already installed")
		return nil
	}

	fmt.Fprintln(w, "  Installing NVIDIA container toolkit ...")

	cmds := [][]string{
		{"bash", "-c", `curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg`},
		{"bash", "-c", `curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | tee /etc/apt/sources.list.d/nvidia-container-toolkit.list`},
		{"apt-get", "update", "-qq"},
		{"apt-get", "install", "-y", "-qq", "nvidia-container-toolkit"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s\n%w", args, string(out), err)
		}
	}

	return nil
}

func configureContainerdNvidia(w io.Writer) error {
	fmt.Fprintln(w, "  Configuring K3s containerd for NVIDIA runtime ...")

	os.MkdirAll("/etc/containerd/conf.d", 0755)

	// Run nvidia-ctk to write the NVIDIA runtime config to conf.d
	cmd := exec.Command("nvidia-ctk", "runtime", "configure",
		"--runtime=containerd",
		"--set-as-default")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nvidia-ctk configure: %s\n%w", string(out), err)
	}

	// Create K3s containerd template that imports the nvidia config.
	// Written AFTER nvidia-ctk so it doesn't get overwritten.
	os.MkdirAll("/var/lib/rancher/k3s/agent/etc/containerd", 0755)
	tmpl := `imports = ["/etc/containerd/conf.d/*.toml"]
`
	if err := os.WriteFile("/var/lib/rancher/k3s/agent/etc/containerd/config.toml.tmpl", []byte(tmpl), 0644); err != nil {
		return fmt.Errorf("write containerd template: %w", err)
	}

	// Restart K3s to pick up the new containerd config
	fmt.Fprintln(w, "  Restarting K3s ...")
	if err := exec.Command("systemctl", "restart", "k3s").Run(); err != nil {
		return fmt.Errorf("restart k3s: %w", err)
	}

	// Wait for K3s to be ready
	fmt.Fprintln(w, "  Waiting for K3s ...")
	for i := 0; i < 30; i++ {
		if exec.Command("kubectl", "get", "nodes").Run() == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}

func deployHAMi(w io.Writer) error {
	fmt.Fprintln(w, "  Deploying HAMi GPU scheduler (official Helm chart) ...")

	// Add HAMi Helm repo
	if out, err := exec.Command("helm", "repo", "add", "hami",
		"https://project-hami.github.io/HAMi").CombinedOutput(); err != nil {
		return fmt.Errorf("helm repo add: %s\n%w", string(out), err)
	}

	exec.Command("helm", "repo", "update").Run()

	// Install HAMi
	cmd := exec.Command("helm", "install", "hami", "hami/hami",
		"-n", "hami-system",
		"--create-namespace",
		"--set", "devicePlugin.deviceSplitCount=10",
		"--set", "devicePlugin.deviceMemoryScaling=1.0",
		"--wait",
		"--timeout", "120s",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("helm install hami: %s\n%w", string(out), err)
	}

	// Label the node so HAMi device-plugin DaemonSet can schedule
	nodeName, _ := os.Hostname()
	exec.Command("kubectl", "label", "node", nodeName, "gpu=on", "--overwrite").Run()
	fmt.Fprintf(w, "  Labeled node %s with gpu=on\n", nodeName)

	return nil
}
