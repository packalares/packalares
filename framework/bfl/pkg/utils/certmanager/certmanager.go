package certmanager

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"bytetrade.io/web3os/bfl/pkg/constants"

	"k8s.io/klog/v2"
)

type Interface interface {
	GenerateCert() error
	DownloadCert() (*ResponseCert, error)
	AddDNSRecord(publicIP, domain *string) error
	DeleteDNSRecord() error
	AddCustomDomainOnCloudflare(customDomain string) (*ResponseCustomDomainStatus, error)
	GetCustomDomainOnCloudflare(customDomain string) (*ResponseCustomDomainStatus, error)
	GetCustomDomainCnameStatus(customDomain string) (*Response, error)
	DeleteCustomDomainOnCloudflare(customDomain string) (*Response, error)
	GetCustomDomainErrorStatus(err error) string
}

func NewCertManager(terminusName constants.TerminusName) Interface {
	switch CertMode() {
	case "acme":
		klog.Info("using ACME cert manager (Let's Encrypt)")
		return newACMECertManager(terminusName)
	default:
		klog.Info("using local cert manager (self-signed certificates)")
		return newLocalCertManager(terminusName)
	}
}

func ValidPemKey(key string) error {
	keyPemBlock, _ := pem.Decode([]byte(key))
	if keyPemBlock == nil {
		return fmt.Errorf("not pem format key")
	}

	_, err := x509.ParsePKCS8PrivateKey(keyPemBlock.Bytes)
	if err != nil {
		_, err = x509.ParsePKCS1PrivateKey(keyPemBlock.Bytes)
		if err != nil {
			_, err = x509.ParseECPrivateKey(keyPemBlock.Bytes)
			if err != nil {
				return fmt.Errorf("parse pkcs private key error %v", err)
			}
		}
	}

	return nil
}

func ValidPemCert(cert string) error {
	pemBlock, _ := pem.Decode([]byte(cert))
	if pemBlock == nil {
		return fmt.Errorf("not pem format cert")
	}

	certs, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse cert error %v", err)
	}

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM([]byte(cert))
	opts := x509.VerifyOptions{
		Roots: roots,
	}

	_, err = certs.Verify(opts)
	if err != nil {
		return fmt.Errorf("verify cert error %v", err)
	}

	return nil
}
