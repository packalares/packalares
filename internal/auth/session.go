package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// SessionStore manages sessions in Redis.
type SessionStore struct {
	rdb    *redis.Client
	prefix string
}

// SessionData holds session information stored in Redis.
type SessionData struct {
	Username     string    `json:"username"`
	Groups       []string  `json:"groups"`
	AuthLevel    int       `json:"auth_level"` // 0=unauth, 1=first_factor, 2=second_factor
	TOTPVerified bool      `json:"totp_verified"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
}

func NewSessionStore(addr, password string, db int) *SessionStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &SessionStore{
		rdb:    rdb,
		prefix: "packalares:session:",
	}
}

// Redis returns the underlying redis client for shared use.
func (s *SessionStore) Redis() *redis.Client {
	return s.rdb
}

// Create stores a new session and returns the session ID.
func (s *SessionStore) Create(ctx context.Context, data *SessionData, ttl time.Duration) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}

	data.CreatedAt = time.Now()
	data.LastActivity = time.Now()

	encoded, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}

	key := s.prefix + sessionID
	if err := s.rdb.Set(ctx, key, string(encoded), ttl).Err(); err != nil {
		return "", fmt.Errorf("redis set: %w", err)
	}

	return sessionID, nil
}

// Get retrieves a session by ID.
func (s *SessionStore) Get(ctx context.Context, sessionID string) (*SessionData, error) {
	key := s.prefix + sessionID
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, err
	}

	var data SessionData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &data, nil
}

// Update replaces session data preserving the same key.
func (s *SessionStore) Update(ctx context.Context, sessionID string, data *SessionData, ttl time.Duration) error {
	data.LastActivity = time.Now()

	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	key := s.prefix + sessionID
	return s.rdb.Set(ctx, key, string(encoded), ttl).Err()
}

// Delete removes a session.
func (s *SessionStore) Delete(ctx context.Context, sessionID string) error {
	key := s.prefix + sessionID
	return s.rdb.Del(ctx, key).Err()
}

// List returns all active sessions for a user.
func (s *SessionStore) List(ctx context.Context, username string) ([]SessionInfo, error) {
	pattern := s.prefix + "*"
	var sessions []SessionInfo

	iter := s.rdb.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		sessionID := strings.TrimPrefix(key, s.prefix)
		data, err := s.Get(ctx, sessionID)
		if err != nil || data == nil {
			continue
		}
		if username != "" && data.Username != username {
			continue
		}
		sessions = append(sessions, SessionInfo{
			ID:           sessionID[:8] + "...",
			FullID:       sessionID,
			Username:     data.Username,
			CreatedAt:    data.CreatedAt,
			LastActivity: data.LastActivity,
			AuthLevel:    data.AuthLevel,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

// SessionInfo is the public representation of a session.
type SessionInfo struct {
	ID           string    `json:"id"`
	FullID       string    `json:"-"`
	Username     string    `json:"username"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
	AuthLevel    int       `json:"auth_level"`
}

// Touch updates the session TTL without changing data.
func (s *SessionStore) Touch(ctx context.Context, sessionID string, ttl time.Duration) error {
	key := s.prefix + sessionID
	return s.rdb.Expire(ctx, key, ttl).Err()
}

func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetSessionCookie writes the session cookie on the response.
func SetSessionCookie(w http.ResponseWriter, name, sessionID, domain string, maxAge int, sameSite string) {
	ss := http.SameSiteLaxMode
	switch strings.ToLower(sameSite) {
	case "none":
		ss = http.SameSiteNoneMode
	case "strict":
		ss = http.SameSiteStrictMode
	}

	cookie := &http.Cookie{
		Name:     name,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: ss,
	}

	// Only set Domain for hostname-based access, not IP addresses.
	// Browsers reject Domain attribute on IP-addressed cookies.
	if domain != "" && net.ParseIP(domain) == nil {
		cookieDomain := domain
		if !strings.HasPrefix(cookieDomain, ".") {
			cookieDomain = "." + cookieDomain
		}
		cookie.Domain = cookieDomain
	}

	http.SetCookie(w, cookie)
}

// ClearSessionCookie clears the session cookie.
func ClearSessionCookie(w http.ResponseWriter, name, domain string) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
	}

	if domain != "" && net.ParseIP(domain) == nil {
		cookieDomain := domain
		if !strings.HasPrefix(cookieDomain, ".") {
			cookieDomain = "." + cookieDomain
		}
		cookie.Domain = cookieDomain
	}

	http.SetCookie(w, cookie)
}

// SignSessionID creates an HMAC signature for tamper detection.
func SignSessionID(sessionID, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sessionID))
	sig := hex.EncodeToString(mac.Sum(nil))
	return sessionID + "." + sig
}

// VerifySessionID validates the HMAC signature and returns the raw session ID.
func VerifySessionID(signed, secret string) (string, bool) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return "", false
	}

	expected := SignSessionID(parts[0], secret)
	if !hmac.Equal([]byte(signed), []byte(expected)) {
		return "", false
	}

	return parts[0], true
}
