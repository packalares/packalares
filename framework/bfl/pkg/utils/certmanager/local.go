package certmanager

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"bytetrade.io/web3os/bfl/pkg/constants"

	"k8s.io/klog/v2"
)

// localCertManager generates self-signed wildcard certificates locally
// instead of downloading them from api.olares.com.
// Activated when OLARES_CERT_MODE=local (or when OLARES_SYSTEM_REMOTE_SERVICE is empty).
type localCertManager struct {
	terminusName constants.TerminusName
}

var _ Interface = &localCertManager{}

func newLocalCertManager(terminusName constants.TerminusName) Interface {
	return &localCertManager{terminusName: terminusName}
}

func (c *localCertManager) GenerateCert() error {
	klog.Info("local cert manager: skipping remote cert generation")
	return nil
}

func (c *localCertManager) DownloadCert() (*ResponseCert, error) {
	zone := c.terminusName.UserZone()
	klog.Infof("local cert manager: generating self-signed cert for *.%s", zone)

	certPEM, keyPEM, err := generateSelfSignedCert(zone, c.terminusName)
	if err != nil {
		return nil, fmt.Errorf("generate self-signed cert: %w", err)
	}

	expiry := time.Now().AddDate(10, 0, 0) // 10 year validity

	return &ResponseCert{
		Zone:      zone,
		Cert:      certPEM,
		Key:       keyPEM,
		ExpiredAt: expiry.Format(CertExpiredDateTimeLayout),
	}, nil
}

func (c *localCertManager) AddDNSRecord(publicIP, domain *string) error {
	klog.Info("local cert manager: DNS record management disabled (using local DNS)")
	return nil
}

func (c *localCertManager) DeleteDNSRecord() error {
	klog.Info("local cert manager: DNS record management disabled (using local DNS)")
	return nil
}

func (c *localCertManager) AddCustomDomainOnCloudflare(customDomain string) (*ResponseCustomDomainStatus, error) {
	klog.Info("local cert manager: Cloudflare custom domain disabled")
	return &ResponseCustomDomainStatus{}, nil
}

func (c *localCertManager) GetCustomDomainOnCloudflare(customDomain string) (*ResponseCustomDomainStatus, error) {
	return &ResponseCustomDomainStatus{}, nil
}

func (c *localCertManager) GetCustomDomainCnameStatus(customDomain string) (*Response, error) {
	return &Response{Success: true}, nil
}

func (c *localCertManager) DeleteCustomDomainOnCloudflare(customDomain string) (*Response, error) {
	return &Response{Success: true}, nil
}

func (c *localCertManager) GetCustomDomainErrorStatus(err error) string {
	return constants.CustomDomainCnameStatusNone
}

// CertMode returns the configured certificate mode: "local" or "acme".
// Defaults to "local" (self-signed).
func CertMode() string {
	if mode := os.Getenv("OLARES_CERT_MODE"); strings.EqualFold(strings.TrimSpace(mode), "acme") {
		return "acme"
	}
	return "local"
}

// IsLocalCertMode always returns true — this fork never calls api.olares.com.
func IsLocalCertMode() bool {
	return true
}

// generateSelfSignedCert creates a self-signed CA and wildcard cert for the given zone.
// Returns PEM-encoded cert and key.
func generateSelfSignedCert(zone string, terminusName constants.TerminusName) (certPEM, keyPEM string, err error) {
	// Generate CA key
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Olares Local CA"},
			CommonName:   "Olares Local CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("create CA cert: %w", err)
	}

	// Generate server key
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate server key: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Olares Local"},
			CommonName:   "*." + zone,
		},
		DNSNames: []string{
			zone,
			"*." + zone,
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().AddDate(10, 0, 0),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	// Also add common local IPs as SANs
	for _, ip := range []string{"127.0.0.1", "::1"} {
		serverTemplate.IPAddresses = append(serverTemplate.IPAddresses, net.ParseIP(ip))
	}

	// Also add local domain wildcard if configured
	localDomain := os.Getenv("OLARES_LOCAL_DOMAIN")
	if localDomain != "" && localDomain != zone {
		username := terminusName.UserName()
		serverTemplate.DNSNames = append(serverTemplate.DNSNames,
			username+"."+localDomain,
			"*."+username+"."+localDomain,
		)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return "", "", fmt.Errorf("parse CA cert: %w", err)
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("create server cert: %w", err)
	}

	// Encode cert chain (server + CA)
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})) +
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER}))

	// Encode server key
	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal server key: %w", err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}))

	return certPEM, keyPEM, nil
}
