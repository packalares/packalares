package phases

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/packalares/packalares/pkg/config"
)

// generateTLSCert creates a CA and signs a wildcard TLS certificate, then
// stores it as a K8s Secret. The CA cert is also stored as a ConfigMap so
// users can download and install it for browser trust.
func generateTLSCert(opts *InstallOptions, w io.Writer) error {
	certDir := "/etc/packalares/ssl"
	os.MkdirAll(certDir, 0755)

	caKeyFile := certDir + "/ca.key"
	caCertFile := certDir + "/ca.crt"
	certFile := certDir + "/tls.crt"
	keyFile := certDir + "/tls.key"
	csrFile := certDir + "/tls.csr"

	zone := config.UserZone()
	ns := config.FrameworkNamespace()
	secretName := config.TLSSecretName()
	serverIP := os.Getenv("SERVER_IP")

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		fmt.Fprintf(w, "  Generating Packalares CA and TLS certificate for *.%s ...\n", zone)

		// Step 1: Generate CA key + cert
		cmd := exec.Command("openssl", "req", "-x509", "-nodes", "-days", "3650",
			"-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
			"-keyout", caKeyFile,
			"-out", caCertFile,
			"-subj", "/CN=Packalares CA/O=Packalares",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("openssl ca: %s\n%w", string(out), err)
		}

		// Step 2: Generate server key + CSR
		cmd = exec.Command("openssl", "req", "-nodes", "-newkey", "ec",
			"-pkeyopt", "ec_paramgen_curve:prime256v1",
			"-keyout", keyFile,
			"-out", csrFile,
			"-subj", fmt.Sprintf("/CN=*.%s/O=Packalares", zone),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("openssl csr: %s\n%w", string(out), err)
		}

		// Step 3: Build SAN extension file
		sanConf := certDir + "/san.cnf"
		sanContent := fmt.Sprintf("[v3_ext]\nsubjectAltName=DNS:%s,DNS:*.%s,DNS:localhost,IP:127.0.0.1,IP:::1", zone, zone)
		if serverIP != "" {
			sanContent += fmt.Sprintf(",IP:%s", serverIP)
		}
		sanContent += "\nbasicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n"
		os.WriteFile(sanConf, []byte(sanContent), 0644)

		// Step 4: Sign with CA
		cmd = exec.Command("openssl", "x509", "-req", "-days", "3650",
			"-in", csrFile,
			"-CA", caCertFile,
			"-CAkey", caKeyFile,
			"-CAcreateserial",
			"-out", certFile,
			"-extfile", sanConf,
			"-extensions", "v3_ext",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("openssl sign: %s\n%w", string(out), err)
		}

		os.Chmod(keyFile, 0600)
		os.Chmod(certFile, 0644)
		os.Chmod(caCertFile, 0644)

		// Cleanup temp files
		os.Remove(csrFile)
		os.Remove(sanConf)
		os.Remove(certDir + "/ca.srl")
	} else {
		fmt.Fprintln(w, "  TLS certificate already exists")
		// Ensure CA files exist (may have been lost during cleanup)
		if _, err := os.Stat(caCertFile); os.IsNotExist(err) {
			fmt.Fprintln(w, "  CA certificate missing, regenerating ...")
			cmd := exec.Command("openssl", "req", "-x509", "-nodes", "-days", "3650",
				"-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1",
				"-keyout", caKeyFile,
				"-out", caCertFile,
				"-subj", "/CN=Packalares CA/O=Packalares",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(w, "  Warning: CA regeneration failed: %s %v\n", string(out), err)
			}
		}
	}

	// Create K8s TLS Secret
	fmt.Fprintf(w, "  Creating K8s Secret %s in %s ...\n", secretName, ns)
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

	// Create ConfigMap with CA cert for user download
	if _, err := os.Stat(caCertFile); err == nil {
		caData, _ := os.ReadFile(caCertFile)
		if len(caData) > 0 {
			caCmd := exec.Command("kubectl", "create", "configmap", "packalares-ca",
				"--from-file=ca.crt="+caCertFile,
				"-n", ns,
				"--dry-run=client", "-o", "yaml")
			caYaml, err := caCmd.Output()
			if err == nil {
				caApply := exec.Command("kubectl", "apply", "-f", "-")
				caApply.Stdin = bytes.NewReader(caYaml)
				caApply.CombinedOutput()
			}
		}
	}

	// Store CA key as K8s Secret so BFL can regenerate certs (e.g. after Tailscale enable)
	if _, err := os.Stat(caKeyFile); err == nil {
		caKeyCmd := exec.Command("kubectl", "create", "secret", "generic", "packalares-ca-key",
			"--from-file=ca.key="+caKeyFile,
			"-n", ns,
			"--dry-run=client", "-o", "yaml")
		caKeyYaml, err := caKeyCmd.Output()
		if err == nil {
			caKeyApply := exec.Command("kubectl", "apply", "-f", "-")
			caKeyApply.Stdin = bytes.NewReader(caKeyYaml)
			caKeyApply.CombinedOutput()
		}
	}

	fmt.Fprintf(w, "  TLS certificate ready for *.%s\n", zone)
	return nil
}
