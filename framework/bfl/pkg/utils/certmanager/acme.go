package certmanager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bytetrade.io/web3os/bfl/pkg/constants"

	"k8s.io/klog/v2"
)

// acmeCertManager uses Let's Encrypt (ACME) to obtain real trusted certificates.
// Requires:
//   - OLARES_ACME_EMAIL: email for Let's Encrypt registration
//   - OLARES_ACME_DNS_PROVIDER: DNS provider for DNS-01 challenge (e.g., cloudflare, route53)
//   - Provider-specific env vars (e.g., CF_DNS_API_TOKEN for Cloudflare)
//
// Uses the 'lego' CLI tool if available, falls back to 'certbot'.
type acmeCertManager struct {
	terminusName constants.TerminusName
}

var _ Interface = &acmeCertManager{}

func newACMECertManager(terminusName constants.TerminusName) Interface {
	return &acmeCertManager{terminusName: terminusName}
}

func (c *acmeCertManager) GenerateCert() error {
	zone := c.terminusName.UserZone()
	email := os.Getenv("OLARES_ACME_EMAIL")
	provider := os.Getenv("OLARES_ACME_DNS_PROVIDER")

	if email == "" {
		return fmt.Errorf("OLARES_ACME_EMAIL is required for ACME cert mode")
	}
	if provider == "" {
		return fmt.Errorf("OLARES_ACME_DNS_PROVIDER is required for ACME cert mode (e.g., cloudflare, route53, duckdns)")
	}

	certDir := acmeCertDir()
	os.MkdirAll(certDir, 0700)

	// Try lego first (lightweight Go ACME client), then certbot
	if legoPath, err := exec.LookPath("lego"); err == nil {
		return c.runLego(legoPath, zone, email, provider, certDir)
	}
	if certbotPath, err := exec.LookPath("certbot"); err == nil {
		return c.runCertbot(certbotPath, zone, email, provider, certDir)
	}

	return fmt.Errorf("ACME cert mode requires 'lego' or 'certbot' to be installed. " +
		"Install lego: https://go-acme.github.io/lego/ or certbot: https://certbot.eff.org/")
}

func (c *acmeCertManager) runLego(legoPath, zone, email, provider, certDir string) error {
	klog.Infof("ACME: requesting cert for *.%s via lego (provider: %s)", zone, provider)

	args := []string{
		"--accept-tos",
		"--email", email,
		"--dns", provider,
		"--domains", "*." + zone,
		"--domains", zone,
		"--path", certDir,
		"run",
	}

	cmd := exec.Command(legoPath, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("LEGO_PATH=%s", certDir))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lego failed: %w", err)
	}
	return nil
}

func (c *acmeCertManager) runCertbot(certbotPath, zone, email, provider, certDir string) error {
	klog.Infof("ACME: requesting cert for *.%s via certbot (provider: %s)", zone, provider)

	args := []string{
		"certonly",
		"--non-interactive",
		"--agree-tos",
		"--email", email,
		"--dns-" + provider,
		"-d", "*." + zone,
		"-d", zone,
		"--config-dir", filepath.Join(certDir, "certbot"),
		"--work-dir", filepath.Join(certDir, "work"),
		"--logs-dir", filepath.Join(certDir, "logs"),
	}

	cmd := exec.Command(certbotPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("certbot failed: %w", err)
	}
	return nil
}

func (c *acmeCertManager) DownloadCert() (*ResponseCert, error) {
	zone := c.terminusName.UserZone()
	certDir := acmeCertDir()

	// Try lego output paths first
	legoCertDir := filepath.Join(certDir, "certificates")
	certFile := filepath.Join(legoCertDir, "_."+zone+".crt")
	keyFile := filepath.Join(legoCertDir, "_."+zone+".key")

	if _, err := os.Stat(certFile); err != nil {
		// Try certbot output paths
		certbotLive := filepath.Join(certDir, "certbot", "live", zone)
		certFile = filepath.Join(certbotLive, "fullchain.pem")
		keyFile = filepath.Join(certbotLive, "privkey.pem")
	}

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("read cert file %s: %w", certFile, err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read key file %s: %w", keyFile, err)
	}

	// Let's Encrypt certs are valid for 90 days
	expiry := time.Now().AddDate(0, 0, 90)

	klog.Infof("ACME: loaded cert for *.%s from %s", zone, certFile)

	return &ResponseCert{
		Zone:      zone,
		Cert:      string(certPEM),
		Key:       string(keyPEM),
		ExpiredAt: expiry.Format(CertExpiredDateTimeLayout),
	}, nil
}

func (c *acmeCertManager) AddDNSRecord(publicIP, domain *string) error {
	klog.Info("ACME cert manager: DNS record management disabled (manage DNS externally)")
	return nil
}

func (c *acmeCertManager) DeleteDNSRecord() error {
	return nil
}

func (c *acmeCertManager) AddCustomDomainOnCloudflare(customDomain string) (*ResponseCustomDomainStatus, error) {
	return &ResponseCustomDomainStatus{}, nil
}

func (c *acmeCertManager) GetCustomDomainOnCloudflare(customDomain string) (*ResponseCustomDomainStatus, error) {
	return &ResponseCustomDomainStatus{}, nil
}

func (c *acmeCertManager) GetCustomDomainCnameStatus(customDomain string) (*Response, error) {
	return &Response{Success: true}, nil
}

func (c *acmeCertManager) DeleteCustomDomainOnCloudflare(customDomain string) (*Response, error) {
	return &Response{Success: true}, nil
}

func (c *acmeCertManager) GetCustomDomainErrorStatus(err error) string {
	if err != nil && strings.Contains(err.Error(), "not found") {
		return constants.CustomDomainCnameStatusNone
	}
	return constants.CustomDomainCnameStatusError
}

func acmeCertDir() string {
	if v := os.Getenv("OLARES_ACME_CERT_DIR"); v != "" {
		return v
	}
	return "/olares/data/certs/acme"
}
