package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLDAPClient authenticates users against LLDAP's HTTP API.
type LLDAPClient struct {
	host     string
	port     int
	client   *http.Client
}

type lldapLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type lldapLoginResponse struct {
	Token string `json:"token"`
}

func NewLLDAPClient(host string, port int) *LLDAPClient {
	return &LLDAPClient{
		host: host,
		port: port,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Authenticate validates username/password against LLDAP.
// Returns nil on success, error on failure.
func (l *LLDAPClient) Authenticate(username, password string) error {
	url := fmt.Sprintf("http://%s:%d/auth/simple/login", l.host, l.port)

	body, err := json.Marshal(lldapLoginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return fmt.Errorf("marshal login request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("lldap request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, string(respBody))
}

// GetUserGroups fetches groups for a user from LLDAP GraphQL API.
// This requires an admin token. Used to check if user is admin.
func (l *LLDAPClient) GetUserGroups(adminUser, adminPassword, username string) ([]string, error) {
	// First get admin token
	adminToken, err := l.getToken(adminUser, adminPassword)
	if err != nil {
		return nil, fmt.Errorf("get admin token: %w", err)
	}

	// Query GraphQL for user groups
	url := fmt.Sprintf("http://%s:%d/api/graphql", l.host, l.port)

	query := fmt.Sprintf(`{"query":"{ user(userId: \"%s\") { groups { displayName } } }"}`, username)

	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte(query)))
	if err != nil {
		return nil, fmt.Errorf("create graphql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graphql request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql query failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			User struct {
				Groups []struct {
					DisplayName string `json:"displayName"`
				} `json:"groups"`
			} `json:"user"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse graphql response: %w", err)
	}

	var groups []string
	for _, g := range result.Data.User.Groups {
		groups = append(groups, g.DisplayName)
	}
	return groups, nil
}

// ChangePassword changes a user's password via LLDAP GraphQL API.
func (l *LLDAPClient) ChangePassword(adminUser, adminPassword, username, newPassword string) error {
	adminToken, err := l.getToken(adminUser, adminPassword)
	if err != nil {
		return fmt.Errorf("get admin token: %w", err)
	}

	url := fmt.Sprintf("http://%s:%d/api/graphql", l.host, l.port)
	query := fmt.Sprintf(`{"query":"mutation { updateUser(user: {id: \"%s\", password: \"%s\"}) { ok } }"}`, username, newPassword)

	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte(query)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("change password failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (l *LLDAPClient) getToken(username, password string) (string, error) {
	url := fmt.Sprintf("http://%s:%d/auth/simple/login", l.host, l.port)

	body, err := json.Marshal(lldapLoginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var loginResp lldapLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", err
	}

	return loginResp.Token, nil
}
