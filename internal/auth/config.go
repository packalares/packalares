package auth

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds auth service configuration, all from environment variables.
type Config struct {
	// ListenAddr is the address to listen on (default ":9091").
	ListenAddr string

	// UserZone is the user's domain zone (e.g., "alice.packalares.local").
	// Session cookies are scoped to this domain.
	UserZone string

	// JWTSecret is the HMAC key for signing JWT tokens (HS512).
	JWTSecret string

	// SessionSecret is the key used to encrypt session data in Redis.
	SessionSecret string

	// RedisAddr is the Redis address for session storage.
	RedisAddr string
	RedisPassword string
	RedisDB       int

	// LLDAP connection for user authentication.
	LLDAPHost     string
	LLDAPPort     int
	LLDAPUser     string
	LLDAPPassword string
	LLDAPBaseDN   string

	// Session settings.
	SessionName       string
	SessionMaxAgeSec  int
	SessionInactivity int

	// TOTP issuer name shown in authenticator apps.
	TOTPIssuer string

	// AccessControl rules loaded from config.
	PublicDomains []string

	// CookieSameSite controls the SameSite attribute.
	CookieSameSite string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:     ":9091",
		UserZone:       "",
		JWTSecret:      "",
		SessionSecret:  "",
		RedisAddr:      "localhost:6379",
		RedisPassword:  "",
		RedisDB:        0,
		LLDAPHost:      "lldap-service",
		LLDAPPort:      17170,
		LLDAPUser:      "admin",
		LLDAPPassword:  "adminpassword",
		LLDAPBaseDN:    "dc=example,dc=com",
		SessionName:    "packalares_session",
		SessionMaxAgeSec:  1209600, // 14 days
		SessionInactivity: 604800,  // 7 days
		TOTPIssuer:     "packalares",
		CookieSameSite: "none",
	}

	if v := os.Getenv("AUTH_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("USER_ZONE"); v != "" {
		cfg.UserZone = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}
	if v := os.Getenv("SESSION_SECRET"); v != "" {
		cfg.SessionSecret = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.RedisAddr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.RedisPassword = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RedisDB = n
		}
	}
	if v := os.Getenv("LLDAP_HOST"); v != "" {
		cfg.LLDAPHost = v
	}
	if v := os.Getenv("LLDAP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LLDAPPort = n
		}
	}
	if v := os.Getenv("LLDAP_USER"); v != "" {
		cfg.LLDAPUser = v
	}
	if v := os.Getenv("LLDAP_PASSWORD"); v != "" {
		cfg.LLDAPPassword = v
	}
	if v := os.Getenv("LLDAP_BASE_DN"); v != "" {
		cfg.LLDAPBaseDN = v
	}
	if v := os.Getenv("SESSION_NAME"); v != "" {
		cfg.SessionName = v
	}
	if v := os.Getenv("SESSION_MAX_AGE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SessionMaxAgeSec = n
		}
	}
	if v := os.Getenv("TOTP_ISSUER"); v != "" {
		cfg.TOTPIssuer = v
	}
	if v := os.Getenv("PUBLIC_DOMAINS"); v != "" {
		cfg.PublicDomains = strings.Split(v, ",")
		for i := range cfg.PublicDomains {
			cfg.PublicDomains[i] = strings.TrimSpace(cfg.PublicDomains[i])
		}
	}
	if v := os.Getenv("COOKIE_SAMESITE"); v != "" {
		cfg.CookieSameSite = v
	}

	if cfg.UserZone == "" {
		return nil, fmt.Errorf("USER_ZONE environment variable is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET environment variable is required")
	}

	return cfg, nil
}
