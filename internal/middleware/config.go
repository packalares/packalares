package middleware

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds middleware operator configuration.
type Config struct {
	// PostgreSQL admin connection
	PGHost     string
	PGPort     int
	PGAdminUser string
	PGAdminPassword string

	// Redis connection
	RedisHost     string
	RedisPort     int
	RedisPassword string

	// NATS connection
	NATSHost string
	NATSPort int

	// Kubernetes namespace where platform services run
	PlatformNamespace string

	// Watch namespace (empty = all namespaces)
	WatchNamespace string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		PGHost:            "citus-headless",
		PGPort:            5432,
		PGAdminUser:       "postgres",
		PGAdminPassword:   "",
		RedisHost:         "redis-cluster-proxy",
		RedisPort:         6379,
		RedisPassword:     "",
		NATSHost:          "nats",
		NATSPort:          4222,
		PlatformNamespace: "os-platform",
		WatchNamespace:    "",
	}

	if v := os.Getenv("PG_HOST"); v != "" {
		cfg.PGHost = v
	}
	if v := os.Getenv("PG_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.PGPort = n
		}
	}
	if v := os.Getenv("PG_ADMIN_USER"); v != "" {
		cfg.PGAdminUser = v
	}
	if v := os.Getenv("PG_ADMIN_PASSWORD"); v != "" {
		cfg.PGAdminPassword = v
	}
	if v := os.Getenv("REDIS_HOST"); v != "" {
		cfg.RedisHost = v
	}
	if v := os.Getenv("REDIS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RedisPort = n
		}
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.RedisPassword = v
	}
	if v := os.Getenv("NATS_HOST"); v != "" {
		cfg.NATSHost = v
	}
	if v := os.Getenv("NATS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.NATSPort = n
		}
	}
	if v := os.Getenv("PLATFORM_NAMESPACE"); v != "" {
		cfg.PlatformNamespace = v
	}
	if v := os.Getenv("WATCH_NAMESPACE"); v != "" {
		cfg.WatchNamespace = v
	}

	if cfg.PGAdminPassword == "" {
		return nil, fmt.Errorf("PG_ADMIN_PASSWORD is required")
	}

	return cfg, nil
}
