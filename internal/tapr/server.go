package tapr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Server is the tapr secrets gateway. It sits alongside Infisical and provides
// a simple API for services to read/write secrets without SRP authentication.
type Server struct {
	infisicalURL string // e.g. http://localhost:4000
	jwtSecret    string // from infisical-backend K8s Secret
	userID       string
	orgID        string
	sessionID    string // auth_token_sessions.id — required for JWT
	publicKey    string // base64 NaCl public key
	privateKey   string // base64 NaCl private key (plaintext)
	password     string // admin password for decrypting private key
	pgDSN        string
	authToken    string // bearer token for API authentication
}

// NewServer creates a tapr server.
func NewServer() *Server {
	jwtSecret := os.Getenv("JWT_AUTH_SECRET")
	if len(jwtSecret) < 32 {
		log.Fatalf("FATAL: JWT_AUTH_SECRET is too short (%d chars). Set a secret of at least 32 characters.", len(jwtSecret))
	}

	authToken := os.Getenv("TAPR_AUTH_TOKEN")
	if authToken == "" {
		log.Println("WARNING: TAPR_AUTH_TOKEN not set — tapr API is unauthenticated!")
	}

	return &Server{
		infisicalURL: envOr("INFISICAL_URL", "http://localhost:4000"),
		jwtSecret:    jwtSecret,
		userID:       os.Getenv("TAPR_USER_ID"),
		orgID:        os.Getenv("TAPR_ORG_ID"),
		publicKey:    os.Getenv("TAPR_PUBLIC_KEY"),
		privateKey:   os.Getenv("TAPR_PRIVATE_KEY"),
		password:     os.Getenv("TAPR_PASSWORD"),
		pgDSN:        os.Getenv("PG_DSN"),
		authToken:    authToken,
	}
}

// issueToken creates a JWT token that Infisical's API accepts.
func (s *Server) issueToken() (string, error) {
	if s.jwtSecret == "" {
		return "", fmt.Errorf("JWT_AUTH_SECRET not set")
	}

	claims := jwt.MapClaims{
		"userId":         s.userID,
		"authTokenType":  "accessToken",
		"tokenVersionId": s.sessionID,
		"accessVersion":  1,
		"organizationId": s.orgID,
		"authMethod":     "email",
		"iat":            time.Now().Unix(),
		"exp":            time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// requireAuth is middleware that checks the Bearer token on every request.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authToken == "" {
			next(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" || auth != "Bearer "+s.authToken {
			http.Error(w, "unauthorized", 401)
			return
		}
		next(w, r)
	}
}

// Handler returns the HTTP handler for the tapr API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// GET /secrets — list all secrets from the default project
	mux.HandleFunc("/secrets", s.requireAuth(s.handleSecrets))

	// GET /secrets/{key} — get a single secret
	mux.HandleFunc("/secrets/", s.requireAuth(s.handleGetSecret))

	// POST /secrets — create/update secrets (bulk)
	mux.HandleFunc("POST /secrets", s.requireAuth(s.handleStoreSecrets))

	return mux
}

func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}

	secrets, err := s.fetchAllSecrets()
	if err != nil {
		log.Printf("tapr: GET /secrets error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(secrets)
}

func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/secrets/")
	if key == "" {
		http.Error(w, "missing key", 400)
		return
	}

	secrets, err := s.fetchAllSecrets()
	if err != nil {
		log.Printf("tapr: GET /secrets/%s error: %v", key, err)
		http.Error(w, "internal error", 500)
		return
	}

	val, ok := secrets[key]
	if !ok {
		http.Error(w, "not found", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": key, "value": val})
}

func (s *Server) handleStoreSecrets(w http.ResponseWriter, r *http.Request) {
	var secrets map[string]string
	if err := json.NewDecoder(r.Body).Decode(&secrets); err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	if err := s.storeSecrets(secrets); err != nil {
		log.Printf("tapr: POST /secrets error: %v", err)
		http.Error(w, "internal error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// fetchAllSecrets gets all secrets from the default project using the raw API.
func (s *Server) fetchAllSecrets() (map[string]string, error) {
	token, err := s.issueToken()
	if err != nil {
		return nil, err
	}

	workspaceID, err := s.getDefaultWorkspace(token)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v3/secrets/raw?workspaceId=%s&environment=prod&secretPath=/", s.infisicalURL, workspaceID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create secrets request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch secrets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fetch secrets: %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Secrets []struct {
			SecretKey   string `json:"secretKey"`
			SecretValue string `json:"secretValue"`
		} `json:"secrets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("decode secrets response: %v", err)
	}

	secrets := make(map[string]string)
	for _, s := range result.Secrets {
		secrets[s.SecretKey] = s.SecretValue
	}

	return secrets, nil
}

// storeSecrets creates/updates secrets in the default project using the raw API.
func (s *Server) storeSecrets(secrets map[string]string) error {
	token, err := s.issueToken()
	if err != nil {
		return err
	}

	workspaceID, err := s.getOrCreateDefaultWorkspace(token)
	if err != nil {
		return err
	}

	for name, value := range secrets {
		body := map[string]interface{}{
			"workspaceId": workspaceID,
			"environment": "prod",
			"secretPath":  "/",
			"secretValue": value,
			"type":        "shared",
		}

		bodyJSON, _ := json.Marshal(body)
		url := fmt.Sprintf("%s/api/v3/secrets/raw/%s", s.infisicalURL, name)

		req, err := http.NewRequest("POST", url, strings.NewReader(string(bodyJSON)))
		if err != nil {
			log.Printf("create store secret request: %v", err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("store secret %s: %w", name, err)
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			// Try PATCH for update
			patchBody, _ := json.Marshal(map[string]interface{}{
				"workspaceId": workspaceID,
				"environment": "prod",
				"secretPath":  "/",
				"secretValue": value,
			})
			req2, _ := http.NewRequest("PATCH", url, strings.NewReader(string(patchBody)))
			req2.Header.Set("Authorization", "Bearer "+token)
			req2.Header.Set("Content-Type", "application/json")
			resp2, err := http.DefaultClient.Do(req2)
			if err != nil {
				return fmt.Errorf("update secret %s: %w", name, err)
			}
			resp2.Body.Close()
		}
		_ = respBody
	}

	return nil
}

func (s *Server) getDefaultWorkspace(token string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/workspace", s.infisicalURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create workspace request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("list workspaces: %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Workspaces []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"workspaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("decode workspaces response: %v", err)
	}

	for _, w := range result.Workspaces {
		if w.Name == "packalares-system" {
			return w.ID, nil
		}
	}

	return "", fmt.Errorf("workspace 'packalares-system' not found")
}

func (s *Server) getOrCreateDefaultWorkspace(token string) (string, error) {
	id, err := s.getDefaultWorkspace(token)
	if err == nil {
		return id, nil
	}

	// Create workspace
	body, _ := json.Marshal(map[string]string{"projectName": "packalares-system"})
	url := fmt.Sprintf("%s/api/v2/workspace", s.infisicalURL)
	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("create workspace request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("create workspace: %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("decode project response: %v", err)
	}

	return result.Project.ID, nil
}

// SeedAndStart seeds Infisical if needed, then starts the HTTP server.
func SeedAndStart(ctx context.Context, listenAddr string, initialSecrets map[string]string) error {
	srv := NewServer()

	// Seed if user ID not set (first run)
	if srv.userID == "" && srv.pgDSN != "" {
		log.Println("tapr: seeding Infisical database...")
		result, err := Seed(ctx, SeedConfig{
			PGDSN:       srv.pgDSN,
			Email:       envOr("ADMIN_EMAIL", "admin@packalares.local"),
			Username:    envOr("ADMIN_USERNAME", "admin"),
			Password:    srv.password,
			OrgName:     "Packalares",
			ProjectName: "packalares-system",
		})
		if err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		srv.userID = result.UserID
		srv.orgID = result.OrgID
		srv.sessionID = result.SessionID
		srv.publicKey = result.PublicKey
		srv.privateKey = result.PrivateKey
		log.Printf("tapr: seeded user=%s org=%s session=%s", srv.userID, srv.orgID, srv.sessionID)

		// Create workspace — retry until Infisical's HTTP API is ready
		token, err := srv.issueToken()
		if err == nil {
			for attempt := 0; attempt < 30; attempt++ {
				wsID, err := srv.getOrCreateDefaultWorkspace(token)
				if err == nil {
					log.Printf("tapr: workspace ready: %s", wsID)
					break
				}
				if attempt < 29 {
					time.Sleep(2 * time.Second)
				} else {
					log.Printf("tapr: warning: create workspace failed after 30 attempts: %v", err)
				}
			}
		}
	}

	// Store initial secrets if provided
	if len(initialSecrets) > 0 {
		log.Printf("tapr: storing %d initial secrets...", len(initialSecrets))
		if err := srv.storeSecrets(initialSecrets); err != nil {
			log.Printf("tapr: warning: store initial secrets: %v", err)
		}
	}

	log.Printf("tapr: listening on %s", listenAddr)
	return http.ListenAndServe(listenAddr, srv.Handler())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
