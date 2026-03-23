package phases

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/packalares/packalares/pkg/config"
	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultDomain  = "olares.local"
	DefaultBaseDir = "/opt/packalares"

	ReleaseFile = "/etc/packalares/release"

	BinDir           = "/usr/local/bin"
	ETCDCertDir      = "/etc/ssl/etcd/ssl"
	KubeConfigDir    = "/etc/kubernetes"
	ContainerdCfgDir = "/etc/containerd"
	SystemdDir       = "/etc/systemd/system"
)

type InstallOptions struct {
	Username            string
	Password            string
	Domain              string
	BaseDir             string
	Registry            string
	CertMode            string
	AcmeEmail           string
	AcmeDNSProvider     string
	TailscaleAuthKey    string
	TailscaleControlURL string
	SkipPrecheck        bool
}

func (o *InstallOptions) applyDefaults() {
	if o.Domain == "" {
		o.Domain = os.Getenv("PACKALARES_DOMAIN")
	}
	if o.Domain == "" {
		o.Domain = DefaultDomain
	}
	if o.BaseDir == "" {
		o.BaseDir = os.Getenv("PACKALARES_BASE_DIR")
	}
	if o.BaseDir == "" {
		o.BaseDir = DefaultBaseDir
	}
	if o.Registry == "" {
		o.Registry = os.Getenv("PACKALARES_REGISTRY")
	}
	if o.CertMode == "" {
		o.CertMode = os.Getenv("OLARES_CERT_MODE")
	}
	if o.CertMode == "" {
		o.CertMode = "local"
	}
	if o.AcmeEmail == "" {
		o.AcmeEmail = os.Getenv("OLARES_ACME_EMAIL")
	}
	if o.AcmeDNSProvider == "" {
		o.AcmeDNSProvider = os.Getenv("OLARES_ACME_DNS_PROVIDER")
	}
	if o.TailscaleAuthKey == "" {
		o.TailscaleAuthKey = os.Getenv("OLARES_TAILSCALE_AUTH_KEY")
	}
	if o.TailscaleControlURL == "" {
		o.TailscaleControlURL = os.Getenv("OLARES_TAILSCALE_CONTROL_URL")
	}
	if o.Username == "" {
		o.Username = os.Getenv("PACKALARES_USERNAME")
	}
	if o.Password == "" {
		o.Password = os.Getenv("PACKALARES_PASSWORD")
	}
}

func (o *InstallOptions) validate() error {
	if o.CertMode == "acme" {
		if o.AcmeEmail == "" {
			return fmt.Errorf("--acme-email is required when cert-mode=acme")
		}
		if o.AcmeDNSProvider == "" {
			return fmt.Errorf("--acme-dns-provider is required when cert-mode=acme")
		}
		if o.Domain == DefaultDomain {
			return fmt.Errorf("a real domain is required for cert-mode=acme (not %s)", DefaultDomain)
		}
	}
	return nil
}

func (o *InstallOptions) resolvedDirs() (installerDir, wizardDir string) {
	installerDir = filepath.Join(o.BaseDir, "installer")
	wizardDir = filepath.Join(installerDir, "wizard")
	return
}

func generatePassword(length int) (plain string, hashed string, err error) {
	b := make([]byte, length)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate random password: %w", err)
	}
	plain = hex.EncodeToString(b)[:length]

	// Generate random salt (not a fixed one like olares's @Olares2025)
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return "", "", fmt.Errorf("generate salt: %w", err)
	}
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("hash password: %w", err)
	}
	hashed = string(hashBytes)
	return plain, hashed, nil
}

func getArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

// writeConfigYAML reads config.yaml.template, replaces placeholders with
// installer options, and writes to /etc/packalares/config.yaml.
// Must run before any phase that uses config.*() functions.
func writeConfigYAML(opts *InstallOptions) error {
	content := config.ConfigTemplate

	tailscaleEnabled := "false"
	if opts.TailscaleAuthKey != "" {
		tailscaleEnabled = "true"
	}

	replacements := map[string]string{
		"{{DOMAIN}}":            opts.Domain,
		"{{USERNAME}}":          opts.Username,
		"{{TAILSCALE_ENABLED}}": tailscaleEnabled,
		"{{TAILSCALE_AUTH_KEY}}": opts.TailscaleAuthKey,
	}

	for placeholder, value := range replacements {
		content = strings.ReplaceAll(content, placeholder, value)
	}

	return os.WriteFile("/etc/packalares/config.yaml", []byte(content), 0600)
}

func registryImage(registry, image string) string {
	if registry == "" {
		return image
	}
	// Replace the registry portion
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 {
		return registry + "/" + parts[1]
	}
	return registry + "/" + image
}
