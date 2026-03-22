package phases

import (
	"fmt"
	"os/exec"
	"strings"
)

// DetectGPU checks if NVIDIA GPU is present via lspci.
func DetectGPU() bool {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "nvidia")
}

// InstallGPU installs NVIDIA driver, container toolkit, and deploys HAMi.
// Only called if DetectGPU() returns true.
func InstallGPU(opts *InstallOptions) error {
	if !DetectGPU() {
		fmt.Println("  No NVIDIA GPU detected, skipping")
		return nil
	}

	fmt.Println("  NVIDIA GPU detected")

	// Step 1: Install NVIDIA driver
	if err := installNVIDIADriver(); err != nil {
		return fmt.Errorf("install nvidia driver: %w", err)
	}

	// Step 2: Install NVIDIA container toolkit
	if err := installContainerToolkit(); err != nil {
		return fmt.Errorf("install container toolkit: %w", err)
	}

	// Step 3: Configure containerd to use nvidia runtime
	if err := configureContainerdNvidia(); err != nil {
		return fmt.Errorf("configure containerd: %w", err)
	}

	// Step 4: Deploy HAMi for GPU sharing
	if err := deployHAMi(opts); err != nil {
		return fmt.Errorf("deploy hami: %w", err)
	}

	// Step 5: Deploy DCGM exporter for GPU metrics
	if err := applyManifestFile("deploy/framework/dcgm-exporter.yaml", opts); err != nil {
		return fmt.Errorf("deploy dcgm-exporter: %w", err)
	}

	fmt.Println("  GPU setup complete")
	return nil
}

func installNVIDIADriver() error {
	// Check if already installed
	if _, err := exec.Command("nvidia-smi").Output(); err == nil {
		fmt.Println("  NVIDIA driver already installed")
		return nil
	}

	fmt.Println("  Installing NVIDIA driver ...")

	// Add NVIDIA package repo
	cmds := [][]string{
		{"apt-get", "update", "-qq"},
		{"apt-get", "install", "-y", "-qq", "ubuntu-drivers-common"},
		{"ubuntu-drivers", "install", "--gpgpu"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			// Try alternative: install nvidia-driver-535 directly
			if args[0] == "ubuntu-drivers" {
				fmt.Println("  ubuntu-drivers failed, trying direct install ...")
				alt := exec.Command("apt-get", "install", "-y", "-qq", "nvidia-driver-535-server")
				if altOut, altErr := alt.CombinedOutput(); altErr != nil {
					return fmt.Errorf("%s: %s\n%w", args[0], string(altOut), altErr)
				}
				continue
			}
			return fmt.Errorf("%s: %s\n%w", args[0], string(out), err)
		}
	}

	// Verify
	if out, err := exec.Command("nvidia-smi").Output(); err != nil {
		return fmt.Errorf("nvidia-smi failed after install: %w", err)
	} else {
		fmt.Printf("  %s\n", strings.Split(string(out), "\n")[2])
	}

	return nil
}

func installContainerToolkit() error {
	// Check if already installed
	if _, err := exec.LookPath("nvidia-container-runtime"); err == nil {
		fmt.Println("  NVIDIA container toolkit already installed")
		return nil
	}

	fmt.Println("  Installing NVIDIA container toolkit ...")

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

func configureContainerdNvidia() error {
	fmt.Println("  Configuring containerd for NVIDIA runtime ...")

	// Configure nvidia-container-toolkit for containerd
	cmd := exec.Command("nvidia-ctk", "runtime", "configure", "--runtime=containerd")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nvidia-ctk configure: %s\n%w", string(out), err)
	}

	// Restart containerd to pick up the config
	if err := exec.Command("systemctl", "restart", "containerd").Run(); err != nil {
		return fmt.Errorf("restart containerd: %w", err)
	}

	return nil
}

func deployHAMi(opts *InstallOptions) error {
	fmt.Println("  Deploying HAMi GPU scheduler ...")

	if err := applyManifestFile("deploy/framework/hami.yaml", opts); err != nil {
		return fmt.Errorf("deploy hami: %w", err)
	}

	return nil
}
