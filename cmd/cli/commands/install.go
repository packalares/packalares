package commands

import (
	"errors"
	"fmt"
	"log"

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

	return cmd
}
