package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Config struct {
	DataPath      string
	Port          int
	Domain        string
	JWTSecret     string
	DBPath        string
	CatalogURL    string
	MaxUploadSize int64
	StaticPath    string
	CaddyAdmin    string
	CaddyfilePath string
	AppNamespace  string
	HelmPrefix    string
	MountBasePath string
}

var (
	cfg  *Config
	once sync.Once
)

func Load() *Config {
	once.Do(func() {
		cfg = &Config{
			DataPath:      "/packalares/data",
			Port:          8080,
			Domain:        "localhost",
			JWTSecret:     "",
			DBPath:        "/etc/packalares/users.db",
			CatalogURL:    "",
			MaxUploadSize: 10 << 30,
			StaticPath:    "/app/static",
			CaddyAdmin:    "http://localhost:2019",
			CaddyfilePath: "/etc/caddy/Caddyfile",
			AppNamespace:  "packalares-apps",
			HelmPrefix:    "pack-",
			MountBasePath: "/packalares/mounts",
		}

		data, err := os.ReadFile("/etc/packalares/config.yaml")
		if err == nil {
			parseYAML(data, cfg)
		}

		if v := os.Getenv("DATA_PATH"); v != "" {
			cfg.DataPath = v
		}
		if v := os.Getenv("PORT"); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				cfg.Port = p
			}
		}
		if v := os.Getenv("DOMAIN"); v != "" {
			cfg.Domain = v
		}
		if v := os.Getenv("JWT_SECRET"); v != "" {
			cfg.JWTSecret = v
		}
		if v := os.Getenv("DB_PATH"); v != "" {
			cfg.DBPath = v
		}
		if v := os.Getenv("CATALOG_URL"); v != "" {
			cfg.CatalogURL = v
		}
		if v := os.Getenv("MAX_UPLOAD_SIZE"); v != "" {
			if s, err := strconv.ParseInt(v, 10, 64); err == nil {
				cfg.MaxUploadSize = s
			}
		}
		if v := os.Getenv("STATIC_PATH"); v != "" {
			cfg.StaticPath = v
		}
		if v := os.Getenv("CADDY_ADMIN"); v != "" {
			cfg.CaddyAdmin = v
		}
		if v := os.Getenv("CADDYFILE_PATH"); v != "" {
			cfg.CaddyfilePath = v
		}
		if v := os.Getenv("APP_NAMESPACE"); v != "" {
			cfg.AppNamespace = v
		}
		if v := os.Getenv("HELM_PREFIX"); v != "" {
			cfg.HelmPrefix = v
		}
		if v := os.Getenv("MOUNT_BASE_PATH"); v != "" {
			cfg.MountBasePath = v
		}

		if cfg.JWTSecret == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				panic(fmt.Sprintf("failed to generate JWT secret: %v", err))
			}
			cfg.JWTSecret = hex.EncodeToString(b)
			_ = os.MkdirAll("/etc/packalares", 0755)
			_ = os.WriteFile("/etc/packalares/jwt_secret", []byte(cfg.JWTSecret), 0600)
		}

		_ = os.MkdirAll(cfg.DataPath, 0755)
		_ = os.MkdirAll(cfg.MountBasePath, 0755)
	})
	return cfg
}

func parseYAML(data []byte, cfg *Config) {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")

		switch key {
		case "data_path":
			cfg.DataPath = val
		case "port":
			if p, err := strconv.Atoi(val); err == nil {
				cfg.Port = p
			}
		case "domain":
			cfg.Domain = val
		case "jwt_secret":
			cfg.JWTSecret = val
		case "db_path":
			cfg.DBPath = val
		case "catalog_url":
			cfg.CatalogURL = val
		case "max_upload_size":
			if s, err := strconv.ParseInt(val, 10, 64); err == nil {
				cfg.MaxUploadSize = s
			}
		case "static_path":
			cfg.StaticPath = val
		case "caddy_admin":
			cfg.CaddyAdmin = val
		case "caddyfile_path":
			cfg.CaddyfilePath = val
		case "app_namespace":
			cfg.AppNamespace = val
		case "helm_prefix":
			cfg.HelmPrefix = val
		case "mount_base_path":
			cfg.MountBasePath = val
		}
	}
}
