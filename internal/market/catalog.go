package market

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// catVersionSuffix matches version suffixes like "_v112" in category/page names.
var catVersionSuffix = regexp.MustCompile(`_v\d+$`)

// Catalog loads and serves app catalog data from local files.
// All data is self-contained: catalog.json, charts, icons, and screenshots
// are baked into the Docker image at /data/market/.
type Catalog struct {
	mu         sync.RWMutex
	apps       []MarketApp
	appsByName map[string]*MarketApp
	categories []Category
	lastFetch  time.Time
	cacheTTL   time.Duration

	// Appstore-provided curated data
	recommendations map[string]RecommendGroup
	topicLists      map[string]TopicListEntry
	tops            []TopApp
	latest          []string
	pages           map[string]PageLayout

	// Detail cache
	detailMu    sync.RWMutex
	detailCache map[string]*MarketApp // name -> enriched app

	localPath string // path to local catalog JSON file
}

// NewCatalog creates a new catalog.
func NewCatalog(localPath string) *Catalog {
	c := &Catalog{
		appsByName:      make(map[string]*MarketApp),
		recommendations: make(map[string]RecommendGroup),
		topicLists:      make(map[string]TopicListEntry),
		pages:           make(map[string]PageLayout),
		detailCache:     make(map[string]*MarketApp),
		cacheTTL:        30 * time.Minute,
		localPath:       localPath,
	}
	return c
}

// Load reads the catalog from local files only.
// Priority: /data/market/catalog.json -> user-specified path -> fallback paths -> built-in.
func (c *Catalog) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	localPaths := []string{
		"/data/market/catalog.json",
	}

	if c.localPath != "" {
		localPaths = append(localPaths, c.localPath)
	}

	localPaths = append(localPaths,
		"/etc/packalares/catalog.json",
		"/app/catalog.json",
		"/tmp/packalares-catalog.json",
		"catalog.json",
	)

	for _, p := range localPaths {
		enriched, err := c.loadLocal(p)
		if err == nil && len(enriched.Apps) > 0 {
			c.setFromEnriched(enriched)
			klog.Infof("loaded %d apps from local file %s", len(enriched.Apps), p)
			return nil
		}
	}

	// Final fallback: built-in catalog
	apps := builtinCatalog()
	c.setApps(apps)
	klog.Infof("loaded %d apps from built-in catalog", len(apps))
	return nil
}

// setFromEnriched populates the catalog from an EnrichedCatalog.
func (c *Catalog) setFromEnriched(ec *EnrichedCatalog) {
	c.apps = ec.Apps
	c.appsByName = make(map[string]*MarketApp, len(ec.Apps))
	for i := range ec.Apps {
		ec.Apps[i].Categories = cleanCategories(ec.Apps[i].Categories)
		c.appsByName[ec.Apps[i].Name] = &ec.Apps[i]
	}

	if len(ec.Categories) > 0 {
		c.categories = ec.Categories
		catCount := make(map[string]int)
		for _, app := range ec.Apps {
			for _, cat := range app.Categories {
				catCount[cat]++
			}
		}
		for i := range c.categories {
			c.categories[i].Count = catCount[c.categories[i].Name]
		}
	} else {
		c.deriveCategories()
	}

	if ec.Recommendations != nil {
		c.recommendations = ec.Recommendations
	}
	if ec.TopicLists != nil {
		c.topicLists = ec.TopicLists
	}
	if ec.Tops != nil {
		c.tops = ec.Tops
	}
	if ec.Latest != nil {
		c.latest = ec.Latest
	}
	if ec.Pages != nil {
		c.pages = ec.Pages
	}

	c.lastFetch = time.Now()
}

func (c *Catalog) setApps(apps []MarketApp) {
	c.apps = apps
	c.appsByName = make(map[string]*MarketApp, len(apps))

	for i := range apps {
		apps[i].Categories = cleanCategories(apps[i].Categories)
		c.appsByName[apps[i].Name] = &apps[i]
	}

	c.deriveCategories()
	c.lastFetch = time.Now()
}

// deriveCategories builds the category list from app data.
func (c *Catalog) deriveCategories() {
	catCount := make(map[string]int)
	for _, app := range c.apps {
		for _, cat := range app.Categories {
			catCount[cat]++
		}
	}

	c.categories = make([]Category, 0, len(catCount))
	for name, count := range catCount {
		c.categories = append(c.categories, Category{Name: name, Count: count})
	}
	sort.Slice(c.categories, func(i, j int) bool {
		return c.categories[i].Name < c.categories[j].Name
	})
}

// loadLocal loads a catalog from a local JSON file.
// Supports both the enriched format and legacy flat array format.
func (c *Catalog) loadLocal(path string) (*EnrichedCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try enriched format first (has "apps" key at top level)
	var enriched EnrichedCatalog
	if err := json.Unmarshal(data, &enriched); err != nil {
		klog.Warningf("loadLocal %s enriched parse error: %v", path, err)
	} else if len(enriched.Apps) > 0 {
		return &enriched, nil
	} else {
		klog.Warningf("loadLocal %s enriched parsed OK but 0 apps", path)
	}

	// Try flat array of apps
	var apps []MarketApp
	if err := json.Unmarshal(data, &apps); err == nil && len(apps) > 0 {
		return &EnrichedCatalog{Apps: apps}, nil
	}

	// Try wrapped format with "data" key
	var wrapped struct {
		Data []MarketApp `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Data) > 0 {
		return &EnrichedCatalog{Apps: wrapped.Data}, nil
	}

	return nil, os.ErrNotExist
}

// Refresh reloads the catalog from local files if cache has expired.
func (c *Catalog) Refresh() error {
	c.mu.RLock()
	expired := time.Since(c.lastFetch) > c.cacheTTL
	c.mu.RUnlock()

	if !expired {
		return nil
	}

	return c.Load()
}

// ListApps returns all apps in the catalog.
func (c *Catalog) ListApps() []MarketApp {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]MarketApp, len(c.apps))
	copy(result, c.apps)
	return result
}

// GetApp returns a single app by name.
func (c *Catalog) GetApp(name string) (*MarketApp, bool) {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	app, ok := c.appsByName[name]
	return app, ok
}

// ListCategories returns all known categories.
func (c *Catalog) ListCategories() []Category {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]Category, len(c.categories))
	copy(result, c.categories)
	return result
}

// ListRecommendations returns app recommendation groups.
func (c *Catalog) ListRecommendations() map[string][]MarketApp {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]MarketApp, len(c.recommendations))
	for groupName, rec := range c.recommendations {
		var apps []MarketApp
		for _, id := range rec.AppIDs {
			if app, ok := c.appsByName[id]; ok {
				apps = append(apps, *app)
			}
		}
		if len(apps) > 0 {
			result[groupName] = apps
		}
	}
	return result
}

// GetAppDetail returns the full detail for an app.
// All data is local — no remote enrichment.
func (c *Catalog) GetAppDetail(name string) (*MarketApp, bool) {
	app, ok := c.GetApp(name)
	if !ok {
		return nil, false
	}

	// Return a copy
	detail := *app
	return &detail, true
}

// Search searches apps by query string (matches name, title, description).
func (c *Catalog) Search(query string) []MarketApp {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if query == "" {
		result := make([]MarketApp, len(c.apps))
		copy(result, c.apps)
		return result
	}

	q := strings.ToLower(query)
	var results []MarketApp

	for _, app := range c.apps {
		if strings.Contains(strings.ToLower(app.Name), q) ||
			strings.Contains(strings.ToLower(app.Title), q) ||
			strings.Contains(strings.ToLower(app.Description), q) ||
			matchesCategory(app.Categories, q) {
			results = append(results, app)
		}
	}

	return results
}

func matchesCategory(categories []string, query string) bool {
	for _, cat := range categories {
		if strings.Contains(strings.ToLower(cat), query) {
			return true
		}
	}
	return false
}

// StartRefreshLoop periodically refreshes the catalog in the background.
func (c *Catalog) StartRefreshLoop(done <-chan struct{}) {
	ticker := time.NewTicker(c.cacheTTL)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := c.Load(); err != nil {
				klog.Warningf("catalog refresh: %v", err)
			}
		}
	}
}
