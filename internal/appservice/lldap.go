package appservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/packalares/packalares/pkg/config"
	"k8s.io/klog/v2"
)

// LLDAPClient manages user sync to LLDAP via its GraphQL API.
// This is a simplified replacement for the beclab/lldap-client package.
type LLDAPClient struct {
	host     string
	username string
	password string

	mu    sync.Mutex
	token string
}

// NewLLDAPClient creates a client from environment or explicit config.
func NewLLDAPClient() *LLDAPClient {
	host := os.Getenv("LLDAP_HOST")
	if host == "" {
		host = "http://" + config.LLDAPHost() + ":" + config.LLDAPPort()
	}
	user := os.Getenv("LLDAP_BIND_DN")
	pass := os.Getenv("LLDAP_BIND_PASSWORD")

	return &LLDAPClient{
		host:     host,
		username: user,
		password: pass,
	}
}

// IsConfigured returns true if LLDAP credentials are available.
func (c *LLDAPClient) IsConfigured() bool {
	return c.username != "" && c.password != ""
}

// authenticate obtains a JWT token from LLDAP.
func (c *LLDAPClient) authenticate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" {
		return nil
	}

	body := map[string]string{
		"username": c.username,
		"password": c.password,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/auth/simple/login", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("lldap auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lldap auth status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("lldap auth decode: %w", err)
	}

	c.token = result.Token
	return nil
}

func (c *LLDAPClient) graphql(ctx context.Context, query string, variables map[string]interface{}) (json.RawMessage, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/graphql", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lldap graphql: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Reset token and retry once
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		if err := c.authenticate(ctx); err != nil {
			return nil, err
		}
		return c.graphql(ctx, query, variables)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lldap graphql status %d: %s", resp.StatusCode, string(body))
	}

	var gqlResp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, err
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("lldap graphql error: %s", gqlResp.Errors[0].Message)
	}

	return gqlResp.Data, nil
}

// CreateUser creates a user in LLDAP.
func (c *LLDAPClient) CreateUser(ctx context.Context, id, email, displayName, password string) error {
	query := `mutation CreateUser($user: CreateUserInput!) {
		createUser(user: $user) {
			id
		}
	}`

	user := map[string]interface{}{
		"id":          id,
		"email":       email,
		"displayName": displayName,
	}

	vars := map[string]interface{}{
		"user": user,
	}

	_, err := c.graphql(ctx, query, vars)
	if err != nil {
		return fmt.Errorf("create user %s: %w", id, err)
	}

	// Set password separately if provided
	if password != "" {
		if err := c.SetPassword(ctx, id, password); err != nil {
			klog.Warningf("failed to set password for %s: %v", id, err)
		}
	}

	return nil
}

// SetPassword sets a user's password in LLDAP.
func (c *LLDAPClient) SetPassword(ctx context.Context, userID, password string) error {
	// LLDAP uses a separate REST endpoint for password
	body := map[string]string{
		"userId":   userID,
		"password": password,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/auth/simple/change_password", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set password status %d: %s", resp.StatusCode, string(b))
	}

	return nil
}

// DeleteUser deletes a user from LLDAP.
func (c *LLDAPClient) DeleteUser(ctx context.Context, id string) error {
	query := `mutation DeleteUser($userId: String!) {
		deleteUser(userId: $userId) {
			ok
		}
	}`

	vars := map[string]interface{}{
		"userId": id,
	}

	_, err := c.graphql(ctx, query, vars)
	return err
}

// GetUser checks if a user exists in LLDAP.
func (c *LLDAPClient) GetUser(ctx context.Context, id string) (bool, error) {
	query := `query GetUser($userId: String!) {
		user(userId: $userId) {
			id
			email
			displayName
		}
	}`

	vars := map[string]interface{}{
		"userId": id,
	}

	_, err := c.graphql(ctx, query, vars)
	if err != nil {
		return false, err
	}
	return true, nil
}

// CreateGroup creates a group in LLDAP.
func (c *LLDAPClient) CreateGroup(ctx context.Context, name string) (int, error) {
	query := `mutation CreateGroup($name: String!) {
		createGroup(name: $name) {
			id
		}
	}`

	vars := map[string]interface{}{
		"name": name,
	}

	data, err := c.graphql(ctx, query, vars)
	if err != nil {
		return 0, err
	}

	var result struct {
		CreateGroup struct {
			ID int `json:"id"`
		} `json:"createGroup"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, err
	}

	return result.CreateGroup.ID, nil
}

// AddUserToGroup adds a user to a group in LLDAP.
func (c *LLDAPClient) AddUserToGroup(ctx context.Context, userID string, groupID int) error {
	query := `mutation AddUserToGroup($userId: String!, $groupId: Int!) {
		addUserToGroup(userId: $userId, groupId: $groupId) {
			ok
		}
	}`

	vars := map[string]interface{}{
		"userId":  userID,
		"groupId": groupID,
	}

	_, err := c.graphql(ctx, query, vars)
	return err
}

// SyncUser ensures a user exists in LLDAP with the given properties.
func (c *LLDAPClient) SyncUser(ctx context.Context, id, email, displayName, password string, groups []string) error {
	exists, err := c.GetUser(ctx, id)
	if err != nil {
		// Try creating the user (GetUser error might mean not found)
		klog.V(2).Infof("user %s lookup failed, attempting create: %v", id, err)
	}

	if !exists {
		if err := c.CreateUser(ctx, id, email, displayName, password); err != nil {
			return err
		}
		klog.Infof("created user %s in LLDAP", id)
	}

	// Sync groups
	for _, groupName := range groups {
		groupID, err := c.CreateGroup(ctx, groupName)
		if err != nil {
			klog.Warningf("create group %s: %v (may already exist)", groupName, err)
			continue
		}
		if err := c.AddUserToGroup(ctx, id, groupID); err != nil {
			klog.Warningf("add user %s to group %s: %v", id, groupName, err)
		}
	}

	return nil
}

// userSyncInterval is how often we check for users to sync.
var userSyncInterval = 30 * time.Second

// StartUserSyncLoop periodically checks for pending user syncs.
// In a full deployment this watches User CRDs; here it provides the framework.
func (c *LLDAPClient) StartUserSyncLoop(ctx context.Context) {
	if !c.IsConfigured() {
		klog.Info("LLDAP not configured, user sync disabled")
		return
	}

	klog.Info("starting LLDAP user sync loop")
	ticker := time.NewTicker(userSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// In production this would watch User CRDs and sync new users.
			// The framework is here for when CRD watching is wired up.
		}
	}
}
