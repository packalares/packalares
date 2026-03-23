package phases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
		"LLDAP_ADMIN_PASSWORD": generateSecret(16),
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

// SeedInfisical waits for the Infisical pod to be ready, then runs a
// K8s Job inside the cluster to create the admin account.
func SeedInfisical(opts *InstallOptions) error {
	ns := config.PlatformNamespace()

	// Wait for Infisical pod to be ready (has init containers that wait for PG+Redis)
	fmt.Println("  Waiting for Infisical pod to be ready ...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("infisical pod not ready after 5 minutes")
		default:
		}

		cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", ns,
			"-l", "app=infisical", "-o",
			"jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
		out, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(out)) == "True" {
			fmt.Println("  Infisical pod is ready")
			break
		}
		time.Sleep(5 * time.Second)
	}

	// Seed admin account via kubectl exec into the running Infisical pod
	fmt.Println("  Seeding Infisical admin account ...")
	adminEmail := config.Username() + "@" + config.Domain()
	adminPassword := "Packalares" + generateSecret(4) + "!"

	// Use curl inside the pod to hit localhost
	seedCmd := fmt.Sprintf(
		`curl -sf -X POST http://localhost:8080/api/v1/admin/signup `+
			`-H "Content-Type: application/json" `+
			`-d '{"email":"%s","firstName":"%s","lastName":"Admin","password":"%s"}' || true`,
		adminEmail, config.Username(), adminPassword,
	)

	cmd := exec.Command("kubectl", "exec", "-n", ns,
		"deploy/infisical", "-c", "infisical", "--",
		"sh", "-c", seedCmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  Admin signup: %v (may already exist)\n%s\n", err, string(out))
	}

	// Save credentials
	stateDir := filepath.Join(opts.BaseDir, "state")
	os.WriteFile(filepath.Join(stateDir, "infisical_admin_password"), []byte(adminPassword), 0600)
	os.WriteFile(filepath.Join(stateDir, "infisical_admin_email"), []byte(adminEmail), 0600)

	// Create machine identity secrets in both namespaces
	infisicalURL := "http://infisical-svc." + ns + ":8080"
	for _, targetNS := range []string{ns, config.FrameworkNamespace()} {
		secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: infisical-machine-identity
  namespace: %s
type: Opaque
stringData:
  clientId: ""
  clientSecret: ""
  projectId: ""
  infisicalUrl: "%s"
`, targetNS, infisicalURL)
		if err := kubectlApply(secretYAML); err != nil {
			fmt.Printf("  Warning: create infisical-machine-identity in %s: %v\n", targetNS, err)
		}
	}

	fmt.Printf("  Infisical admin: %s / %s\n", adminEmail, adminPassword)
	return nil
}

func generateSecret(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}
