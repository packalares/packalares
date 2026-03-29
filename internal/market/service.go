package market

import (
	"os"
)

// Config holds configuration for the market backend.
type Config struct {
	CatalogPath string // local catalog file path
	ListenAddr  string // HTTP listen address
}

// DefaultConfig returns config populated from environment variables.
func DefaultConfig() *Config {
	cfg := &Config{
		CatalogPath: "/data/market/catalog.json",
		ListenAddr:  ":6756",
	}

	if v := os.Getenv("CATALOG_PATH"); v != "" {
		cfg.CatalogPath = v
	}
	if v := os.Getenv("MARKET_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	return cfg
}
