package phases

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	ldapv3 "github.com/go-ldap/ldap/v3"
	"github.com/packalares/packalares/pkg/config"
)

// createLLDAPServiceAccount creates the svc-packalares service account in LLDAP.
// This runs during install so the auth service never needs the admin password at runtime.
func createLLDAPServiceAccount(opts *InstallOptions) error {
	lldapHost := fmt.Sprintf("lldap-svc.%s", config.PlatformNamespace())
	httpPort := 17170
	ldapPort := 3890
	adminUser := "admin"
	adminPassword := os.Getenv("LLDAP_ADMIN_PASSWORD")
	svcUser := "svc-packalares"
	svcPassword := os.Getenv("SVC_LLDAP_PASSWORD")
	baseDN := "dc=packalares,dc=local"

	if adminPassword == "" {
		return fmt.Errorf("LLDAP_ADMIN_PASSWORD not set")
	}
	if svcPassword == "" {
		return fmt.Errorf("SVC_LLDAP_PASSWORD not set")
	}

	// Wait for LLDAP to be ready
	fmt.Println("  Waiting for LLDAP ...")
	httpURL := fmt.Sprintf("http://%s:%d", lldapHost, httpPort)
	for i := 0; i < 60; i++ {
		resp, err := http.Get(httpURL + "/api/graphql")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Get admin token
	fmt.Println("  Creating service account ...")
	token, err := lldapLogin(httpURL, adminUser, adminPassword)
	if err != nil {
		return fmt.Errorf("admin login: %w", err)
	}

	// Create user (no-op if exists)
	err = lldapCreateUser(httpURL, token, svcUser, "Service Account")
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	// Add to lldap_admin group (group ID 1)
	_ = lldapAddToGroup(httpURL, token, svcUser, 1)

	// Set password via LDAP protocol
	ldapAddr := fmt.Sprintf("%s:%d", lldapHost, ldapPort)
	if err := lldapSetPassword(ldapAddr, adminUser, adminPassword, svcUser, svcPassword, baseDN); err != nil {
		return fmt.Errorf("set password: %w", err)
	}

	fmt.Printf("  Service account %q created\n", svcUser)
	return nil
}

func lldapLogin(baseURL, username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})

	resp, err := http.Post(baseURL+"/auth/simple/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Token, nil
}

func lldapCreateUser(baseURL, token, username, displayName string) error {
	gqlBody, _ := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`mutation { createUser(user: {id: "%s", email: "%s@local", displayName: "%s"}) { id } }`,
			escapeGQL(username), escapeGQL(username), escapeGQL(displayName)),
	})

	req, _ := http.NewRequest("POST", baseURL+"/api/graphql", bytes.NewReader(gqlBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if strings.Contains(string(body), "already exists") {
		return nil
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("create user: %s", string(body))
	}
	return nil
}

func lldapAddToGroup(baseURL, token, username string, groupID int) error {
	gqlBody, _ := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`mutation { addUserToGroup(userId: "%s", groupId: %d) { ok } }`,
			escapeGQL(username), groupID),
	})

	req, _ := http.NewRequest("POST", baseURL+"/api/graphql", bytes.NewReader(gqlBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func lldapSetPassword(ldapAddr, adminUser, adminPassword, targetUser, newPassword, baseDN string) error {
	conn, err := ldapv3.Dial("tcp", ldapAddr)
	if err != nil {
		return fmt.Errorf("ldap connect: %w", err)
	}
	defer conn.Close()

	adminDN := fmt.Sprintf("uid=%s,ou=people,%s", ldapv3.EscapeFilter(adminUser), baseDN)
	if err := conn.Bind(adminDN, adminPassword); err != nil {
		return fmt.Errorf("ldap bind: %w", err)
	}

	targetDN := fmt.Sprintf("uid=%s,ou=people,%s", ldapv3.EscapeFilter(targetUser), baseDN)
	req := ldapv3.NewPasswordModifyRequest(targetDN, "", newPassword)
	_, err = conn.PasswordModify(req)
	return err
}

func escapeGQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
