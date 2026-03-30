package phases

import (
	"fmt"
	"os"
	"os/exec"
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

// InstallGPU installs NVIDIA driver, container toolkit, and deploys HAMi.
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

	// Step 3: Configure K3s containerd to use nvidia runtime
	if err := configureContainerdNvidia(); err != nil {
		return fmt.Errorf("configure containerd: %w", err)
	}

	// Step 4: Deploy HAMi via official Helm chart
	if err := deployHAMi(); err != nil {
		return fmt.Errorf("deploy hami: %w", err)
	}

	// Step 5: Deploy DCGM exporter for GPU metrics
	if err := applyManifestFile("deploy/framework/dcgm-exporter.yaml", opts); err != nil {
		fmt.Printf("  Warning: DCGM exporter failed: %v (non-fatal)\n", err)
	}

	fmt.Println("  GPU setup complete")
	return nil
}

func installNVIDIADriver() error {
	if _, err := exec.Command("nvidia-smi").Output(); err == nil {
		fmt.Println("  NVIDIA driver already installed")
		return nil
	}

	fmt.Println("  Installing NVIDIA driver ...")

	cmds := [][]string{
		{"apt-get", "update", "-qq"},
		{"apt-get", "install", "-y", "-qq", "ubuntu-drivers-common"},
		{"ubuntu-drivers", "install", "--gpgpu"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
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

	// Install nvidia-utils (provides nvidia-smi)
	exec.Command("apt-get", "install", "-y", "-qq", "nvidia-utils-server").Run()

	// Verify — may need reboot for kernel module
	if out, err := exec.Command("nvidia-smi").Output(); err != nil {
		if exec.Command("dpkg", "-l", "nvidia-driver*").Run() == nil {
			fmt.Println("  NVIDIA driver installed but nvidia-smi not available yet")
			fmt.Println("  A reboot may be required to load the kernel module")
			return nil
		}
		return fmt.Errorf("nvidia-smi failed after install: %w", err)
	} else {
		fmt.Printf("  %s\n", strings.Split(string(out), "\n")[2])
	}

	return nil
}

func installContainerToolkit() error {
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
	fmt.Println("  Configuring K3s containerd for NVIDIA runtime ...")

	// nvidia-ctk writes to /etc/containerd/conf.d/99-nvidia.toml by default.
	// K3s needs a config template that imports from conf.d.
	// We write the template FIRST, then run nvidia-ctk which writes to conf.d.
	os.MkdirAll("/etc/containerd/conf.d", 0755)

	// Run nvidia-ctk to write the NVIDIA runtime config to conf.d
	// Don't use --config flag — let nvidia-ctk write to conf.d by default
	cmd := exec.Command("nvidia-ctk", "runtime", "configure",
		"--runtime=containerd",
		"--set-as-default")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nvidia-ctk configure: %s\n%w", string(out), err)
	}

	// Create K3s containerd template that imports the nvidia config.
	// This must be written AFTER nvidia-ctk so it doesn't get overwritten.
	os.MkdirAll("/var/lib/rancher/k3s/agent/etc/containerd", 0755)
	tmpl := `imports = ["/etc/containerd/conf.d/*.toml"]
`
	if err := os.WriteFile("/var/lib/rancher/k3s/agent/etc/containerd/config.toml.tmpl", []byte(tmpl), 0644); err != nil {
		return fmt.Errorf("write containerd template: %w", err)
	}

	// Restart K3s to pick up the new containerd config
	fmt.Println("  Restarting K3s ...")
	if err := exec.Command("systemctl", "restart", "k3s").Run(); err != nil {
		return fmt.Errorf("restart k3s: %w", err)
	}

	// Wait for K3s to be ready
	fmt.Println("  Waiting for K3s ...")
	for i := 0; i < 30; i++ {
		if exec.Command("kubectl", "get", "nodes").Run() == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}

func deployHAMi() error {
	fmt.Println("  Deploying HAMi GPU scheduler (official Helm chart) ...")

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
	fmt.Printf("  Labeled node %s with gpu=on\n", nodeName)

	return nil
}
