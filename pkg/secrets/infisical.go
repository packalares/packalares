// Package secrets provides a client for fetching secrets from the tapr gateway.
// Services call LoadSecrets() at startup to get all secrets from Infisical
// (via tapr), which handles authentication and decryption.
package secrets

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/packalares/packalares/pkg/config"
)

// Client fetches secrets from the tapr gateway.
type Client struct {
	url        string
	httpClient *http.Client
}

// NewClient creates a secrets client that talks to tapr.
func NewClient() *Client {
	url := os.Getenv("TAPR_URL")
	if url == "" {
		url = "http://tapr-svc." + config.PlatformNamespace() + ":8080"
	}
	return &Client{
		url:        url,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// LoadSecrets fetches all secrets from tapr.
// Returns a map of secret key → value.
func (c *Client) LoadSecrets() (map[string]string, error) {
	resp, err := c.httpClient.Get(c.url + "/secrets")
	if err != nil {
		return nil, fmt.Errorf("fetch secrets from tapr: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("tapr returned %d: %s", resp.StatusCode, string(body))
	}

	var secrets map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&secrets); err != nil {
		return nil, fmt.Errorf("decode secrets: %w", err)
	}

	return secrets, nil
}

// Get fetches a single secret from tapr.
func (c *Client) Get(key string) (string, error) {
	resp, err := c.httpClient.Get(c.url + "/secrets/" + key)
	if err != nil {
		return "", fmt.Errorf("fetch secret %s: %w", key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("secret %s not found", key)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("tapr returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Value, nil
}

// MustLoadSecrets loads secrets and sets them as environment variables.
// This is the primary entrypoint for services — call at startup.
// Falls back to existing env vars if tapr is unavailable.
func MustLoadSecrets() {
	client := NewClient()
	secrets, err := client.LoadSecrets()
	if err != nil {
		// Tapr not available — use existing env vars
		fmt.Printf("secrets: tapr unavailable (%v), using env vars\n", err)
		return
	}

	for k, v := range secrets {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	fmt.Printf("secrets: loaded %d secrets from tapr\n", len(secrets))
}
