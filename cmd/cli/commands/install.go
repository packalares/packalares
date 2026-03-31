package commands

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/packalares/packalares/pkg/installer/phases"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	var opts phases.InstallOptions

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Packalares (full installation)",
		Long: `Performs a full Packalares installation:
  1. Precheck system requirements
  2. Download binaries (K3s, containerd, etcd, helm)
  3. Install containerd + configure
  4. Install etcd + generate TLS certs
  5. Install K3s (with external etcd)
  6. Deploy Calico CNI
  7. Deploy OpenEBS storage
  8. Install Redis as host systemd service
  9. Configure kernel modules and sysctl
 10. Deploy platform Helm charts (Citus, KVRocks, NATS, LLDAP, OPA)
 11. Deploy framework charts (auth, app-service, BFL, system-server, files, market)
 12. Deploy user namespace charts (desktop, wizard)
 13. Deploy monitoring (Prometheus, node-exporter, kube-state-metrics)
 14. Deploy KubeBlocks
 15. Wait for all pods to be ready`,
		Run: func(cmd *cobra.Command, args []string) {
			// Interactive prompts if username/domain not passed as flags
			if !cmd.Flags().Changed("username") && !cmd.Flags().Changed("domain") {
				promptInstallOptions(&opts)
			}

			if err := phases.RunInstall(&opts); err != nil {
				if errors.Is(err, phases.ErrRebootRequired) {
					// Clean exit — state saved, user will resume after reboot.
					return
				}
				log.Fatalf("installation failed: %v", err)
			}
			fmt.Println("\nPackalares installation complete.")
		},
	}

	cmd.Flags().StringVar(&opts.Username, "username", "", "admin username")
	cmd.Flags().StringVar(&opts.Password, "password", "", "admin password (auto-generated if empty)")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "domain name (default: olares.local)")
	cmd.Flags().StringVar(&opts.BaseDir, "base-dir", "", "base directory for installation data")
	cmd.Flags().StringVar(&opts.Registry, "registry", "", "container image registry override (env: PACKALARES_REGISTRY)")
	cmd.Flags().StringVar(&opts.CertMode, "cert-mode", "local", "SSL cert mode: local (self-signed) or acme (Let's Encrypt)")
	cmd.Flags().StringVar(&opts.AcmeEmail, "acme-email", "", "email for Let's Encrypt (required if cert-mode=acme)")
	cmd.Flags().StringVar(&opts.AcmeDNSProvider, "acme-dns-provider", "", "DNS provider for ACME (cloudflare, route53, etc.)")
	cmd.Flags().StringVar(&opts.TailscaleAuthKey, "tailscale-auth-key", "", "Tailscale auth key for VPN access")
	cmd.Flags().StringVar(&opts.TailscaleControlURL, "tailscale-control-url", "", "Tailscale/Headscale control URL")
	cmd.Flags().BoolVar(&opts.SkipPrecheck, "skip-precheck", false, "skip system requirements check")
	cmd.Flags().StringVar(&opts.GPUMethod, "gpu", "", "GPU driver install method: cuda or ubuntu")

	return cmd
}

func promptInstallOptions(opts *phases.InstallOptions) {
	reader := bufio.NewReader(os.Stdin)

	// Apply defaults first so we can show them
	if opts.Username == "" {
		opts.Username = "admin"
	}
	if opts.Domain == "" {
		opts.Domain = os.Getenv("PACKALARES_DOMAIN")
		if opts.Domain == "" {
			opts.Domain = "olares.local"
		}
	}

	fmt.Println()
	fmt.Println("  Packalares Installer")
	fmt.Println()

	printHardwareInfo()

	opts.Username = prompt(reader, "  Username", opts.Username)
	opts.Domain = prompt(reader, "  Domain", opts.Domain)

	// GPU prompt — only if GPU detected and not already set via flag
	if opts.GPUMethod == "" && phases.DetectGPU() {
		gpuName := phases.GetGPUName()
		fmt.Println()
		fmt.Printf("  NVIDIA GPU detected: %s\n", gpuName)
		fmt.Println()
		fmt.Println("  GPU driver install method:")
		fmt.Println("    1) NVIDIA CUDA repo (nvidia-open, latest driver) [recommended]")
		fmt.Println("    2) Ubuntu drivers (ubuntu-drivers autoinstall)")
		fmt.Println()
		choice := prompt(reader, "  Select [1/2]", "1")
		switch choice {
		case "2":
			opts.GPUMethod = phases.GPUMethodUbuntu
		default:
			opts.GPUMethod = phases.GPUMethodCUDA
		}
	}

	fmt.Println()
}

func prompt(reader *bufio.Reader, label, defaultVal string) string {
	fmt.Printf("%s [%s]: ", label, defaultVal)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

func printHardwareInfo() {
	fmt.Println("  Hardware")
	fmt.Println()

	// CPU
	if out, err := exec.Command("bash", "-c", `lscpu | grep "Model name" | sed 's/.*: *//'`).Output(); err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			fmt.Printf("    CPU     %s (%d cores)\n", name, runtime.NumCPU())
		}
	}

	// Memory
	if out, err := exec.Command("bash", "-c", `free -h | awk '/^Mem:/{print $2}'`).Output(); err == nil {
		mem := strings.TrimSpace(string(out))
		if mem != "" {
			fmt.Printf("    Memory  %s\n", mem)
		}
	}

	// Storage
	if out, err := exec.Command("bash", "-c", `df -h / | awk 'NR==2{print $2 " total, " $4 " free"}'`).Output(); err == nil {
		disk := strings.TrimSpace(string(out))
		if disk != "" {
			fmt.Printf("    Disk    %s\n", disk)
		}
	}

	// GPU
	if phases.DetectGPU() {
		fmt.Printf("    GPU     %s\n", phases.GetGPUName())
	}

	// NPU
	if out, err := exec.Command("lspci").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(strings.ToLower(line), "npu") || strings.Contains(strings.ToLower(line), "processing accelerator") {
				parts := strings.SplitN(line, ": ", 3)
				if len(parts) >= 3 {
					fmt.Printf("    NPU     %s\n", strings.TrimSpace(parts[2]))
				}
				break
			}
		}
	}

	// Network
	if out, err := exec.Command("lspci").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			lower := strings.ToLower(line)
			parts := strings.SplitN(line, ": ", 3)
			if len(parts) < 3 {
				continue
			}
			name := strings.TrimSpace(parts[2])
			if strings.Contains(lower, "ethernet") {
				fmt.Printf("    Net     %s\n", name)
			} else if strings.Contains(lower, "network controller") || strings.Contains(lower, "wi-fi") {
				fmt.Printf("    WiFi    %s\n", name)
			}
		}
	}

	fmt.Println()
}
