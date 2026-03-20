package terminus

import (
	"fmt"
	"os"
	"strings"

	"github.com/beclab/Olares/cli/pkg/common"
	"github.com/beclab/Olares/cli/pkg/core/connector"
	"github.com/beclab/Olares/cli/pkg/core/logger"
	"github.com/beclab/Olares/cli/pkg/core/task"
	"github.com/beclab/Olares/cli/pkg/utils"
)

// GetSetupConfig prompts for cert mode and Tailscale config during install.
type GetSetupConfig struct {
	common.KubeAction
}

func (s *GetSetupConfig) Execute(runtime connector.Runtime) error {
	var err error

	s.KubeConf.Arg.CertMode, err = s.getCertMode()
	if err != nil {
		return err
	}

	if s.KubeConf.Arg.CertMode == "acme" {
		s.KubeConf.Arg.AcmeEmail, err = s.getAcmeEmail()
		if err != nil {
			return err
		}
		s.KubeConf.Arg.AcmeDNSProvider, err = s.getAcmeDNSProvider()
		if err != nil {
			return err
		}
	}

	s.KubeConf.Arg.TailscaleAuthKey, err = s.getTailscaleAuthKey()
	if err != nil {
		return err
	}

	if s.KubeConf.Arg.TailscaleAuthKey != "" {
		s.KubeConf.Arg.TailscaleControlURL, err = s.getTailscaleControlURL()
		if err != nil {
			return err
		}
	}

	// Set env vars so the rest of the system picks them up
	if s.KubeConf.Arg.CertMode != "" {
		os.Setenv("OLARES_CERT_MODE", s.KubeConf.Arg.CertMode)
	}
	if s.KubeConf.Arg.AcmeEmail != "" {
		os.Setenv("OLARES_ACME_EMAIL", s.KubeConf.Arg.AcmeEmail)
	}
	if s.KubeConf.Arg.AcmeDNSProvider != "" {
		os.Setenv("OLARES_ACME_DNS_PROVIDER", s.KubeConf.Arg.AcmeDNSProvider)
	}
	if s.KubeConf.Arg.TailscaleAuthKey != "" {
		os.Setenv("OLARES_TAILSCALE_AUTH_KEY", s.KubeConf.Arg.TailscaleAuthKey)
	}
	if s.KubeConf.Arg.TailscaleControlURL != "" {
		os.Setenv("OLARES_TAILSCALE_CONTROL_URL", s.KubeConf.Arg.TailscaleControlURL)
	}

	return nil
}

func (s *GetSetupConfig) getCertMode() (string, error) {
	// Check env var first
	if v := os.Getenv("OLARES_CERT_MODE"); v != "" {
		return strings.ToLower(strings.TrimSpace(v)), nil
	}

	reader, err := utils.GetBufIOReaderOfTerminalInput()
	if err != nil {
		return "local", nil // non-interactive, default to local
	}

	fmt.Println("\n--- SSL Certificate ---")
	fmt.Println("  1) local  — Self-signed (for Tailscale/LAN, trust CA once on your devices)")
	fmt.Println("  2) acme   — Let's Encrypt (real trusted cert, needs your own domain)")
	fmt.Printf("\nChoose cert mode [1]: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "2", "acme":
		return "acme", nil
	default:
		return "local", nil
	}
}

func (s *GetSetupConfig) getAcmeEmail() (string, error) {
	if v := os.Getenv("OLARES_ACME_EMAIL"); v != "" {
		return v, nil
	}

	reader, err := utils.GetBufIOReaderOfTerminalInput()
	if err != nil {
		return "", fmt.Errorf("OLARES_ACME_EMAIL is required for ACME mode")
	}

	fmt.Printf("Enter email for Let's Encrypt: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("email is required for ACME cert mode")
	}
	return input, nil
}

func (s *GetSetupConfig) getAcmeDNSProvider() (string, error) {
	if v := os.Getenv("OLARES_ACME_DNS_PROVIDER"); v != "" {
		return v, nil
	}

	reader, err := utils.GetBufIOReaderOfTerminalInput()
	if err != nil {
		return "", fmt.Errorf("OLARES_ACME_DNS_PROVIDER is required for ACME mode")
	}

	fmt.Printf("Enter DNS provider (cloudflare, route53, duckdns, etc.): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("DNS provider is required for ACME cert mode")
	}
	return input, nil
}

func (s *GetSetupConfig) getTailscaleAuthKey() (string, error) {
	if v := os.Getenv("OLARES_TAILSCALE_AUTH_KEY"); v != "" {
		return v, nil
	}

	reader, err := utils.GetBufIOReaderOfTerminalInput()
	if err != nil {
		return "", nil // non-interactive, skip
	}

	fmt.Println("\n--- Tailscale VPN (optional) ---")
	fmt.Println("  Enables remote access from anywhere via your Tailscale account.")
	fmt.Println("  Get an auth key at: https://login.tailscale.com/admin/settings/keys")
	fmt.Printf("\nEnter Tailscale auth key (or press Enter to skip): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	return input, nil
}

func (s *GetSetupConfig) getTailscaleControlURL() (string, error) {
	if v := os.Getenv("OLARES_TAILSCALE_CONTROL_URL"); v != "" {
		return v, nil
	}

	reader, err := utils.GetBufIOReaderOfTerminalInput()
	if err != nil {
		return "", nil
	}

	fmt.Printf("Tailscale control URL (Enter for default tailscale.com, or your Headscale URL): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	return input, nil
}

// SetupConfigModule adds setup prompts before account installation.
type SetupConfigModule struct {
	common.KubeModule
}

func (m *SetupConfigModule) Init() {
	logger.InfoInstallationProgress("Configuring system ...")
	m.Name = "SetupConfig"

	getSetupConfig := &task.LocalTask{
		Name:   "GetSetupConfig",
		Action: new(GetSetupConfig),
	}

	m.Tasks = []task.Interface{
		getSetupConfig,
	}
}
