package market

import (
	"os"
)

// Config holds configuration for the market backend.
type Config struct {
	MarketURL   string // upstream marketplace URL (e.g. https://market.olares.com)
	CatalogPath string // local catalog file path
	ListenAddr  string // HTTP listen address
}

// DefaultConfig returns config populated from environment variables.
func DefaultConfig() *Config {
	cfg := &Config{
		MarketURL:   "https://market.olares.com",
		CatalogPath: "/etc/packalares/catalog.json",
		ListenAddr:  ":6756",
	}

	if v := os.Getenv("MARKET_URL"); v != "" {
		cfg.MarketURL = v
	}
	if v := os.Getenv("CATALOG_PATH"); v != "" {
		cfg.CatalogPath = v
	}
	if v := os.Getenv("MARKET_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	return cfg
}
