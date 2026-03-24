package phases

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/packalares/packalares/pkg/config"
)

// generateTLSCert creates a self-signed TLS certificate and stores it as a
// K8s Secret in the framework namespace. This must run before waitForAllPods
// so the proxy pod can mount the secret.
func generateTLSCert(opts *InstallOptions) error {
	certDir := "/etc/packalares/ssl"
	os.MkdirAll(certDir, 0755)

	certFile := certDir + "/tls.crt"
	keyFile := certDir + "/tls.key"

	zone := config.UserZone()
	ns := config.FrameworkNamespace()
	secretName := config.TLSSecretName()

	// Generate self-signed cert
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		fmt.Printf("  Generating self-signed TLS certificate for *.%s ...\n", zone)

		cmd := exec.Command("openssl", "req", "-x509", "-nodes", "-days", "3650",
			"-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
			"-keyout", keyFile,
			"-out", certFile,
			"-subj", fmt.Sprintf("/CN=*.%s/O=Packalares", zone),
			"-addext", fmt.Sprintf("subjectAltName=DNS:%s,DNS:*.%s,DNS:localhost,IP:127.0.0.1,IP:::1", zone, zone),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("openssl: %s\n%w", string(out), err)
		}

		os.Chmod(keyFile, 0600)
		os.Chmod(certFile, 0644)
	} else {
		fmt.Println("  TLS certificate already exists")
	}

	// Create K8s TLS Secret
	fmt.Printf("  Creating K8s Secret %s in %s ...\n", secretName, ns)
	cmd := exec.Command("kubectl", "create", "secret", "tls", secretName,
		"--cert="+certFile,
		"--key="+keyFile,
		"-n", ns,
		"--dry-run=client", "-o", "yaml")
	yamlOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("create secret yaml: %w", err)
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Stdin = bytes.NewReader(yamlOut)
	if out, err := applyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply tls secret: %s\n%w", string(out), err)
	}

	fmt.Printf("  TLS certificate ready for *.%s\n", zone)
	return nil
}
