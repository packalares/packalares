package auth

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/packalares/packalares/pkg/config"
	"github.com/packalares/packalares/pkg/secrets"
)

// loadInfisicalSecrets tries to fetch secrets from Infisical vault.
// Returns nil if Infisical is not configured or unavailable.
func loadInfisicalSecrets() map[string]string {
	client := secrets.NewClient()
	s, err := client.LoadSecrets()
	if err != nil {
		log.Printf("infisical: %v (using env vars)", err)
		return nil
	}
	if s != nil {
		log.Printf("infisical: loaded %d secrets from vault", len(s))
	}
	return s
}

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
	// Try loading secrets from Infisical vault first
	infisicalSecrets := loadInfisicalSecrets()

	cfg := &Config{
		ListenAddr:     ":9091",
		UserZone:       "",
		JWTSecret:      "",
		SessionSecret:  "",
		RedisAddr:      config.KVRocksHost() + ":" + config.KVRocksPort(),
		RedisPassword:  "",
		RedisDB:        0,
		LLDAPHost:      config.LLDAPHost(),
		LLDAPPort:      17170,
		LLDAPUser:      "admin",
		LLDAPPassword:  "",
		LLDAPBaseDN:    "dc=packalares,dc=local",
		SessionName:    "packalares_session",
		SessionMaxAgeSec:  1209600, // 14 days
		SessionInactivity: 604800,  // 7 days
		TOTPIssuer:     "packalares",
		CookieSameSite: "none",
	}

	// Helper: get from Infisical first, then env var
	get := func(key string) string {
		if infisicalSecrets != nil {
			if v, ok := infisicalSecrets[key]; ok && v != "" {
				return v
			}
		}
		return os.Getenv(key)
	}

	if v := get("AUTH_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := get("USER_ZONE"); v != "" {
		cfg.UserZone = v
	}
	if v := get("JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}
	if v := get("SESSION_SECRET"); v != "" {
		cfg.SessionSecret = v
	}
	if v := get("REDIS_ADDR"); v != "" {
		cfg.RedisAddr = v
	}
	if v := get("REDIS_PASSWORD"); v != "" {
		cfg.RedisPassword = v
	}
	if v := get("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RedisDB = n
		}
	}
	if v := get("LLDAP_HOST"); v != "" {
		cfg.LLDAPHost = v
	}
	if v := get("LLDAP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LLDAPPort = n
		}
	}
	if v := get("LLDAP_USER"); v != "" {
		cfg.LLDAPUser = v
	}
	if v := get("LLDAP_PASSWORD"); v != "" {
		cfg.LLDAPPassword = v
	}
	if v := get("LLDAP_BASE_DN"); v != "" {
		cfg.LLDAPBaseDN = v
	}
	if v := get("SESSION_NAME"); v != "" {
		cfg.SessionName = v
	}
	if v := get("SESSION_MAX_AGE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SessionMaxAgeSec = n
		}
	}
	if v := get("TOTP_ISSUER"); v != "" {
		cfg.TOTPIssuer = v
	}
	if v := get("PUBLIC_DOMAINS"); v != "" {
		cfg.PublicDomains = strings.Split(v, ",")
		for i := range cfg.PublicDomains {
			cfg.PublicDomains[i] = strings.TrimSpace(cfg.PublicDomains[i])
		}
	}
	if v := get("COOKIE_SAMESITE"); v != "" {
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
