package bfl

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
	"time"
)

// GenerateSelfSignedCert creates an ECDSA P-256 self-signed CA + wildcard
// server certificate for the given zone. Returns PEM-encoded cert chain and
// key. Validity: 10 years. The cert covers *.zone and zone itself, plus
// localhost IPs and an optional OLARES_LOCAL_DOMAIN.
func GenerateSelfSignedCert(zone string, terminusName TerminusName) (certPEM, keyPEM string, err error) {
	// --- CA ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Packalares Local CA"},
			CommonName:   "Packalares Local CA",
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

	// --- Server ---
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate server key: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Packalares Local"},
			CommonName:   "*." + zone,
		},
		DNSNames: []string{
			zone,
			"*." + zone,
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().AddDate(10, 0, 0),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	// Optional local domain SANs
	if localDomain := os.Getenv("OLARES_LOCAL_DOMAIN"); localDomain != "" && localDomain != zone {
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

	// Cert chain: server + CA
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})) +
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER}))

	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal server key: %w", err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}))

	return certPEM, keyPEM, nil
}
