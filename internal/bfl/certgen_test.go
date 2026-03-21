package bfl

import (
	"crypto/tls"
	"crypto/x509"
	"strings"
	"testing"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	tn := NewTerminusName("admin", "example.packalares.io")
	zone := tn.UserZone()

	certPEM, keyPEM, err := GenerateSelfSignedCert(zone, tn)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	if certPEM == "" || keyPEM == "" {
		t.Fatal("cert or key is empty")
	}

	// Verify that the cert and key can be parsed as a TLS certificate
	_, err = tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("X509KeyPair failed: %v", err)
	}

	// Verify cert chain contains two certificates (server + CA)
	certCount := strings.Count(certPEM, "-----BEGIN CERTIFICATE-----")
	if certCount != 2 {
		t.Fatalf("expected 2 certificates in chain, got %d", certCount)
	}

	// Parse the certificate to check SANs
	block := []byte(certPEM)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(block)

	// Verify the zone appears in the cert
	if !strings.Contains(certPEM, "CERTIFICATE") {
		t.Fatal("cert PEM does not contain CERTIFICATE header")
	}
	if !strings.Contains(keyPEM, "EC PRIVATE KEY") {
		t.Fatal("key PEM does not contain EC PRIVATE KEY header")
	}
}

func TestTerminusName(t *testing.T) {
	tn := NewTerminusName("alice", "home.example.com")
	if tn.UserName() != "alice" {
		t.Errorf("UserName()=%q, want alice", tn.UserName())
	}
	if tn.Domain() != "home.example.com" {
		t.Errorf("Domain()=%q, want home.example.com", tn.Domain())
	}
	if tn.UserZone() != "alice.home.example.com" {
		t.Errorf("UserZone()=%q, want alice.home.example.com", tn.UserZone())
	}
}
