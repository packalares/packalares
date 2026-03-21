package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Catalog fetches, caches, and serves app catalog data.
// It can pull from the Olares marketplace (market.olares.com) or a local file.
type Catalog struct {
	mu        sync.RWMutex
	apps      []MarketApp
	appsByName map[string]*MarketApp
	categories []Category
	lastFetch time.Time
	cacheTTL  time.Duration

	marketURL  string // upstream marketplace URL
	localPath  string // path to local catalog JSON file
}

// NewCatalog creates a new catalog with the given upstream URL.
func NewCatalog(marketURL, localPath string) *Catalog {
	c := &Catalog{
		appsByName: make(map[string]*MarketApp),
		cacheTTL:   10 * time.Minute,
		marketURL:  marketURL,
		localPath:  localPath,
	}
	return c
}

// Load fetches the catalog from remote or local sources.
func (c *Catalog) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Try remote marketplace first
	if c.marketURL != "" {
		apps, err := c.fetchRemote(c.marketURL)
		if err == nil {
			c.setApps(apps)
			klog.Infof("loaded %d apps from remote marketplace %s", len(apps), c.marketURL)
			return nil
		}
		klog.Warningf("fetch remote catalog: %v, trying local", err)
	}

	// Try local catalog file
	if c.localPath != "" {
		apps, err := c.loadLocal(c.localPath)
		if err == nil {
			c.setApps(apps)
			klog.Infof("loaded %d apps from local file %s", len(apps), c.localPath)
			return nil
		}
		klog.Warningf("load local catalog %s: %v", c.localPath, err)
	}

	// Try default paths
	defaultPaths := []string{
		"/etc/packalares/catalog.json",
		"/app/catalog.json",
		"catalog.json",
	}
	for _, p := range defaultPaths {
		apps, err := c.loadLocal(p)
		if err == nil {
			c.setApps(apps)
			klog.Infof("loaded %d apps from default path %s", len(apps), p)
			return nil
		}
	}

	// Empty catalog is ok -- will be populated when marketplace becomes available
	klog.Info("starting with empty catalog")
	c.setApps(nil)
	return nil
}

func (c *Catalog) setApps(apps []MarketApp) {
	c.apps = apps
	c.appsByName = make(map[string]*MarketApp, len(apps))
	catCount := make(map[string]int)

	for i := range apps {
		c.appsByName[apps[i].Name] = &apps[i]
		for _, cat := range apps[i].Categories {
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

	c.lastFetch = time.Now()
}

func (c *Catalog) fetchRemote(baseURL string) ([]MarketApp, error) {
	// The Olares marketplace serves a list of apps at /api/v1/apps or similar.
	// We try multiple known endpoints.
	urls := []string{
		baseURL + "/api/v1/apps",
		baseURL + "/apps",
		baseURL,
	}

	client := &http.Client{Timeout: 30 * time.Second}

	for _, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
		if err != nil {
			continue
		}

		// Try parsing as direct array
		var apps []MarketApp
		if err := json.Unmarshal(body, &apps); err == nil && len(apps) > 0 {
			return apps, nil
		}

		// Try parsing as wrapped response {"code":200,"data":[...]}
		var wrapped struct {
			Code int         `json:"code"`
			Data []MarketApp `json:"data"`
		}
		if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Data) > 0 {
			return wrapped.Data, nil
		}
	}

	return nil, fmt.Errorf("no apps found at marketplace %s", baseURL)
}

func (c *Catalog) loadLocal(path string) ([]MarketApp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var apps []MarketApp
	if err := json.Unmarshal(data, &apps); err != nil {
		// Try wrapped format
		var wrapped struct {
			Data []MarketApp `json:"data"`
		}
		if err := json.Unmarshal(data, &wrapped); err != nil {
			return nil, err
		}
		return wrapped.Data, nil
	}

	return apps, nil
}

// Refresh re-fetches the catalog if cache has expired.
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
