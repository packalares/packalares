package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Server is the auth HTTP server.
type Server struct {
	cfg      *Config
	lldap    *LLDAPClient
	sessions *SessionStore
	mux      *http.ServeMux
}

func NewServer(cfg *Config) *Server {
	s := &Server{
		cfg:      cfg,
		lldap:    NewLLDAPClient(cfg.LLDAPHost, cfg.LLDAPPort),
		sessions: NewSessionStore(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB),
		mux:      http.NewServeMux(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/api/verify", s.handleVerify)
	s.mux.HandleFunc("/api/firstfactor", s.handleFirstFactor)
	s.mux.HandleFunc("/api/secondfactor/totp", s.handleSecondFactorTOTP)
	s.mux.HandleFunc("/api/secondfactor/totp/register", s.handleTOTPRegister)
	s.mux.HandleFunc("/api/logout", s.handleLogout)
	s.mux.HandleFunc("/api/state", s.handleState)
	s.mux.HandleFunc("/api/health", s.handleHealth)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("auth server listening on %s (user_zone=%s)", s.cfg.ListenAddr, s.cfg.UserZone)
	return srv.ListenAndServe()
}

// handleVerify is the nginx auth_request endpoint.
// Returns 200 if authenticated, 401 if not.
// nginx uses this with auth_request directive.
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	// Check if the target domain is public
	targetDomain := r.Header.Get("X-Original-URL")
	if targetDomain == "" {
		targetDomain = r.Header.Get("X-Forwarded-Host")
	}
	if s.isDomainPublic(targetDomain) {
		w.WriteHeader(http.StatusOK)
		return
	}

	session, err := s.getSession(r)
	if err != nil || session == nil {
		s.sendUnauthorizedRedirect(w, r)
		return
	}

	// Check auth level - need at least first factor
	if session.AuthLevel < 1 {
		s.sendUnauthorizedRedirect(w, r)
		return
	}

	// Refresh session TTL on activity
	sessionID := s.getSessionIDFromCookie(r)
	if sessionID != "" {
		_ = s.sessions.Touch(r.Context(), sessionID, time.Duration(s.cfg.SessionMaxAgeSec)*time.Second)
	}

	// Set headers for downstream services
	w.Header().Set("Remote-User", session.Username)
	w.Header().Set("Remote-Groups", strings.Join(session.Groups, ","))
	w.Header().Set("Remote-Auth-Level", fmt.Sprintf("%d", session.AuthLevel))
	w.WriteHeader(http.StatusOK)
}

// handleFirstFactor authenticates username/password against LLDAP.
func (s *Server) handleFirstFactor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"status": "error",
			"message": "method not allowed",
		})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		KeepMe   bool   `json:"keep_me_logged_in"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"status": "error",
			"message": "invalid request body",
		})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"status": "error",
			"message": "username and password required",
		})
		return
	}

	// Authenticate against LLDAP
	if err := s.lldap.Authenticate(req.Username, req.Password); err != nil {
		log.Printf("authentication failed for user %q: %v", req.Username, err)
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"status": "error",
			"message": "invalid credentials",
		})
		return
	}

	// Get user groups from LLDAP
	groups, err := s.lldap.GetUserGroups(s.cfg.LLDAPUser, s.cfg.LLDAPPassword, req.Username)
	if err != nil {
		log.Printf("failed to get groups for user %q: %v", req.Username, err)
		// Non-fatal - continue without groups
		groups = []string{}
	}

	// Create session
	sessionData := &SessionData{
		Username:  req.Username,
		Groups:    groups,
		AuthLevel: 1, // first factor complete
	}

	ttl := time.Duration(s.cfg.SessionMaxAgeSec) * time.Second
	sessionID, err := s.sessions.Create(r.Context(), sessionData, ttl)
	if err != nil {
		log.Printf("failed to create session for user %q: %v", req.Username, err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"status": "error",
			"message": "internal error",
		})
		return
	}

	// Sign session ID for cookie tamper protection
	signedID := SignSessionID(sessionID, s.cfg.SessionSecret)

	// Set cookie
	maxAge := s.cfg.SessionMaxAgeSec
	if !req.KeepMe {
		maxAge = 0 // session cookie
	}
	SetSessionCookie(w, s.cfg.SessionName, signedID, s.cfg.UserZone, maxAge, s.cfg.CookieSameSite)

	// Also issue a JWT token for API usage
	jwt, err := SignJWT(&JWTClaims{
		Subject:   req.Username,
		Username:  req.Username,
		Groups:    groups,
		AuthLevel: 1,
	}, s.cfg.JWTSecret)
	if err != nil {
		log.Printf("failed to sign JWT for user %q: %v", req.Username, err)
	}

	log.Printf("user %q authenticated successfully (first factor)", req.Username)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "OK",
		"data": map[string]interface{}{
			"token":     jwt,
			"redirect":  fmt.Sprintf("https://%s/", s.cfg.UserZone),
		},
	})
}

// handleSecondFactorTOTP validates a TOTP code for 2FA.
func (s *Server) handleSecondFactorTOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"status": "error",
			"message": "method not allowed",
		})
		return
	}

	session, err := s.getSession(r)
	if err != nil || session == nil || session.AuthLevel < 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"status": "error",
			"message": "first factor required",
		})
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"status": "error",
			"message": "invalid request body",
		})
		return
	}

	// Retrieve the user's TOTP secret from session-stored metadata
	// In a real deployment, TOTP secrets would be stored in LLDAP or a dedicated store.
	// For now, we store them in Redis under a separate key.
	totpSecret, err := s.getTOTPSecret(r.Context(), session.Username)
	if err != nil || totpSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"status": "error",
			"message": "TOTP not configured for this user",
		})
		return
	}

	if !ValidateTOTPCode(totpSecret, req.Token) {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"status": "error",
			"message": "invalid TOTP code",
		})
		return
	}

	// Upgrade session to second factor
	session.AuthLevel = 2
	session.TOTPVerified = true
	sessionID := s.getSessionIDFromCookie(r)
	if sessionID != "" {
		_ = s.sessions.Update(r.Context(), sessionID, session, time.Duration(s.cfg.SessionMaxAgeSec)*time.Second)
	}

	// Issue upgraded JWT
	jwt, _ := SignJWT(&JWTClaims{
		Subject:   session.Username,
		Username:  session.Username,
		Groups:    session.Groups,
		AuthLevel: 2,
	}, s.cfg.JWTSecret)

	log.Printf("user %q completed second factor (TOTP)", session.Username)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "OK",
		"data": map[string]interface{}{
			"token": jwt,
		},
	})
}

// handleTOTPRegister registers a TOTP secret for the current user.
func (s *Server) handleTOTPRegister(w http.ResponseWriter, r *http.Request) {
	session, err := s.getSession(r)
	if err != nil || session == nil || session.AuthLevel < 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"status": "error",
			"message": "authentication required",
		})
		return
	}

	switch r.Method {
	case http.MethodPost:
		// Generate new TOTP secret
		secret, uri, err := GenerateTOTPSecret(session.Username, s.cfg.TOTPIssuer)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status": "error",
				"message": "failed to generate TOTP secret",
			})
			return
		}

		// Store pending secret (not yet verified)
		if err := s.storeTOTPSecret(r.Context(), session.Username, secret); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status": "error",
				"message": "failed to store TOTP secret",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "OK",
			"data": map[string]interface{}{
				"secret":   secret,
				"otpauth":  uri,
			},
		})

	case http.MethodDelete:
		// Remove TOTP secret
		if err := s.deleteTOTPSecret(r.Context(), session.Username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status": "error",
				"message": "failed to remove TOTP",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "OK",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"status": "error",
			"message": "method not allowed",
		})
	}
}

// handleLogout destroys the session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID := s.getSessionIDFromCookie(r)
	if sessionID != "" {
		_ = s.sessions.Delete(r.Context(), sessionID)
	}
	ClearSessionCookie(w, s.cfg.SessionName, s.cfg.UserZone)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "OK",
	})
}

// handleState returns the current authentication state.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	session, _ := s.getSession(r)
	if session == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "OK",
			"data": map[string]interface{}{
				"authenticated": false,
			},
		})
		return
	}

	hasTOTP, _ := s.hasTOTPConfigured(r.Context(), session.Username)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "OK",
		"data": map[string]interface{}{
			"authenticated": true,
			"username":      session.Username,
			"auth_level":    session.AuthLevel,
			"groups":        session.Groups,
			"totp_enabled":  hasTOTP,
		},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "OK",
	})
}

// Helper methods

func (s *Server) getSession(r *http.Request) (*SessionData, error) {
	sessionID := s.getSessionIDFromCookie(r)
	if sessionID == "" {
		return nil, fmt.Errorf("no session cookie")
	}
	return s.sessions.Get(r.Context(), sessionID)
}

func (s *Server) getSessionIDFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(s.cfg.SessionName)
	if err != nil {
		return ""
	}

	rawID, valid := VerifySessionID(cookie.Value, s.cfg.SessionSecret)
	if !valid {
		return ""
	}
	return rawID
}

func (s *Server) isDomainPublic(domain string) bool {
	for _, d := range s.cfg.PublicDomains {
		if d == domain || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

func (s *Server) sendUnauthorizedRedirect(w http.ResponseWriter, r *http.Request) {
	authURL := fmt.Sprintf("https://auth.%s/", s.cfg.UserZone)
	// For nginx auth_request, return 401 with redirect header
	w.Header().Set("X-Redirect-URL", authURL)
	w.WriteHeader(http.StatusUnauthorized)
}

// TOTP secret storage in Redis

func (s *Server) getTOTPSecret(ctx context.Context, username string) (string, error) {
	conn, err := s.sessions.redisConn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return redisGetCmd(conn, "packalares:totp:"+username)
}

func (s *Server) storeTOTPSecret(ctx context.Context, username, secret string) error {
	conn, err := s.sessions.redisConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return redisCmd(conn, "SET", "packalares:totp:"+username, secret)
}

func (s *Server) deleteTOTPSecret(ctx context.Context, username string) error {
	conn, err := s.sessions.redisConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return redisCmd(conn, "DEL", "packalares:totp:"+username)
}

func (s *Server) hasTOTPConfigured(ctx context.Context, username string) (bool, error) {
	secret, err := s.getTOTPSecret(ctx, username)
	if err != nil {
		return false, err
	}
	return secret != "", nil
}

func writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
