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

	"gopkg.in/yaml.v3"
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
//
// The new flow for each source:
//  1. FetchCatalog → get basic app list (names, icons, categories) from appstore API
//  2. DownloadAll → download entire repo tarball in ONE HTTP request, extract to tmpDir
//  3. Walk extracted app directories, parse OlaresManifest.yaml + Chart.yaml
//  4. Merge manifest data into catalog apps (enriching with full descriptions, etc.)
//  5. Package each chart into .tgz, cache icons, generate index.yaml
//  6. Save enriched catalog.json with ALL data
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

		apps, err := m.syncSource(ctx, src, chartsDir, iconsDir)
		if err != nil {
			m.addError(fmt.Sprintf("sync source %s: %v", src.Name(), err))
			continue
		}

		allApps = append(allApps, apps...)

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

// syncSource runs the full sync for a single source using the bulk tarball flow.
func (m *ChartSyncManager) syncSource(ctx context.Context, src Source, chartsDir, iconsDir string) ([]MarketApp, error) {
	// Step 1: Fetch catalog from appstore API (basic app list)
	klog.Infof("chart sync: fetching catalog from source %q", src.Name())
	catalogApps, err := src.FetchCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	klog.Infof("chart sync: got %d apps from %s catalog", len(catalogApps), src.Name())

	// Build lookup map: app name -> catalog MarketApp
	catalogMap := make(map[string]*MarketApp, len(catalogApps))
	for i := range catalogApps {
		catalogMap[catalogApps[i].Name] = &catalogApps[i]
	}

	// Step 2: Bulk download all charts via tarball
	tmpDir, err := os.MkdirTemp("", "chartsync-bulk-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	klog.Infof("chart sync: downloading all charts via tarball for source %q", src.Name())
	if err := src.DownloadAll(ctx, tmpDir); err != nil {
		return nil, fmt.Errorf("download all: %w", err)
	}

	// Step 3: Find the extracted root directory (e.g., tmpDir/apps-main/)
	extractedRoot, err := findExtractedRoot(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("find extracted root: %w", err)
	}
	klog.Infof("chart sync: extracted root is %s", extractedRoot)

	// Step 4: Walk app directories, parse manifests, merge, package
	appDirs, err := os.ReadDir(extractedRoot)
	if err != nil {
		return nil, fmt.Errorf("read extracted root: %w", err)
	}

	m.mu.Lock()
	m.status.TotalApps = len(appDirs)
	m.mu.Unlock()

	var resultApps []MarketApp

	for _, entry := range appDirs {
		if ctx.Err() != nil {
			break
		}

		// Skip hidden dirs and non-directories
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		appName := entry.Name()
		appDir := filepath.Join(extractedRoot, appName)

		// Skip non-chart dirs (no Chart.yaml)
		chartYamlPath := filepath.Join(appDir, "Chart.yaml")
		if _, err := os.Stat(chartYamlPath); os.IsNotExist(err) {
			klog.V(3).Infof("chart sync: skipping %s (no Chart.yaml)", appName)
			continue
		}

		m.mu.Lock()
		m.status.CurrentApp = appName
		m.mu.Unlock()

		klog.V(2).Infof("chart sync: processing %s", appName)

		// Parse OlaresManifest.yaml (or TerminusManifest.yaml)
		manifestApp := m.parseManifestFromDir(appDir, appName)

		// Parse Chart.yaml for chartVersion and appVersion
		chartVersion, appVersion := m.parseChartYaml(appDir)

		// Start with the catalog entry if it exists, otherwise use the manifest data
		var app MarketApp
		if catalogEntry, ok := catalogMap[appName]; ok {
			app = *catalogEntry
			// Enrich catalog data with manifest data where the catalog is missing info
			mergeManifestIntoCatalog(&app, &manifestApp)
		} else {
			// App exists in repo but not in appstore API — use manifest data entirely
			app = manifestApp
		}

		// Set chart version info from Chart.yaml
		if chartVersion != "" && app.Version == "" {
			app.Version = chartVersion
		}
		if appVersion != "" && app.VersionName == "" {
			app.VersionName = appVersion
		}

		// Ensure source is set
		if app.Source == "" {
			app.Source = src.Name()
		}

		// Cache icon from CDN URL
		if app.Icon != "" && strings.HasPrefix(app.Icon, "http") {
			iconPath := filepath.Join(iconsDir, appName+".png")
			if _, err := os.Stat(iconPath); os.IsNotExist(err) {
				localIcon := m.cacheIcon(ctx, appName, app.Icon, iconsDir)
				if localIcon != "" {
					app.Icon = localIcon
				}
			} else {
				app.Icon = "/icons/" + appName + ".png"
			}
		}

		// Check if chart .tgz already exists — skip packaging if so
		existingTgz := m.findExistingChart(chartsDir, appName)
		if existingTgz != "" {
			klog.V(2).Infof("chart sync: %s already cached at %s, skipping packaging", appName, existingTgz)
			app.HasChart = true
		} else {
			// Package the chart directory into a .tgz
			if err := m.packageChart(appDir, chartsDir); err != nil {
				klog.Warningf("chart sync: package %s: %v", appName, err)
				m.addError(fmt.Sprintf("package chart %s: %v", appName, err))
			} else {
				app.HasChart = true
			}
		}

		// Remove from catalogMap so we can track which catalog-only apps have no chart
		delete(catalogMap, appName)

		resultApps = append(resultApps, app)

		m.mu.Lock()
		m.status.SyncedApps++
		m.mu.Unlock()
	}

	// Add any remaining catalog apps that had no chart directory in the repo
	// (they still appear in the catalog but without HasChart)
	for _, catalogApp := range catalogMap {
		// Cache icon
		if catalogApp.Icon != "" && strings.HasPrefix(catalogApp.Icon, "http") {
			iconPath := filepath.Join(iconsDir, catalogApp.Name+".png")
			if _, err := os.Stat(iconPath); os.IsNotExist(err) {
				localIcon := m.cacheIcon(ctx, catalogApp.Name, catalogApp.Icon, iconsDir)
				if localIcon != "" {
					catalogApp.Icon = localIcon
				}
			} else {
				catalogApp.Icon = "/icons/" + catalogApp.Name + ".png"
			}
		}
		resultApps = append(resultApps, *catalogApp)
	}

	return resultApps, nil
}

// findExtractedRoot finds the single top-level directory inside the extraction dir.
// GitHub tarballs extract to a directory like "apps-main/".
func findExtractedRoot(tmpDir string) (string, error) {
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", err
	}

	// Look for the first directory entry (should be "apps-main" or similar)
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			return filepath.Join(tmpDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no extracted root directory found in %s", tmpDir)
}

// parseManifestFromDir reads OlaresManifest.yaml (or TerminusManifest.yaml) from
// an app directory and returns a MarketApp with the parsed data.
func (m *ChartSyncManager) parseManifestFromDir(appDir, appName string) MarketApp {
	// Try OlaresManifest.yaml first, then TerminusManifest.yaml (legacy)
	candidates := []string{
		filepath.Join(appDir, "OlaresManifest.yaml"),
		filepath.Join(appDir, "TerminusManifest.yaml"),
	}

	var data []byte
	var readErr error
	for _, path := range candidates {
		data, readErr = os.ReadFile(path)
		if readErr == nil {
			break
		}
	}

	if readErr != nil {
		klog.V(3).Infof("chart sync: no manifest for %s: %v", appName, readErr)
		return MarketApp{Name: appName, ChartName: appName, Source: "olares"}
	}

	app, err := parseOlaresManifest(data, appName)
	if err != nil {
		klog.V(2).Infof("chart sync: parse manifest for %s: %v", appName, err)
		return MarketApp{Name: appName, ChartName: appName, Source: "olares"}
	}

	return app
}

// parseChartYaml reads Chart.yaml and returns (chartVersion, appVersion).
func (m *ChartSyncManager) parseChartYaml(appDir string) (string, string) {
	data, err := os.ReadFile(filepath.Join(appDir, "Chart.yaml"))
	if err != nil {
		return "", ""
	}

	var chart struct {
		Version    string `yaml:"version"`
		AppVersion string `yaml:"appVersion"`
	}
	if err := yaml.Unmarshal(data, &chart); err != nil {
		return "", ""
	}

	return chart.Version, chart.AppVersion
}

// mergeManifestIntoCatalog enriches a catalog MarketApp with data from the
// manifest where the catalog entry is missing information. The catalog data
// (from the appstore API) takes priority for fields it already has; the
// manifest fills in the gaps.
func mergeManifestIntoCatalog(catalog, manifest *MarketApp) {
	if catalog.FullDescription == "" && manifest.FullDescription != "" {
		catalog.FullDescription = manifest.FullDescription
	}
	if catalog.UpgradeDescription == "" && manifest.UpgradeDescription != "" {
		catalog.UpgradeDescription = manifest.UpgradeDescription
	}
	if len(catalog.PromoteImage) == 0 && len(manifest.PromoteImage) > 0 {
		catalog.PromoteImage = manifest.PromoteImage
	}
	if catalog.PromoteVideo == "" && manifest.PromoteVideo != "" {
		catalog.PromoteVideo = manifest.PromoteVideo
	}
	if catalog.Developer == "" && manifest.Developer != "" {
		catalog.Developer = manifest.Developer
	}
	if catalog.Website == "" && manifest.Website != "" {
		catalog.Website = manifest.Website
	}
	if catalog.Doc == "" && manifest.Doc != "" {
		catalog.Doc = manifest.Doc
	}
	if catalog.SourceCode == "" && manifest.SourceCode != "" {
		catalog.SourceCode = manifest.SourceCode
	}
	if len(catalog.License) == 0 && len(manifest.License) > 0 {
		catalog.License = manifest.License
	}
	if catalog.RequiredMemory == "" && manifest.RequiredMemory != "" {
		catalog.RequiredMemory = manifest.RequiredMemory
	}
	if catalog.RequiredCPU == "" && manifest.RequiredCPU != "" {
		catalog.RequiredCPU = manifest.RequiredCPU
	}
	if catalog.RequiredDisk == "" && manifest.RequiredDisk != "" {
		catalog.RequiredDisk = manifest.RequiredDisk
	}
	if catalog.RequiredGPU == "" && manifest.RequiredGPU != "" {
		catalog.RequiredGPU = manifest.RequiredGPU
	}
	if catalog.LimitedMemory == "" && manifest.LimitedMemory != "" {
		catalog.LimitedMemory = manifest.LimitedMemory
	}
	if catalog.LimitedCPU == "" && manifest.LimitedCPU != "" {
		catalog.LimitedCPU = manifest.LimitedCPU
	}
	if len(catalog.SupportArch) == 0 && len(manifest.SupportArch) > 0 {
		catalog.SupportArch = manifest.SupportArch
	}
	if len(catalog.Locale) == 0 && len(manifest.Locale) > 0 {
		catalog.Locale = manifest.Locale
	}
	if len(catalog.Entrances) == 0 && len(manifest.Entrances) > 0 {
		catalog.Entrances = manifest.Entrances
	}
	if catalog.Permission == nil && manifest.Permission != nil {
		catalog.Permission = manifest.Permission
	}
	if len(catalog.Dependencies) == 0 && len(manifest.Dependencies) > 0 {
		catalog.Dependencies = manifest.Dependencies
	}
	if catalog.VersionName == "" && manifest.VersionName != "" {
		catalog.VersionName = manifest.VersionName
	}
	if catalog.Version == "" && manifest.Version != "" {
		catalog.Version = manifest.Version
	}
	if catalog.Icon == "" && manifest.Icon != "" {
		catalog.Icon = manifest.Icon
	}
	if catalog.Description == "" && manifest.Description != "" {
		catalog.Description = manifest.Description
	}
	if catalog.Title == "" && manifest.Title != "" {
		catalog.Title = manifest.Title
	}
	if len(catalog.Categories) == 0 && len(manifest.Categories) > 0 {
		catalog.Categories = manifest.Categories
	}
	if catalog.Target == "" && manifest.Target != "" {
		catalog.Target = manifest.Target
	}
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
