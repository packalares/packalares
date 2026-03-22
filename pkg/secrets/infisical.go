// Package secrets provides a client for fetching secrets from Infisical.
// Services call LoadSecrets() at startup to get all secrets from Infisical,
// falling back to environment variables if Infisical is unavailable.
package secrets

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/config"
)

// Client fetches secrets from Infisical via machine identity auth.
type Client struct {
	url         string
	clientID    string
	clientSecret string
	projectID   string
	environment string
	httpClient  *http.Client
}

// NewClient creates an Infisical client from environment variables or K8s Secret mount.
func NewClient() *Client {
	return &Client{
		url:          envOr("INFISICAL_URL", "http://"+config.InfisicalDNS()+":8080"),
		clientID:     envOr("INFISICAL_CLIENT_ID", ""),
		clientSecret: envOr("INFISICAL_CLIENT_SECRET", ""),
		projectID:    envOr("INFISICAL_PROJECT_ID", ""),
		environment:  envOr("INFISICAL_ENVIRONMENT", "prod"),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// LoadSecrets authenticates with Infisical and returns all secrets as a map.
// Returns nil error and empty map if Infisical is not configured.
func (c *Client) LoadSecrets() (map[string]string, error) {
	if c.clientID == "" || c.clientSecret == "" {
		return nil, nil // Not configured, use env vars
	}

	// Step 1: Authenticate
	token, err := c.authenticate()
	if err != nil {
		return nil, fmt.Errorf("infisical auth: %w", err)
	}

	// Step 2: Fetch secrets
	return c.fetchSecrets(token)
}

func (c *Client) authenticate() (string, error) {
	body := fmt.Sprintf(`{"clientId":"%s","clientSecret":"%s"}`, c.clientID, c.clientSecret)
	req, err := http.NewRequest("POST", c.url+"/api/v1/auth/universal-auth/login",
		strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("auth failed (%d): %s", resp.StatusCode, string(b))
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

func (c *Client) fetchSecrets(token string) (map[string]string, error) {
	url := fmt.Sprintf("%s/api/v3/secrets/raw?workspaceId=%s&environment=%s&secretPath=/",
		c.url, c.projectID, c.environment)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fetch secrets (%d): %s", resp.StatusCode, string(b))
	}

	var result struct {
		Secrets []struct {
			SecretKey   string `json:"secretKey"`
			SecretValue string `json:"secretValue"`
		} `json:"secrets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	secrets := make(map[string]string, len(result.Secrets))
	for _, s := range result.Secrets {
		secrets[s.SecretKey] = s.SecretValue
	}
	return secrets, nil
}

// GetOrEnv returns the secret value from Infisical secrets map,
// falling back to the environment variable if not found.
func GetOrEnv(secrets map[string]string, key string) string {
	if secrets != nil {
		if v, ok := secrets[key]; ok && v != "" {
			return v
		}
	}
	return os.Getenv(key)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
