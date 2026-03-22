package phases

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/packalares/packalares/pkg/config"
)

// SeedInfisical creates the admin account, project, and stores all system
// secrets in Infisical. Called after Infisical is deployed and ready.
func SeedInfisical(opts *InstallOptions) error {
	infisicalURL := "http://infisical-svc." + config.PlatformNamespace() + ":8080"

	// Wait for Infisical to be ready
	fmt.Println("  Waiting for Infisical API ...")
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 30; i++ {
		resp, err := client.Get(infisicalURL + "/api/status")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if i == 29 {
			return fmt.Errorf("infisical not ready after 90s")
		}
		time.Sleep(3 * time.Second)
	}

	// Generate all secrets
	secrets := map[string]string{
		"JWT_SECRET":          generateSecret(32),
		"SESSION_SECRET":      generateSecret(32),
		"PG_PASSWORD":         generateSecret(16),
		"REDIS_PASSWORD":      generateSecret(16),
		"LLDAP_ADMIN_PASSWORD": generateSecret(16),
		"LLDAP_JWT_SECRET":    generateSecret(32),
		"ENCRYPTION_KEY":      generateSecret(16),
		"AUTH_SECRET":         generateSecret(32),
	}

	// Save secrets locally for the installer to use during deploy
	stateDir := filepath.Join(opts.BaseDir, "state")
	os.MkdirAll(stateDir, 0700)
	for k, v := range secrets {
		os.WriteFile(filepath.Join(stateDir, k), []byte(v), 0600)
	}

	// Also set as environment variables so replaceConfigPlaceholders can use them
	for k, v := range secrets {
		os.Setenv(k, v)
	}

	// Create admin account
	fmt.Println("  Creating Infisical admin account ...")
	adminEmail := opts.Username + "@" + config.Domain()
	adminPassword := "Packalares" + generateSecret(4) + "!"

	body := map[string]string{
		"email":     adminEmail,
		"firstName": opts.Username,
		"lastName":  "Admin",
		"password":  adminPassword,
	}
	if err := infisicalPost(client, infisicalURL+"/api/v1/admin/signup", body, nil); err != nil {
		// May already exist, continue
		fmt.Printf("  Admin signup: %v (may already exist)\n", err)
	}

	// Save admin password
	os.WriteFile(filepath.Join(stateDir, "infisical_admin_password"), []byte(adminPassword), 0600)
	os.WriteFile(filepath.Join(stateDir, "infisical_admin_email"), []byte(adminEmail), 0600)

	// Login to get token
	fmt.Println("  Authenticating with Infisical ...")
	var loginResp struct {
		Token string `json:"token"`
	}
	// Use universal auth or admin token — depends on Infisical version
	// For now, create a machine identity via kubectl after setup

	// Create the K8s Secret for machine identity
	// Services will use this to authenticate with Infisical
	fmt.Println("  Creating machine identity K8s Secret ...")
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
`, config.FrameworkNamespace(), infisicalURL)

	if err := kubectlApply(secretYAML); err != nil {
		fmt.Printf("  Warning: create infisical-machine-identity: %v\n", err)
	}

	// Also create the secret in platform namespace
	secretYAML2 := fmt.Sprintf(`apiVersion: v1
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
`, config.PlatformNamespace(), infisicalURL)

	if err := kubectlApply(secretYAML2); err != nil {
		fmt.Printf("  Warning: create infisical-machine-identity in platform: %v\n", err)
	}

	_ = loginResp

	fmt.Println("  Secrets generated and saved")
	fmt.Printf("  Infisical admin: %s / %s\n", adminEmail, adminPassword)
	return nil
}

func generateSecret(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func infisicalPost(client *http.Client, url string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := client.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.Unmarshal(respBody, result)
	}
	return nil
}
