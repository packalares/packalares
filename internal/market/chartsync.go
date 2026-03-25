package market

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

const (
	// defaultDataDir is the base directory for synced chart data.
	defaultDataDir = "/data/market"
	chartsSubdir   = "charts"
	iconsSubdir    = "icons"
	catalogFile    = "catalog.json"
	statusFile     = "sync-status.json"
)

// SyncStatus tracks the current sync state.
type SyncStatus struct {
	State      string            `json:"state"`      // "idle", "running", "error"
	TotalApps  int               `json:"totalApps"`
	SyncedApps int               `json:"syncedApps"`
	CurrentApp string            `json:"currentApp,omitempty"`
	Errors     []string          `json:"errors"`
	LastSync   map[string]string `json:"lastSync"` // source name -> ISO timestamp
	StartedAt  string            `json:"startedAt,omitempty"`
	FinishedAt string            `json:"finishedAt,omitempty"`
}

// ChartSyncManager orchestrates chart downloads, packaging, and index generation.
type ChartSyncManager struct {
	mu       sync.RWMutex
	status   SyncStatus
	dataDir  string
	sources  map[string]Source
	catalog  *Catalog // reference to the main catalog for updating after sync
}

// NewChartSyncManager creates a sync manager with the given data directory.
func NewChartSyncManager(dataDir string, catalog *Catalog) *ChartSyncManager {
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	mgr := &ChartSyncManager{
		dataDir: dataDir,
		sources: make(map[string]Source),
		catalog: catalog,
		status: SyncStatus{
			State:    "idle",
			LastSync: make(map[string]string),
		},
	}

	// Load persisted status
	mgr.loadStatus()

	return mgr
}

// RegisterSource adds a catalog/chart source.
func (m *ChartSyncManager) RegisterSource(src Source) {
	m.sources[src.Name()] = src
}

// Status returns the current sync status.
func (m *ChartSyncManager) Status() SyncStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Return a copy
	s := m.status
	s.Errors = append([]string(nil), m.status.Errors...)
	lastSync := make(map[string]string, len(m.status.LastSync))
	for k, v := range m.status.LastSync {
		lastSync[k] = v
	}
	s.LastSync = lastSync
	return s
}

// IsRunning returns true if a sync is currently in progress.
func (m *ChartSyncManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.State == "running"
}

// SyncAll fetches catalogs and downloads charts from the specified sources.
// If sourceNames is nil/empty, all registered sources are synced.
// Runs in the calling goroutine (caller should use `go` for background).
func (m *ChartSyncManager) SyncAll(ctx context.Context, sourceNames []string) {
	if m.IsRunning() {
		klog.Warning("chart sync already running, skipping")
		return
	}

	// Determine which sources to sync
	srcs := m.resolveSources(sourceNames)
	if len(srcs) == 0 {
		klog.Warning("no sources to sync")
		return
	}

	// Ensure directories exist
	chartsDir := filepath.Join(m.dataDir, chartsSubdir)
	iconsDir := filepath.Join(m.dataDir, iconsSubdir)
	_ = os.MkdirAll(chartsDir, 0755)
	_ = os.MkdirAll(iconsDir, 0755)

	m.mu.Lock()
	m.status.State = "running"
	m.status.TotalApps = 0
	m.status.SyncedApps = 0
	m.status.CurrentApp = ""
	m.status.Errors = nil
	m.status.StartedAt = time.Now().UTC().Format(time.RFC3339)
	m.status.FinishedAt = ""
	m.mu.Unlock()

	var allApps []MarketApp

	for _, src := range srcs {
		if ctx.Err() != nil {
			break
		}

		klog.Infof("chart sync: fetching catalog from source %q", src.Name())

		apps, err := src.FetchCatalog(ctx)
		if err != nil {
			m.addError(fmt.Sprintf("fetch catalog from %s: %v", src.Name(), err))
			continue
		}

		klog.Infof("chart sync: got %d apps from %s", len(apps), src.Name())

		m.mu.Lock()
		m.status.TotalApps += len(apps)
		m.mu.Unlock()

		for i := range apps {
			if ctx.Err() != nil {
				break
			}

			app := &apps[i]

			m.mu.Lock()
			m.status.CurrentApp = app.Name
			m.mu.Unlock()

			klog.V(2).Infof("chart sync: processing %s", app.Name)

			// Check if chart .tgz already exists locally — skip download if so
			existingTgz := m.findExistingChart(chartsDir, app.Name)
			if existingTgz != "" {
				klog.V(2).Infof("chart sync: %s already cached at %s, skipping download", app.Name, existingTgz)
			} else {
				// Download chart to temp dir
				tmpDir, err := os.MkdirTemp("", "chartsync-"+app.Name+"-")
				if err != nil {
					m.addError(fmt.Sprintf("create temp dir for %s: %v", app.Name, err))
					allApps = append(allApps, *app)
					m.mu.Lock()
					m.status.SyncedApps++
					m.mu.Unlock()
					continue
				}

				chartDir := filepath.Join(tmpDir, app.Name)
				err = src.DownloadChart(ctx, app.Name, chartDir)
				if err != nil {
					klog.V(2).Infof("chart sync: skip %s: %v", app.Name, err)
					_ = os.RemoveAll(tmpDir)
					m.addError(fmt.Sprintf("download chart %s: %v", app.Name, err))
					allApps = append(allApps, *app)
					m.mu.Lock()
					m.status.SyncedApps++
					m.mu.Unlock()
					continue
				}

				// Package the chart into a .tgz
				if err := m.packageChart(chartDir, chartsDir); err != nil {
					klog.Warningf("chart sync: package %s: %v", app.Name, err)
					m.addError(fmt.Sprintf("package chart %s: %v", app.Name, err))
				}

				_ = os.RemoveAll(tmpDir)
			}

			// Cache icon if not already cached
			if app.Icon != "" && strings.HasPrefix(app.Icon, "http") {
				iconPath := filepath.Join(iconsDir, app.Name+".png")
				if _, err := os.Stat(iconPath); os.IsNotExist(err) {
					localIcon := m.cacheIcon(ctx, app.Name, app.Icon, iconsDir)
					if localIcon != "" {
						app.Icon = localIcon
					}
				} else {
					app.Icon = "/icons/" + app.Name + ".png"
				}
			}

			allApps = append(allApps, *app)

			m.mu.Lock()
			m.status.SyncedApps++
			m.mu.Unlock()
		}

		m.mu.Lock()
		m.status.LastSync[src.Name()] = time.Now().UTC().Format(time.RFC3339)
		m.mu.Unlock()
	}

	// Generate helm repo index
	m.generateIndex(chartsDir)

	// Save catalog.json
	m.saveCatalog(allApps)

	// Update the in-memory catalog
	if m.catalog != nil && len(allApps) > 0 {
		m.catalog.mu.Lock()
		m.catalog.setApps(allApps)
		m.catalog.mu.Unlock()
		klog.Infof("chart sync: updated in-memory catalog with %d apps", len(allApps))
	}

	m.mu.Lock()
	m.status.State = "idle"
	m.status.CurrentApp = ""
	m.status.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	m.mu.Unlock()

	m.saveStatus()

	klog.Infof("chart sync complete: %d/%d apps synced, %d errors",
		m.status.SyncedApps, m.status.TotalApps, len(m.status.Errors))
}

// Sync syncs a single named source.
func (m *ChartSyncManager) Sync(ctx context.Context, sourceName string) error {
	src, ok := m.sources[sourceName]
	if !ok {
		return fmt.Errorf("unknown source %q", sourceName)
	}
	m.SyncAll(ctx, []string{src.Name()})
	return nil
}

// packageChart runs `helm package` on a chart directory, or falls back to tar+gzip.
func (m *ChartSyncManager) packageChart(chartDir, outputDir string) error {
	// Try helm binary first
	helmPath, err := exec.LookPath("helm")
	if err == nil {
		cmd := exec.Command(helmPath, "package", chartDir, "-d", outputDir)
		out, err := cmd.CombinedOutput()
		if err == nil {
			klog.V(3).Infof("helm package: %s", strings.TrimSpace(string(out)))
			return nil
		}
		klog.V(2).Infof("helm package failed (%v), falling back to tar+gzip", err)
	}

	// Fallback: create .tgz manually
	return m.packageChartManual(chartDir, outputDir)
}

// packageChartManual creates a .tgz chart package using tar+gzip.
func (m *ChartSyncManager) packageChartManual(chartDir, outputDir string) error {
	chartName := filepath.Base(chartDir)

	// Read Chart.yaml for version
	version := "0.0.0"
	chartYaml, err := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
	if err == nil {
		// Simple parse for version line
		for _, line := range strings.Split(string(chartYaml), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "version:") {
				v := strings.TrimSpace(strings.TrimPrefix(line, "version:"))
				v = strings.Trim(v, `"'`)
				if v != "" {
					version = v
				}
				break
			}
		}
	}

	tgzName := fmt.Sprintf("%s-%s.tgz", chartName, version)
	tgzPath := filepath.Join(outputDir, tgzName)

	f, err := os.Create(tgzPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", tgzPath, err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Walk the chart directory and add all files
	parentDir := filepath.Dir(chartDir)
	err = filepath.Walk(chartDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Create relative path from parent of chartDir, so archive has chartName/ prefix
		relPath, err := filepath.Rel(parentDir, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})

	if err != nil {
		_ = os.Remove(tgzPath)
		return fmt.Errorf("create tar for %s: %w", chartName, err)
	}

	klog.V(3).Infof("packaged chart %s -> %s", chartName, tgzPath)
	return nil
}

// generateIndex runs `helm repo index` or generates index.yaml manually.
func (m *ChartSyncManager) generateIndex(chartsDir string) {
	helmPath, err := exec.LookPath("helm")
	if err == nil {
		cmd := exec.Command(helmPath, "repo", "index", chartsDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			klog.Warningf("helm repo index: %v: %s", err, string(out))
		} else {
			klog.V(2).Info("generated index.yaml via helm repo index")
			return
		}
	}

	// Manual fallback: create a minimal index.yaml
	entries, _ := os.ReadDir(chartsDir)
	var chartEntries []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tgz") {
			chartEntries = append(chartEntries, e.Name())
		}
	}

	indexContent := "apiVersion: v1\nentries:\n"
	for _, name := range chartEntries {
		// Extract chart name from filename (name-version.tgz)
		base := strings.TrimSuffix(name, ".tgz")
		indexContent += fmt.Sprintf("  %s:\n  - urls:\n    - %s\n", base, name)
	}
	indexContent += fmt.Sprintf("generated: %s\n", time.Now().UTC().Format(time.RFC3339))

	indexPath := filepath.Join(chartsDir, "index.yaml")
	if err := os.WriteFile(indexPath, []byte(indexContent), 0644); err != nil {
		klog.Warningf("write index.yaml: %v", err)
	} else {
		klog.V(2).Info("generated minimal index.yaml")
	}
}

// cacheIcon downloads an icon URL and saves it locally.
// Returns the relative URL path (e.g. "/icons/appname.png") or empty on failure.
func (m *ChartSyncManager) cacheIcon(ctx context.Context, appName, iconURL, iconsDir string) string {
	// Determine file extension
	ext := ".png"
	if strings.Contains(iconURL, ".svg") {
		ext = ".svg"
	} else if strings.Contains(iconURL, ".webp") {
		ext = ".webp"
	} else if strings.Contains(iconURL, ".jpg") || strings.Contains(iconURL, ".jpeg") {
		ext = ".jpg"
	}

	destPath := filepath.Join(iconsDir, appName+ext)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "packalares-market/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		klog.V(3).Infof("cache icon for %s: %v", appName, err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	f, err := os.Create(destPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	if _, err := io.Copy(f, io.LimitReader(resp.Body, 5<<20)); err != nil {
		_ = os.Remove(destPath)
		return ""
	}

	return "/icons/" + appName + ext
}

// saveCatalog writes the synced catalog to disk.
func (m *ChartSyncManager) saveCatalog(apps []MarketApp) {
	path := filepath.Join(m.dataDir, catalogFile)
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		klog.Warningf("marshal catalog: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		klog.Warningf("write catalog %s: %v", path, err)
		return
	}
	klog.Infof("saved catalog (%d apps) to %s", len(apps), path)
}

// saveStatus persists sync status to disk.
func (m *ChartSyncManager) saveStatus() {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.status, "", "  ")
	m.mu.RUnlock()

	if err != nil {
		return
	}

	path := filepath.Join(m.dataDir, statusFile)
	_ = os.WriteFile(path, data, 0644)
}

// loadStatus reads persisted sync status from disk.
func (m *ChartSyncManager) loadStatus() {
	path := filepath.Join(m.dataDir, statusFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var status SyncStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return
	}

	// Only restore last-sync times; reset running state
	m.status.LastSync = status.LastSync
	if m.status.LastSync == nil {
		m.status.LastSync = make(map[string]string)
	}
	m.status.State = "idle"
}

// resolveSources returns the source objects for the given names.
func (m *ChartSyncManager) resolveSources(names []string) []Source {
	if len(names) == 0 {
		result := make([]Source, 0, len(m.sources))
		for _, src := range m.sources {
			result = append(result, src)
		}
		return result
	}

	var result []Source
	for _, name := range names {
		if src, ok := m.sources[name]; ok {
			result = append(result, src)
		} else {
			klog.Warningf("unknown source %q", name)
		}
	}
	return result
}

// addError appends an error message to the status (thread-safe).
func (m *ChartSyncManager) addError(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Cap errors to avoid unbounded growth
	if len(m.status.Errors) < 100 {
		m.status.Errors = append(m.status.Errors, msg)
	}
}

// DataDir returns the configured data directory.
func (m *ChartSyncManager) DataDir() string {
	return m.dataDir
}

// findExistingChart checks if a .tgz for the given app already exists in chartsDir.
func (m *ChartSyncManager) findExistingChart(chartsDir, appName string) string {
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		return ""
	}
	prefix := appName + "-"
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".tgz") {
			return filepath.Join(chartsDir, e.Name())
		}
	}
	return ""
}

// GetStatus returns the current sync status (alias for Status).
func (m *ChartSyncManager) GetStatus() *SyncStatus {
	s := m.Status()
	return &s
}
