package etcd

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
	"os/exec"
	"time"
)

const (
	certDir     = "/etc/ssl/etcd/ssl"
	dataDir     = "/var/lib/etcd"
	etcdService = `[Unit]
Description=etcd key-value store
Documentation=https://etcd.io/docs/
After=network.target

[Service]
Type=notify
ExecStart=/usr/local/bin/etcd \
  --name=packalares-etcd \
  --data-dir=/var/lib/etcd \
  --listen-client-urls=https://0.0.0.0:2379 \
  --advertise-client-urls=https://127.0.0.1:2379 \
  --listen-peer-urls=https://0.0.0.0:2380 \
  --initial-advertise-peer-urls=https://127.0.0.1:2380 \
  --initial-cluster=packalares-etcd=https://127.0.0.1:2380 \
  --initial-cluster-token=packalares-etcd-token \
  --initial-cluster-state=new \
  --client-cert-auth=true \
  --trusted-ca-file=/etc/ssl/etcd/ssl/ca.pem \
  --cert-file=/etc/ssl/etcd/ssl/server.pem \
  --key-file=/etc/ssl/etcd/ssl/server-key.pem \
  --peer-client-cert-auth=true \
  --peer-trusted-ca-file=/etc/ssl/etcd/ssl/ca.pem \
  --peer-cert-file=/etc/ssl/etcd/ssl/peer.pem \
  --peer-key-file=/etc/ssl/etcd/ssl/peer-key.pem
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`
)

func Install(baseDir string) error {
	// Verify binary exists
	if _, err := os.Stat("/usr/local/bin/etcd"); os.IsNotExist(err) {
		return fmt.Errorf("etcd binary not found — run download phase first")
	}

	// Create directories
	for _, d := range []string{certDir, dataDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	// Generate TLS certificates
	fmt.Println("  Generating etcd TLS certificates ...")
	if err := generateCerts(); err != nil {
		return fmt.Errorf("generate etcd certs: %w", err)
	}

	// Write systemd unit
	if err := os.WriteFile("/etc/systemd/system/etcd.service", []byte(etcdService), 0644); err != nil {
		return fmt.Errorf("write etcd service: %w", err)
	}

	// Enable and start
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "etcd"},
		{"systemctl", "start", "etcd"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("run %v: %s\n%w", args, string(out), err)
		}
	}

	// Wait for etcd to be healthy
	fmt.Println("  Waiting for etcd to be healthy ...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("/usr/local/bin/etcdctl",
			"--cacert", certDir+"/ca.pem",
			"--cert", certDir+"/server.pem",
			"--key", certDir+"/server-key.pem",
			"--endpoints", "https://127.0.0.1:2379",
			"endpoint", "health",
		)
		if err := cmd.Run(); err == nil {
			fmt.Println("  etcd is healthy")
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("etcd did not become healthy in time")
}

func generateCerts() error {
	// Generate CA
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Packalares"},
			CommonName:   "Packalares etcd CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create CA cert: %w", err)
	}
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return fmt.Errorf("parse CA cert: %w", err)
	}

	// Write CA cert
	if err := writePEM(certDir+"/ca.pem", "CERTIFICATE", caCertDER); err != nil {
		return err
	}
	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		return fmt.Errorf("marshal CA key: %w", err)
	}
	if err := writePEM(certDir+"/ca-key.pem", "EC PRIVATE KEY", caKeyDER); err != nil {
		return err
	}

	// Generate server cert
	if err := generateNodeCert(caCert, caKey, "server", certDir); err != nil {
		return err
	}

	// Generate peer cert
	if err := generateNodeCert(caCert, caKey, "peer", certDir); err != nil {
		return err
	}

	return nil
}

func generateNodeCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, name, dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate %s key: %w", name, err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"Packalares"},
			CommonName:   "packalares-etcd-" + name,
		},
		DNSNames: []string{
			"localhost",
			"packalares-etcd",
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
			x509.ExtKeyUsageClientAuth,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create %s cert: %w", name, err)
	}

	if err := writePEM(dir+"/"+name+".pem", "CERTIFICATE", certDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal %s key: %w", name, err)
	}
	if err := writePEM(dir+"/"+name+"-key.pem", "EC PRIVATE KEY", keyDER); err != nil {
		return err
	}

	return nil
}

func writePEM(path, pemType string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: pemType, Bytes: data})
}
