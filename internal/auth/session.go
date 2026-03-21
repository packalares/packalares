package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SessionStore manages sessions in Redis.
type SessionStore struct {
	addr     string
	password string
	db       int
	prefix   string
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
	return &SessionStore{
		addr:     addr,
		password: password,
		db:       db,
		prefix:   "packalares:session:",
	}
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
	if err := s.redisSetEx(ctx, key, string(encoded), int(ttl.Seconds())); err != nil {
		return "", fmt.Errorf("redis set: %w", err)
	}

	return sessionID, nil
}

// Get retrieves a session by ID.
func (s *SessionStore) Get(ctx context.Context, sessionID string) (*SessionData, error) {
	key := s.prefix + sessionID
	val, err := s.redisGet(ctx, key)
	if err != nil {
		return nil, err
	}
	if val == "" {
		return nil, fmt.Errorf("session not found")
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
	return s.redisSetEx(ctx, key, string(encoded), int(ttl.Seconds()))
}

// Delete removes a session.
func (s *SessionStore) Delete(ctx context.Context, sessionID string) error {
	key := s.prefix + sessionID
	return s.redisDel(ctx, key)
}

// Touch updates the session TTL without changing data.
func (s *SessionStore) Touch(ctx context.Context, sessionID string, ttl time.Duration) error {
	key := s.prefix + sessionID
	return s.redisExpire(ctx, key, int(ttl.Seconds()))
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

// Minimal Redis protocol client (no external dependency).
// Supports AUTH, SELECT, GET, SET, SETEX, DEL, EXPIRE.

func (s *SessionStore) redisConn(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, err
	}

	if s.password != "" {
		if err := redisCmd(conn, "AUTH", s.password); err != nil {
			conn.Close()
			return nil, fmt.Errorf("redis AUTH: %w", err)
		}
	}

	if s.db != 0 {
		if err := redisCmd(conn, "SELECT", strconv.Itoa(s.db)); err != nil {
			conn.Close()
			return nil, fmt.Errorf("redis SELECT: %w", err)
		}
	}

	return conn, nil
}

func (s *SessionStore) redisSetEx(ctx context.Context, key, value string, seconds int) error {
	conn, err := s.redisConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return redisCmd(conn, "SETEX", key, strconv.Itoa(seconds), value)
}

func (s *SessionStore) redisGet(ctx context.Context, key string) (string, error) {
	conn, err := s.redisConn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return redisGetCmd(conn, key)
}

func (s *SessionStore) redisDel(ctx context.Context, key string) error {
	conn, err := s.redisConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return redisCmd(conn, "DEL", key)
}

func (s *SessionStore) redisExpire(ctx context.Context, key string, seconds int) error {
	conn, err := s.redisConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return redisCmd(conn, "EXPIRE", key, strconv.Itoa(seconds))
}

func redisCmd(conn net.Conn, args ...string) error {
	if err := writeRESPArray(conn, args); err != nil {
		return err
	}
	return readRESPOK(conn)
}

func redisGetCmd(conn net.Conn, key string) (string, error) {
	if err := writeRESPArray(conn, []string{"GET", key}); err != nil {
		return "", err
	}
	return readRESPBulkString(conn)
}

func writeRESPArray(w io.Writer, args []string) error {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(args)))
	for _, arg := range args {
		buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}
	_, err := io.WriteString(w, buf.String())
	return err
}

func readRESPOK(r io.Reader) error {
	line, err := readLine(r)
	if err != nil {
		return err
	}
	if len(line) == 0 {
		return fmt.Errorf("empty response")
	}
	switch line[0] {
	case '+':
		return nil // +OK
	case '-':
		return fmt.Errorf("redis error: %s", line[1:])
	case ':':
		return nil // integer response
	case '$':
		// bulk string - read and discard
		n, _ := strconv.Atoi(line[1:])
		if n > 0 {
			buf := make([]byte, n+2) // +2 for \r\n
			io.ReadFull(r, buf)
		}
		return nil
	default:
		return nil
	}
}

func readRESPBulkString(r io.Reader) (string, error) {
	line, err := readLine(r)
	if err != nil {
		return "", err
	}
	if len(line) == 0 {
		return "", fmt.Errorf("empty response")
	}
	switch line[0] {
	case '$':
		n, _ := strconv.Atoi(line[1:])
		if n < 0 {
			return "", nil // nil bulk string
		}
		buf := make([]byte, n+2)
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	case '-':
		return "", fmt.Errorf("redis error: %s", line[1:])
	default:
		return "", fmt.Errorf("unexpected response type: %c", line[0])
	}
}

func readLine(r io.Reader) (string, error) {
	var buf []byte
	b := make([]byte, 1)
	for {
		_, err := r.Read(b)
		if err != nil {
			return "", err
		}
		if b[0] == '\n' {
			if len(buf) > 0 && buf[len(buf)-1] == '\r' {
				buf = buf[:len(buf)-1]
			}
			return string(buf), nil
		}
		buf = append(buf, b[0])
	}
}
