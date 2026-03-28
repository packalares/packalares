package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ldapv3 "github.com/go-ldap/ldap/v3"
)

// escapeGraphQL escapes special characters for use inside GraphQL string literals.
// Prevents injection by escaping quotes and backslashes.
func escapeGraphQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

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

	// Use json.Marshal to build the request body — prevents GraphQL injection
	// by properly escaping quotes and special characters in the username
	gqlBody, _ := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`{ user(userId: "%s") { groups { displayName } } }`, escapeGraphQL(username)),
	})
	query := string(gqlBody)

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

// ChangePasswordLDAP changes a user's password via the LDAP protocol (port 3890).
// LLDAP v0.5 has no HTTP API for password changes, but LDAP password modify works.
func (l *LLDAPClient) ChangePasswordLDAP(username, oldPassword, newPassword, baseDN string) error {
	ldapAddr := fmt.Sprintf("%s:3890", l.host)

	conn, err := ldapv3.Dial("tcp", ldapAddr)
	if err != nil {
		return fmt.Errorf("ldap connect: %w", err)
	}
	defer conn.Close()

	// Bind as the user
	userDN := fmt.Sprintf("uid=%s,ou=people,%s", ldapv3.EscapeFilter(username), baseDN)
	if err := conn.Bind(userDN, oldPassword); err != nil {
		return fmt.Errorf("ldap bind: %w", err)
	}

	// Change password using LDAP Password Modify Extended Operation
	req := ldapv3.NewPasswordModifyRequest(userDN, oldPassword, newPassword)
	_, err = conn.PasswordModify(req)
	if err != nil {
		return fmt.Errorf("ldap password modify: %w", err)
	}

	return nil
}

// CreateUser creates a new user in LLDAP. Returns nil if user already exists.
func (l *LLDAPClient) CreateUser(adminUser, adminPassword, username, password, displayName string) error {
	adminToken, err := l.getToken(adminUser, adminPassword)
	if err != nil {
		return fmt.Errorf("get admin token: %w", err)
	}

	url := fmt.Sprintf("http://%s:%d/api/graphql", l.host, l.port)
	gqlBody, _ := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`mutation { createUser(user: {id: "%s", email: "%s@local", displayName: "%s"}) { id } }`,
			escapeGraphQL(username), escapeGraphQL(username), escapeGraphQL(displayName)),
	})

	req, err := http.NewRequest("POST", url, bytes.NewReader(gqlBody))
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// If user already exists, that's fine
	if strings.Contains(string(body), "already exists") {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create user failed: %s", string(body))
	}

	return nil
}

// AddUserToGroup adds a user to an LLDAP group.
func (l *LLDAPClient) AddUserToGroup(adminUser, adminPassword, username string, groupID int) error {
	adminToken, err := l.getToken(adminUser, adminPassword)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s:%d/api/graphql", l.host, l.port)
	gqlBody, _ := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`mutation { addUserToGroup(userId: "%s", groupId: %d) { ok } }`,
			escapeGraphQL(username), groupID),
	})

	req, err := http.NewRequest("POST", url, bytes.NewReader(gqlBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
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
