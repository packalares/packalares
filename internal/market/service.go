package market

import (
	"os"
)

// Config holds configuration for the market backend.
type Config struct {
	// ChartsDir is the directory containing per-app *.json sidecars.
	// Models live in ChartsDir/models/.
	ChartsDir string

	// IconsDir is the directory containing 256×256 app icons.
	IconsDir string

	// ScreenshotsDir is the directory containing rich screenshot/asset subdirs.
	ScreenshotsDir string

	// CurationPath is the path to curation.json (recommendations, etc.).
	CurationPath string

	// ListenAddr is the HTTP listen address.
	ListenAddr string
}

// DefaultConfig returns config populated from environment variables with
// sensible defaults that match the Docker image layout.
func DefaultConfig() *Config {
	cfg := &Config{
		ChartsDir:      "/data/market/charts",
		IconsDir:       "/data/market/icons",
		ScreenshotsDir: "/data/market/screenshots",
		CurationPath:   "/data/market/curation.json",
		ListenAddr:     ":6756",
	}

	if v := os.Getenv("CHARTS_DIR"); v != "" {
		cfg.ChartsDir = v
	}
	if v := os.Getenv("ICONS_DIR"); v != "" {
		cfg.IconsDir = v
	}
	if v := os.Getenv("SCREENSHOTS_DIR"); v != "" {
		cfg.ScreenshotsDir = v
	}
	if v := os.Getenv("CURATION_PATH"); v != "" {
		cfg.CurationPath = v
	}
	if v := os.Getenv("MARKET_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}

	return cfg
}
