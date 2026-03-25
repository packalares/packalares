package market

import "context"

// Source is the interface for catalog and chart providers.
// Each source knows how to list apps and download their Helm charts.
type Source interface {
	// Name returns the source identifier (e.g. "olares").
	Name() string

	// FetchCatalog retrieves the full app catalog from this source.
	FetchCatalog(ctx context.Context) ([]MarketApp, error)

	// DownloadChart downloads the raw chart directory for an app
	// into destDir. The directory should contain Chart.yaml,
	// values.yaml, templates/, etc.
	DownloadChart(ctx context.Context, appName string, destDir string) error
}
