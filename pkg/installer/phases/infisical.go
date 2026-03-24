package phases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/config"
)

// GenerateSecrets creates all system secrets, saves them to disk and sets
// them as env vars so replaceConfigPlaceholders can inject them into manifests.
// Must be called BEFORE deploying platform/framework services.
func GenerateSecrets(opts *InstallOptions) error {
	fmt.Println("  Generating system secrets ...")

	secrets := map[string]string{
		"JWT_SECRET":           generateSecret(32),
		"SESSION_SECRET":       generateSecret(32),
		"PG_PASSWORD":          generateSecret(16),
		"REDIS_PASSWORD":       generateSecret(16),
		"LLDAP_ADMIN_PASSWORD": opts.Password,
		"LLDAP_JWT_SECRET":     generateSecret(32),
		"ENCRYPTION_KEY":       generateSecret(16),
		"AUTH_SECRET":          generateSecret(32),
	}

	// Also set PG_USER default
	if os.Getenv("PG_USER") == "" {
		secrets["PG_USER"] = "packalares"
	}

	// Save to disk
	stateDir := filepath.Join(opts.BaseDir, "state")
	os.MkdirAll(stateDir, 0700)
	for k, v := range secrets {
		os.WriteFile(filepath.Join(stateDir, k), []byte(v), 0600)
	}

	// Set as env vars for placeholder replacement
	for k, v := range secrets {
		os.Setenv(k, v)
	}

	// Detect SERVER_IP
	if os.Getenv("SERVER_IP") == "" {
		if ip, err := detectServerIP(); err == nil {
			os.Setenv("SERVER_IP", ip)
			fmt.Printf("  Detected server IP: %s\n", ip)
		}
	}

	// Detect COREDNS_CLUSTER_IP
	if os.Getenv("COREDNS_CLUSTER_IP") == "" {
		if ip, err := detectCoreDNSIP(); err == nil {
			os.Setenv("COREDNS_CLUSTER_IP", ip)
			fmt.Printf("  Detected CoreDNS IP: %s\n", ip)
		}
	}

	fmt.Println("  Secrets generated and saved to", stateDir)
	return nil
}

func detectServerIP() (string, error) {
	out, err := exec.Command("sh", "-c",
		"ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \\K[^ ]+'").Output()
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("no IP found")
	}
	return ip, nil
}

func detectCoreDNSIP() (string, error) {
	out, err := exec.Command("kubectl", "get", "svc", "kube-dns",
		"-n", "kube-system", "-o", "jsonpath={.spec.clusterIP}").Output()
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("no CoreDNS IP found")
	}
	return ip, nil
}

// SeedInfisical waits for the tapr sidecar to be ready, then stores all
// generated secrets via tapr's API. Tapr handles the Infisical database
// seeding (user, org, encryption keys) automatically at startup.
func SeedInfisical(opts *InstallOptions) error {
	ns := config.PlatformNamespace()

	// Wait for tapr sidecar to be ready
	fmt.Println("  Waiting for tapr secrets gateway ...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("tapr not ready after 5 minutes")
		default:
		}

		// Check tapr via kubectl exec on the infisical pod (tapr is a sidecar)
		cmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", ns,
			"deploy/infisical", "-c", "tapr", "--",
			"wget", "-q", "-O-", "http://localhost:8081/healthz")
		if out, err := cmd.CombinedOutput(); err == nil && strings.Contains(string(out), "ok") {
			fmt.Println("  Tapr is ready")
			break
		}
		time.Sleep(5 * time.Second)
	}

	// Store all generated secrets via tapr
	fmt.Println("  Storing secrets in Infisical via tapr ...")
	secrets := map[string]string{
		"REDIS_PASSWORD":       os.Getenv("REDIS_PASSWORD"),
		"PG_PASSWORD":          os.Getenv("PG_PASSWORD"),
		"PG_USER":              os.Getenv("PG_USER"),
		"JWT_SECRET":           os.Getenv("JWT_SECRET"),
		"SESSION_SECRET":       os.Getenv("SESSION_SECRET"),
		"LLDAP_ADMIN_PASSWORD": os.Getenv("LLDAP_ADMIN_PASSWORD"),
		"LLDAP_JWT_SECRET":     os.Getenv("LLDAP_JWT_SECRET"),
		"ENCRYPTION_KEY":       os.Getenv("ENCRYPTION_KEY"),
		"AUTH_SECRET":          os.Getenv("AUTH_SECRET"),
	}

	// Store secrets via kubectl exec into the tapr sidecar
	secretsJSON, _ := json.Marshal(secrets)
	storeCmd := fmt.Sprintf(
		`wget -q -O- --post-data='%s' --header='Content-Type: application/json' http://localhost:8081/secrets`,
		string(secretsJSON),
	)

	cmd := exec.Command("kubectl", "exec", "-n", ns,
		"deploy/infisical", "-c", "tapr", "--",
		"sh", "-c", storeCmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  Warning: store secrets: %v\n%s\n", err, string(out))
	} else {
		fmt.Printf("  Stored %d secrets in Infisical\n", len(secrets))
	}

	// Save admin info
	stateDir := filepath.Join(opts.BaseDir, "state")
	adminEmail := config.Username() + "@" + config.Domain()
	os.WriteFile(filepath.Join(stateDir, "infisical_admin_email"), []byte(adminEmail), 0600)

	fmt.Printf("  Infisical admin: %s\n", adminEmail)
	return nil
}

func generateSecret(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}
