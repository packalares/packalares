package bfl

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// regenerateTLSCert regenerates the zone TLS certificate with all current SANs.
// It reads the CA key and cert from the packalares-ca ConfigMap / secret,
// generates a new CSR, builds the SAN extension, signs with CA (10-year validity),
// and updates the zone-tls K8s Secret.
func (s *Server) regenerateTLSCert(ctx context.Context, serverIP, tailscaleIP, zone, customDomain string) error {
	ns := config.FrameworkNamespace()
	tlsSecretName := config.TLSSecretName()

	// Read CA files from disk first (installer writes them to /etc/packalares/ssl/).
	// If not on disk, try to read from K8s ConfigMap / Secret.
	certDir, err := os.MkdirTemp("", "packalares-tls-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(certDir)

	caKeyFile := certDir + "/ca.key"
	caCertFile := certDir + "/ca.crt"
	keyFile := certDir + "/tls.key"
	csrFile := certDir + "/tls.csr"
	certFile := certDir + "/tls.crt"
	sanConf := certDir + "/san.cnf"

	// Try /etc/packalares/ssl first
	caKeyData, caKeyErr := os.ReadFile("/etc/packalares/ssl/ca.key")
	caCertData, caCertErr := os.ReadFile("/etc/packalares/ssl/ca.crt")

	if caKeyErr != nil || caCertErr != nil {
		// Try K8s secret packalares-ca-key in framework namespace
		caSecret, err := s.K8s.Clientset.CoreV1().Secrets(ns).Get(ctx, "packalares-ca-key", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("CA key not found on disk or in K8s secret packalares-ca-key: %w", err)
		}
		caKeyData = caSecret.Data["ca.key"]
		if len(caKeyData) == 0 {
			return fmt.Errorf("CA key is empty in secret packalares-ca-key")
		}

		// CA cert can be in ConfigMap packalares-ca
		caCM, cmErr := s.K8s.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, "packalares-ca", metav1.GetOptions{})
		if cmErr != nil {
			return fmt.Errorf("CA cert ConfigMap packalares-ca not found: %w", cmErr)
		}
		caCertData = []byte(caCM.Data["ca.crt"])
		if len(caCertData) == 0 {
			return fmt.Errorf("CA cert is empty in ConfigMap packalares-ca")
		}
	}

	if err := os.WriteFile(caKeyFile, caKeyData, 0600); err != nil {
		return fmt.Errorf("write CA key: %w", err)
	}
	if err := os.WriteFile(caCertFile, caCertData, 0644); err != nil {
		return fmt.Errorf("write CA cert: %w", err)
	}

	// Generate server key + CSR
	cmd := exec.CommandContext(ctx, "openssl", "req", "-nodes", "-newkey", "ec",
		"-pkeyopt", "ec_paramgen_curve:prime256v1",
		"-keyout", keyFile,
		"-out", csrFile,
		"-subj", fmt.Sprintf("/CN=*.%s/O=Packalares", zone),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("openssl csr: %s\n%w", string(out), err)
	}

	// Build SAN extension
	sans := []string{
		"DNS:" + zone,
		"DNS:*." + zone,
		"DNS:localhost",
		"IP:127.0.0.1",
		"IP:::1",
	}
	if serverIP != "" {
		sans = append(sans, "IP:"+serverIP)
	}
	if tailscaleIP != "" {
		sans = append(sans, "IP:"+tailscaleIP)
	}
	if customDomain != "" {
		sans = append(sans, "DNS:"+customDomain, "DNS:*."+customDomain)
	}

	sanContent := fmt.Sprintf(
		"[v3_ext]\nsubjectAltName=%s\nbasicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n",
		strings.Join(sans, ","),
	)
	if err := os.WriteFile(sanConf, []byte(sanContent), 0644); err != nil {
		return fmt.Errorf("write SAN config: %w", err)
	}

	// Sign with CA (10-year validity)
	cmd = exec.CommandContext(ctx, "openssl", "x509", "-req", "-days", "3650",
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

	// Read generated cert and key
	newCert, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("read cert: %w", err)
	}
	newKey, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}

	// Append CA cert to form chain (server + CA)
	certChain := string(newCert) + string(caCertData)

	// Update K8s TLS secret
	existing, err := s.K8s.Clientset.CoreV1().Secrets(ns).Get(ctx, tlsSecretName, metav1.GetOptions{})
	if err != nil {
		// Create new
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: tlsSecretName, Namespace: ns},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": []byte(certChain),
				"tls.key": newKey,
			},
		}
		if _, err := s.K8s.Clientset.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create TLS secret: %w", err)
		}
	} else {
		existing.Data["tls.crt"] = []byte(certChain)
		existing.Data["tls.key"] = newKey
		if _, err := s.K8s.Clientset.CoreV1().Secrets(ns).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update TLS secret: %w", err)
		}
	}

	// Also update the zone-ssl-config secret (used by BFL internally)
	expiry := time.Now().AddDate(10, 0, 0).Format("2006-01-02 15:04:05")
	sslData := map[string][]byte{
		"zone":       []byte(zone),
		"cert":       []byte(certChain),
		"key":        newKey,
		"expired_at": []byte(expiry),
	}
	sslSecret, sslErr := s.K8s.Clientset.CoreV1().Secrets(s.K8s.Namespace).Get(ctx, SSLSecretName, metav1.GetOptions{})
	if sslErr != nil {
		newSSL := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: SSLSecretName, Namespace: s.K8s.Namespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       sslData,
		}
		s.K8s.Clientset.CoreV1().Secrets(s.K8s.Namespace).Create(ctx, newSSL, metav1.CreateOptions{})
	} else {
		sslSecret.Data = sslData
		s.K8s.Clientset.CoreV1().Secrets(s.K8s.Namespace).Update(ctx, sslSecret, metav1.UpdateOptions{})
	}

	klog.Infof("TLS cert regenerated for zone=%s serverIP=%s tailscaleIP=%s customDomain=%s", zone, serverIP, tailscaleIP, customDomain)
	return nil
}

// updateNginxServerName patches the proxy-config ConfigMap to update
// the IP server block's server_name directive with the given list of IPs/hosts.
func (s *Server) updateNginxServerName(ctx context.Context, names []string) error {
	ns := config.FrameworkNamespace()

	cm, err := s.K8s.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, "proxy-config", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get proxy-config: %w", err)
	}

	conf := cm.Data["nginx.conf"]
	if conf == "" {
		return fmt.Errorf("nginx.conf is empty in proxy-config ConfigMap")
	}

	// Find and replace the server_name line in the IP ACCESS server block.
	// The IP block is the first server block (default_server).
	// Look for: server_name <something>;  after "listen 443 ssl default_server"
	idx := strings.Index(conf, "listen 443 ssl default_server")
	if idx < 0 {
		return fmt.Errorf("cannot find IP server block in nginx.conf")
	}

	// Find server_name after this point
	remaining := conf[idx:]
	snIdx := strings.Index(remaining, "server_name ")
	if snIdx < 0 {
		return fmt.Errorf("cannot find server_name in IP server block")
	}

	// Find the end of the server_name line (semicolon)
	lineStart := idx + snIdx
	endIdx := strings.Index(conf[lineStart:], ";")
	if endIdx < 0 {
		return fmt.Errorf("cannot find server_name terminator")
	}

	// Replace the server_name line
	newServerName := "server_name " + strings.Join(names, " ")
	conf = conf[:lineStart] + newServerName + conf[lineStart+endIdx:]

	cm.Data["nginx.conf"] = conf
	if _, err := s.K8s.Clientset.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update proxy-config: %w", err)
	}

	return nil
}

// restartProxy deletes proxy pods to trigger a reload with the new config/certs.
func (s *Server) restartProxy(ctx context.Context) {
	ns := config.FrameworkNamespace()
	pods, err := s.K8s.Clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app=proxy",
	})
	if err != nil {
		klog.Warningf("list proxy pods for restart: %v", err)
		return
	}
	for _, p := range pods.Items {
		_ = s.K8s.Clientset.CoreV1().Pods(ns).Delete(ctx, p.Name, metav1.DeleteOptions{})
	}
}

// getNodeIP returns the InternalIP of the first node.
func (s *Server) getNodeIP(ctx context.Context) string {
	nodes, err := s.K8s.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil || len(nodes.Items) == 0 {
		return ""
	}
	for _, addr := range nodes.Items[0].Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// getTailscaleIP attempts to get the Tailscale IP from the tailscale pod or host.
// Returns empty string if Tailscale is not available.
func (s *Server) getTailscaleIP(ctx context.Context) string {
	ns := config.FrameworkNamespace()

	// Try pod first
	_, depErr := s.K8s.Clientset.AppsV1().Deployments(ns).Get(ctx, "tailscale", metav1.GetOptions{})
	if depErr == nil {
		out, err := exec.CommandContext(ctx, "kubectl", "exec", "deploy/tailscale", "-n", ns, "--",
			"tailscale", "ip", "-4").Output()
		if err == nil {
			ip := strings.TrimSpace(string(out))
			if ip != "" {
				return ip
			}
		}
	}

	// Try host
	out, err := exec.CommandContext(ctx, "tailscale", "ip", "-4").Output()
	if err == nil {
		ip := strings.TrimSpace(string(out))
		if ip != "" {
			return ip
		}
	}

	return ""
}

// getTailscaleStatus returns parsed Tailscale status from pod or host.
func (s *Server) getTailscaleStatus(ctx context.Context) *TailscaleStatusResponse {
	ns := config.FrameworkNamespace()

	// Try pod first
	var statusJSON []byte
	_, depErr := s.K8s.Clientset.AppsV1().Deployments(ns).Get(ctx, "tailscale", metav1.GetOptions{})
	if depErr == nil {
		out, err := exec.CommandContext(ctx, "kubectl", "exec", "deploy/tailscale", "-n", ns, "--",
			"tailscale", "status", "--json").Output()
		if err == nil {
			statusJSON = out
		}
	}

	// Try host if pod didn't work
	if len(statusJSON) == 0 {
		out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
		if err == nil {
			statusJSON = out
		}
	}

	if len(statusJSON) == 0 {
		return nil
	}

	return parseTailscaleStatus(statusJSON)
}

// getCustomDomain reads the custom domain from the packalares-network ConfigMap.
func (s *Server) getCustomDomain(ctx context.Context) string {
	ns := config.FrameworkNamespace()
	cm, err := s.K8s.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, "packalares-network", metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return cm.Data["custom_domain"]
}

// setCustomDomain saves the custom domain to the packalares-network ConfigMap.
func (s *Server) setCustomDomain(ctx context.Context, domain string) error {
	ns := config.FrameworkNamespace()
	cm, err := s.K8s.Clientset.CoreV1().ConfigMaps(ns).Get(ctx, "packalares-network", metav1.GetOptions{})
	if err != nil {
		// Create
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "packalares-network", Namespace: ns},
			Data:       map[string]string{"custom_domain": domain},
		}
		_, err = s.K8s.Clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["custom_domain"] = domain
	_, err = s.K8s.Clientset.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// getCertInfo reads the TLS secret and parses the certificate for SANs and expiry.
func (s *Server) getCertInfo(ctx context.Context) (sans []string, expiry string) {
	ns := config.FrameworkNamespace()
	tlsSecretName := config.TLSSecretName()

	secret, err := s.K8s.Clientset.CoreV1().Secrets(ns).Get(ctx, tlsSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, ""
	}

	certData := secret.Data["tls.crt"]
	if len(certData) == 0 {
		return nil, ""
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, ""
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, ""
	}

	for _, dns := range cert.DNSNames {
		sans = append(sans, dns)
	}
	for _, ip := range cert.IPAddresses {
		sans = append(sans, ip.String())
	}

	expiry = cert.NotAfter.Format(time.RFC3339)
	return sans, expiry
}
