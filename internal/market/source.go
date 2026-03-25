package market

import "context"

// Source is the interface for catalog and chart providers.
// Each source knows how to list apps and download their Helm charts.
type Source interface {
	// Name returns the source identifier (e.g. "olares").
	Name() string

	// FetchCatalog retrieves the full enriched catalog from this source.
	// Returns all apps, categories, recommendations, topics, rankings, etc.
	FetchCatalog(ctx context.Context) (*EnrichedCatalog, error)

	// DownloadAll downloads all charts in bulk to destDir.
	// For sources that support it this is a single tarball download
	// instead of per-app API calls.
	DownloadAll(ctx context.Context, destDir string) error

	// DownloadChart downloads the raw chart directory for an app
	// into destDir. The directory should contain Chart.yaml,
	// values.yaml, templates/, etc. Used as a single-app fallback.
	DownloadChart(ctx context.Context, appName string, destDir string) error
}
