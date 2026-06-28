package market

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"k8s.io/klog/v2"
)

const (
	modelsSubdir   = "models"
	rescanInterval = 5 * time.Second
	lruCacheSize   = 64
)

// chartEntry is a lightweight index record for one app/model.
// Tracks both the sidecar JSON and the chart .tgz so cache invalidation
// fires when either file changes.
type chartEntry struct {
	sidecarPath  string
	sidecarMtime time.Time
	tgzPath      string    // "" if no matching .tgz found (e.g. model sidecar)
	tgzMtime     time.Time // zero if tgzPath is empty
	kind         string    // "app" | "model"
}

// Catalog scans per-entity sidecar JSONs from chartsDir on demand.
// No boot-load, no goroutines spawned per request; rescans are debounced.
type Catalog struct {
	chartsDir    string
	curationPath string

	mu          sync.RWMutex
	indexedAt   time.Time
	chartIndex  map[string]chartEntry // name → entry

	parsed   *lru.Cache[string, *MarketApp] // bounded; evicts LRU on overflow
	curation *Curation                      // small; always loaded; reread on mtime change

	curationMu    sync.RWMutex
	curationMtime time.Time
}

// NewCatalog creates a Catalog rooted at the given chartsDir.
// chartsDir is e.g. /data/market/charts; models live in chartsDir/models/.
// curationPath is the path to curation.json.
func NewCatalog(chartsDir, curationPath string) *Catalog {
	cache, _ := lru.New[string, *MarketApp](lruCacheSize)
	return &Catalog{
		chartsDir:    chartsDir,
		curationPath: curationPath,
		chartIndex:   make(map[string]chartEntry),
		parsed:       cache,
	}
}

// Load performs an initial directory scan.
// Idempotent — calling it again is the same as a forced rescan.
func (c *Catalog) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rescan()
}

// ensureFresh rescans at most once per rescanInterval.
func (c *Catalog) ensureFresh() {
	c.mu.RLock()
	age := time.Since(c.indexedAt)
	c.mu.RUnlock()

	if age <= rescanInterval {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check under write lock
	if time.Since(c.indexedAt) > rescanInterval {
		if err := c.rescan(); err != nil {
			klog.Warningf("catalog rescan: %v", err)
		}
	}
}

// rescan reads chartsDir (and chartsDir/models/) to rebuild chartIndex.
// Must be called with c.mu held for writing.
func (c *Catalog) rescan() error {
	newIndex := make(map[string]chartEntry)

	// Scan apps
	if err := scanDir(c.chartsDir, "app", newIndex); err != nil && !os.IsNotExist(err) {
		klog.Warningf("catalog: scan %s: %v", c.chartsDir, err)
	}

	// Scan models
	modelsDir := filepath.Join(c.chartsDir, modelsSubdir)
	if err := scanDir(modelsDir, "model", newIndex); err != nil && !os.IsNotExist(err) {
		klog.Warningf("catalog: scan %s: %v", modelsDir, err)
	}

	// Evict LRU entries for files that changed mtime or were removed.
	// Both sidecar JSON and chart .tgz are tracked — either drift invalidates.
	for name, old := range c.chartIndex {
		newEntry, exists := newIndex[name]
		if !exists ||
			!newEntry.sidecarMtime.Equal(old.sidecarMtime) ||
			!newEntry.tgzMtime.Equal(old.tgzMtime) ||
			newEntry.tgzPath != old.tgzPath {
			c.parsed.Remove(name)
		}
	}
	// Also evict entries that are new (shouldn't be cached yet, but be safe)
	for name := range newIndex {
		if _, existed := c.chartIndex[name]; !existed {
			c.parsed.Remove(name)
		}
	}

	c.chartIndex = newIndex
	c.indexedAt = time.Now()
	klog.Infof("catalog: indexed %d entries (%s)", len(newIndex), c.indexedAt.Format(time.RFC3339))
	return nil
}

// scanDir reads all *.json files in dir and adds them to idx with the given kind.
// For each sidecar JSON it also looks up the matching chart .tgz in the same dir
// (named "<appName>-*.tgz") so the catalog can read entrances live from the chart.
// Sub-directories are not descended (models/ is handled separately by the caller).
func scanDir(dir, kind string, idx map[string]chartEntry) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	// First pass: collect .tgz mtimes by app prefix so each sidecar can pair O(1).
	tgzByApp := make(map[string]struct {
		path  string
		mtime time.Time
	})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".tgz") {
			continue
		}
		stem := strings.TrimSuffix(name, ".tgz")
		// Versioned shape "<appName>-x.y.z". The trailing version block
		// is everything after the last hyphen; the app name is the prefix.
		dash := strings.LastIndex(stem, "-")
		var appName string
		if dash > 0 {
			appName = stem[:dash]
		} else {
			appName = stem
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// If multiple versions exist, prefer the newest mtime so a stale
		// older .tgz doesn't shadow a freshly-copied one.
		if existing, ok := tgzByApp[appName]; !ok || info.ModTime().After(existing.mtime) {
			tgzByApp[appName] = struct {
				path  string
				mtime time.Time
			}{path: filepath.Join(dir, name), mtime: info.ModTime()}
		}
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		appName := strings.TrimSuffix(name, ".json")
		info, err := e.Info()
		if err != nil {
			continue
		}
		entry := chartEntry{
			sidecarPath:  filepath.Join(dir, name),
			sidecarMtime: info.ModTime(),
			kind:         kind,
		}
		if t, ok := tgzByApp[appName]; ok {
			entry.tgzPath = t.path
			entry.tgzMtime = t.mtime
		}
		idx[appName] = entry
	}
	return nil
}

// readOlaresManifestFromTGZ extracts <chartName>/OlaresManifest.yaml from a Helm
// chart .tgz and returns its raw bytes. Streams in-memory — no disk writes.
func readOlaresManifestFromTGZ(tgzPath string) ([]byte, error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(header.Name) == "OlaresManifest.yaml" && header.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("OlaresManifest.yaml not found in %s", tgzPath)
}

// ListApps returns all apps/models as a light slice.
// Each entry is parsed fresh if not in LRU cache, or stale if mtime hasn't changed.
func (c *Catalog) ListApps() []MarketApp {
	c.ensureFresh()

	c.mu.RLock()
	names := make([]string, 0, len(c.chartIndex))
	for n := range c.chartIndex {
		names = append(names, n)
	}
	c.mu.RUnlock()

	sort.Strings(names)
	result := make([]MarketApp, 0, len(names))
	for _, n := range names {
		app := c.loadOne(n)
		if app != nil {
			result = append(result, *app)
		}
	}
	return result
}

// GetApp returns a single app by name. The second return is false if not found.
func (c *Catalog) GetApp(name string) (*MarketApp, bool) {
	c.ensureFresh()
	app := c.loadOne(name)
	if app == nil {
		return nil, false
	}
	cp := *app
	return &cp, true
}

// GetAppDetail is an alias for GetApp (all data is local).
func (c *Catalog) GetAppDetail(name string) (*MarketApp, bool) {
	return c.GetApp(name)
}

// ListCategories derives categories from all apps.
func (c *Catalog) ListCategories() []Category {
	apps := c.ListApps()

	catCount := make(map[string]int)
	for _, app := range apps {
		for _, cat := range app.Categories {
			catCount[cat]++
		}
	}

	cats := make([]Category, 0, len(catCount))
	for name, count := range catCount {
		cats = append(cats, Category{Name: name, Count: count})
	}
	sort.Slice(cats, func(i, j int) bool {
		return cats[i].Name < cats[j].Name
	})
	return cats
}

// ListRecommendations returns recommendation groups with expanded app objects.
func (c *Catalog) ListRecommendations() map[string][]MarketApp {
	cur := c.loadCuration()
	if cur == nil || len(cur.Recommendations) == 0 {
		return nil
	}

	result := make(map[string][]MarketApp, len(cur.Recommendations))
	for groupName, rec := range cur.Recommendations {
		var apps []MarketApp
		for _, id := range rec.AppIDs {
			if app, ok := c.GetApp(id); ok {
				apps = append(apps, *app)
			}
		}
		if len(apps) > 0 {
			result[groupName] = apps
		}
	}
	return result
}

// Search matches apps by name, title, description, or category.
func (c *Catalog) Search(query string) []MarketApp {
	all := c.ListApps()
	if query == "" {
		return all
	}

	q := strings.ToLower(query)
	var results []MarketApp
	for _, app := range all {
		if strings.Contains(strings.ToLower(app.Name), q) ||
			strings.Contains(strings.ToLower(app.Title), q) ||
			strings.Contains(strings.ToLower(app.Description), q) ||
			matchesCategory(app.Categories, q) {
			results = append(results, app)
		}
	}
	return results
}

// StartRefreshLoop is a no-op retained for API compatibility.
// The catalog now self-refreshes lazily via ensureFresh; no goroutine needed.
func (c *Catalog) StartRefreshLoop(_ <-chan struct{}) {}

// --- internal helpers ---

// loadOne fetches a parsed MarketApp from the LRU cache, or reads from disk.
// Entrances are always sourced from the chart .tgz's OlaresManifest.yaml when
// present — the sidecar's entrances[] is treated as legacy storefront metadata
// and overridden. Sidecar and chart are independent files; either changing
// invalidates the cache entry via the mtime check below.
func (c *Catalog) loadOne(name string) *MarketApp {
	c.mu.RLock()
	entry, ok := c.chartIndex[name]
	c.mu.RUnlock()
	if !ok {
		return nil
	}

	// Stat both sources up-front so the cache validation and the read path
	// both see the same on-disk state.
	sidecarFI, sidecarErr := os.Stat(entry.sidecarPath)
	var tgzFI os.FileInfo
	if entry.tgzPath != "" {
		tgzFI, _ = os.Stat(entry.tgzPath)
	}

	// Check LRU cache: must match both sidecar AND chart mtimes.
	if cached, hit := c.parsed.Get(name); hit {
		if sidecarErr == nil &&
			sidecarFI.ModTime().Equal(entry.sidecarMtime) &&
			(entry.tgzPath == "" || (tgzFI != nil && tgzFI.ModTime().Equal(entry.tgzMtime))) {
			return cached
		}
		c.parsed.Remove(name)
	}

	// Read sidecar from disk
	data, err := os.ReadFile(entry.sidecarPath)
	if err != nil {
		klog.Warningf("catalog: read sidecar %s: %v", entry.sidecarPath, err)
		return nil
	}
	var app MarketApp
	if err := json.Unmarshal(data, &app); err != nil {
		klog.Warningf("catalog: parse sidecar %s: %v", entry.sidecarPath, err)
		return nil
	}
	app.Categories = cleanCategories(app.Categories)

	// Override entrances from the chart's OlaresManifest.yaml — the chart is
	// the single source of truth because it's what creates the live CRD.
	// Failures are non-fatal: we keep the sidecar's value as a last resort,
	// but log loud so drift is visible.
	if entry.tgzPath != "" {
		if manifestBytes, err := readOlaresManifestFromTGZ(entry.tgzPath); err != nil {
			klog.Warningf("catalog: read manifest from %s: %v", entry.tgzPath, err)
		} else if parsed, err := parseOlaresManifest(manifestBytes, name); err != nil {
			klog.Warningf("catalog: parse manifest from %s: %v", entry.tgzPath, err)
		} else {
			app.Entrances = parsed.Entrances
		}
	}

	c.parsed.Add(name, &app)
	return &app
}

// loadCuration returns curation data, re-reading from disk if mtime changed.
func (c *Catalog) loadCuration() *Curation {
	c.curationMu.RLock()
	cur := c.curation
	mtime := c.curationMtime
	c.curationMu.RUnlock()

	if fi, err := os.Stat(c.curationPath); err == nil {
		if cur != nil && fi.ModTime().Equal(mtime) {
			return cur
		}
	} else if cur != nil {
		return cur // file gone but we have cached copy
	}

	data, err := os.ReadFile(c.curationPath)
	if err != nil {
		return cur // return stale or nil
	}
	var newCur Curation
	if err := json.Unmarshal(data, &newCur); err != nil {
		klog.Warningf("catalog: parse curation.json: %v", err)
		return cur
	}

	fi, _ := os.Stat(c.curationPath)
	c.curationMu.Lock()
	c.curation = &newCur
	if fi != nil {
		c.curationMtime = fi.ModTime()
	}
	c.curationMu.Unlock()
	return &newCur
}

func matchesCategory(categories []string, query string) bool {
	for _, cat := range categories {
		if strings.Contains(strings.ToLower(cat), query) {
			return true
		}
	}
	return false
}
