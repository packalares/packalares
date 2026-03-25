package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
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
	s.mux.HandleFunc("/api/auth/password", s.handlePasswordChange)
	s.mux.HandleFunc("/api/auth/totp/setup", s.handleTOTPRegister)
	s.mux.HandleFunc("/api/auth/totp/validate", s.handleSecondFactorTOTP)
	s.mux.HandleFunc("/api/auth/sessions", s.handleSessions)
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

	// If TOTP is enabled, require second factor
	hasTOTP, _ := s.hasTOTPConfigured(r.Context(), session.Username)
	if hasTOTP && session.AuthLevel < 2 {
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

	// Set cookie — use request Host to determine domain scoping
	maxAge := s.cfg.SessionMaxAgeSec
	if !req.KeepMe {
		maxAge = 0 // session cookie
	}
	cookieDomain := s.cookieDomainForRequest(r)
	SetSessionCookie(w, s.cfg.SessionName, signedID, cookieDomain, maxAge, s.cfg.CookieSameSite)

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

	// Check if user has TOTP enabled — require second factor
	hasTOTP, _ := s.hasTOTPConfigured(r.Context(), req.Username)
	if hasTOTP {
		log.Printf("user %q has TOTP enabled, requiring second factor", req.Username)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":          "OK",
			"data": map[string]interface{}{
				"token":    jwt,
				"redirect": "",
			},
			"requires_totp": true,
		})
		return
	}

	// No TOTP — redirect to desktop
	redirect := "/desktop/"
	if rd := r.URL.Query().Get("rd"); rd != "" {
		redirect = rd
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "OK",
		"data": map[string]interface{}{
			"token":    jwt,
			"redirect": redirect,
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

	redirect := "/desktop/"
	if rd := r.URL.Query().Get("rd"); rd != "" {
		redirect = rd
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "OK",
		"data": map[string]interface{}{
			"token":    jwt,
			"redirect": redirect,
		},
	})
}

// handleTOTPRegister generates or removes TOTP for the current user.
// POST: generates a new pending secret (stored with 5min TTL, not active until verified)
// PUT:  verifies a code against the pending secret and activates TOTP
// DELETE: removes active TOTP
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
		// Generate new TOTP secret — store as PENDING only
		secret, uri, err := GenerateTOTPSecret(session.Username, s.cfg.TOTPIssuer)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status": "error",
				"message": "failed to generate TOTP secret",
			})
			return
		}

		if err := s.storePendingTOTP(r.Context(), session.Username, secret); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status": "error",
				"message": "failed to store pending TOTP",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "OK",
			"data": map[string]interface{}{
				"secret":  secret,
				"otpauth": uri,
			},
		})

	case http.MethodPut:
		// Verify code against pending secret, then activate
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"status": "error",
				"message": "token required",
			})
			return
		}

		pending, err := s.getPendingTOTP(r.Context(), session.Username)
		if err != nil || pending == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"status": "error",
				"message": "no pending TOTP setup — call POST first",
			})
			return
		}

		if !ValidateTOTPCode(pending, req.Token) {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"status": "error",
				"message": "invalid code — try again",
			})
			return
		}

		// Code valid — promote pending to active
		if err := s.storeTOTPSecret(r.Context(), session.Username, pending); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status": "error",
				"message": "failed to activate TOTP",
			})
			return
		}
		_ = s.deletePendingTOTP(r.Context(), session.Username)

		log.Printf("TOTP enabled for user %q", session.Username)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "OK",
			"message": "TOTP enabled",
		})

	case http.MethodDelete:
		if err := s.deleteTOTPSecret(r.Context(), session.Username); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status": "error",
				"message": "failed to remove TOTP",
			})
			return
		}
		_ = s.deletePendingTOTP(r.Context(), session.Username)
		log.Printf("TOTP disabled for user %q", session.Username)
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
	ClearSessionCookie(w, s.cfg.SessionName, s.cookieDomainForRequest(r))

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

// cookieDomainForRequest returns the appropriate cookie domain.
// If accessed via IP, returns "" (no domain = origin-only cookie).
// If accessed via hostname, returns the UserZone for subdomain sharing.
func (s *Server) cookieDomainForRequest(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// If the request comes via IP, don't set cookie domain
	if net.ParseIP(host) != nil {
		return ""
	}
	// For hostname access, scope cookie to the user zone
	return s.cfg.UserZone
}

func (s *Server) sendUnauthorizedRedirect(w http.ResponseWriter, r *http.Request) {
	// For nginx auth_request, return 401. The proxy handles the redirect to /login/.
	w.WriteHeader(http.StatusUnauthorized)
}

// handleSessions lists or revokes sessions.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	session, err := s.getSession(r)
	if err != nil || session == nil || session.AuthLevel < 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"status": "error", "message": "authentication required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		sessions, err := s.sessions.List(r.Context(), session.Username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"status": "error", "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "OK", "sessions": sessions})

	case http.MethodDelete:
		// Revoke a session by ID prefix
		var req struct {
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"status": "error", "message": "session_id required"})
			return
		}
		// Find full session ID by prefix
		sessions, _ := s.sessions.List(r.Context(), session.Username)
		for _, sess := range sessions {
			if sess.ID == req.SessionID || sess.FullID == req.SessionID {
				_ = s.sessions.Delete(r.Context(), sess.FullID)
				break
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "OK"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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

func (s *Server) storePendingTOTP(ctx context.Context, username, secret string) error {
	conn, err := s.sessions.redisConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return redisCmd(conn, "SETEX", "packalares:totp:pending:"+username, "300", secret) // 5 min TTL
}

func (s *Server) getPendingTOTP(ctx context.Context, username string) (string, error) {
	conn, err := s.sessions.redisConn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return redisGetCmd(conn, "packalares:totp:pending:"+username)
}

func (s *Server) deletePendingTOTP(ctx context.Context, username string) error {
	conn, err := s.sessions.redisConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return redisCmd(conn, "DEL", "packalares:totp:pending:"+username)
}

func (s *Server) hasTOTPConfigured(ctx context.Context, username string) (bool, error) {
	secret, err := s.getTOTPSecret(ctx, username)
	if err != nil {
		return false, err
	}
	return secret != "", nil
}

// handlePasswordChange lets the authenticated user change their password.
func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"status": "error", "message": "method not allowed"})
		return
	}

	session, err := s.getSession(r)
	if err != nil || session == nil || session.AuthLevel < 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"status": "error", "message": "authentication required"})
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"status": "error", "message": "invalid request"})
		return
	}

	// Verify current password
	if err := s.lldap.Authenticate(session.Username, req.CurrentPassword); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"status": "error", "message": "current password is incorrect"})
		return
	}

	// Change password in LLDAP
	if err := s.lldap.ChangePassword(s.cfg.LLDAPUser, s.cfg.LLDAPPassword, session.Username, req.NewPassword); err != nil {
		log.Printf("password change failed for %s: %v", session.Username, err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"status": "error", "message": "failed to change password"})
		return
	}

	log.Printf("password changed for user %s", session.Username)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "OK"})
}

func writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
