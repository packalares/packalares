package systemserver

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds system server configuration.
type Config struct {
	// ListenAddr is the HTTP API listen address.
	ListenAddr string

	// UserZone is the user's domain zone.
	UserZone string

	// Username is the system user.
	Username string

	// Namespace is the user's Kubernetes namespace.
	UserNamespace string

	// NginxConfigPath is where generated nginx configs are written.
	NginxConfigPath string

	// NginxReloadCmd is the command to reload nginx.
	NginxReloadCmd string

	// WatchNamespace for Application CRDs.
	WatchNamespace string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:      ":8080",
		UserZone:        "",
		Username:        "",
		UserNamespace:   "",
		NginxConfigPath: "/etc/nginx/conf.d",
		NginxReloadCmd:  "nginx -s reload",
		WatchNamespace:  "",
	}

	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("USER_ZONE"); v != "" {
		cfg.UserZone = v
	}
	if v := os.Getenv("USERNAME"); v != "" {
		cfg.Username = v
	}
	if v := os.Getenv("USER_NAMESPACE"); v != "" {
		cfg.UserNamespace = v
	}
	if v := os.Getenv("NGINX_CONFIG_PATH"); v != "" {
		cfg.NginxConfigPath = v
	}
	if v := os.Getenv("NGINX_RELOAD_CMD"); v != "" {
		cfg.NginxReloadCmd = v
	}
	if v := os.Getenv("WATCH_NAMESPACE"); v != "" {
		cfg.WatchNamespace = v
	}

	// Allow PORT as alternative
	if v := os.Getenv("PORT"); v != "" {
		if _, err := strconv.Atoi(v); err == nil {
			cfg.ListenAddr = ":" + v
		}
	}

	if cfg.UserZone == "" {
		return nil, fmt.Errorf("USER_ZONE is required")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("USERNAME is required")
	}

	return cfg, nil
}
