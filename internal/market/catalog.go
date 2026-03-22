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
// It pulls from the Olares appstore API first, falls back to GitHub
// (beclab/apps), then to a local catalog file, and finally uses a
// built-in default catalog so the market always has apps to show.
type Catalog struct {
	mu         sync.RWMutex
	apps       []MarketApp
	appsByName map[string]*MarketApp
	categories []Category
	lastFetch  time.Time
	cacheTTL   time.Duration

	// Appstore-provided curated data
	recommendations map[string][]string // group name -> app IDs
	topics          map[string][]string // topic name -> app IDs
	tops            []TopApp            // ranked apps
	latest          []string            // latest app IDs
	pages           map[string][]string // category name -> app IDs

	marketURL    string // upstream marketplace URL (unused now, kept for config compat)
	localPath    string // path to local catalog JSON file
	githubURL    string // GitHub API URL for the apps repo
	appstoreURL  string // Olares appstore API URL
}

// NewCatalog creates a new catalog with the given upstream URL.
func NewCatalog(marketURL, localPath string) *Catalog {
	c := &Catalog{
		appsByName:      make(map[string]*MarketApp),
		recommendations: make(map[string][]string),
		topics:          make(map[string][]string),
		pages:           make(map[string][]string),
		cacheTTL:        30 * time.Minute,
		marketURL:       marketURL,
		localPath:       localPath,
		githubURL:       "https://api.github.com/repos/beclab/apps/contents",
		appstoreURL:     "https://appstore-server-prod.bttcdn.com/api/v1/appstore/info?version=1.12.0",
	}
	return c
}

// Load fetches the catalog from all available sources in priority order:
// 1. Local catalog JSON file (fastest, user-controlled)
// 2. Olares appstore API (authoritative, 158+ apps with recommendations)
// 3. GitHub beclab/apps repo (fallback, all apps)
// 4. Built-in default catalog (always works, curated subset)
func (c *Catalog) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Try local catalog file first (user override)
	if c.localPath != "" {
		apps, err := c.loadLocal(c.localPath)
		if err == nil && len(apps) > 0 {
			c.setApps(apps)
			klog.Infof("loaded %d apps from local file %s", len(apps), c.localPath)
			return nil
		}
		if err != nil {
			klog.V(2).Infof("local catalog %s: %v", c.localPath, err)
		}
	}

	// Try default local paths
	defaultPaths := []string{
		"/etc/packalares/catalog.json",
		"/app/catalog.json",
		"/tmp/packalares-catalog.json",
		"catalog.json",
	}
	for _, p := range defaultPaths {
		apps, err := c.loadLocal(p)
		if err == nil && len(apps) > 0 {
			c.setApps(apps)
			klog.Infof("loaded %d apps from default path %s", len(apps), p)
			return nil
		}
	}

	// Try fetching from Olares appstore API
	apps, err := c.fetchFromAppstore()
	if err == nil && len(apps) > 0 {
		c.setApps(apps)
		klog.Infof("loaded %d apps from Olares appstore API", len(apps))
		c.saveCacheFile(apps)
		return nil
	}
	if err != nil {
		klog.Warningf("fetch from appstore: %v", err)
	}

	// Fall back to GitHub beclab/apps repo
	apps, err = c.fetchFromGitHub()
	if err == nil && len(apps) > 0 {
		c.setApps(apps)
		klog.Infof("loaded %d apps from GitHub beclab/apps", len(apps))
		c.saveCacheFile(apps)
		return nil
	}
	if err != nil {
		klog.Warningf("fetch from GitHub: %v", err)
	}

	// Fall back to built-in catalog
	apps = builtinCatalog()
	c.setApps(apps)
	klog.Infof("loaded %d apps from built-in catalog", len(apps))
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

// appstoreResponse is the top-level JSON envelope from the Olares appstore API.
type appstoreResponse struct {
	Data    appstoreData `json:"data"`
	Version string       `json:"version"`
}

// appstoreData holds the nested data fields from the appstore API.
type appstoreData struct {
	Apps       map[string]appstoreApp    `json:"apps"`
	Pages      map[string][]string       `json:"pages"`
	Tags       map[string][]string       `json:"tags"`
	Recommends map[string][]string       `json:"recommends"`
	Topics     map[string][]string       `json:"topics"`
	Tops       []TopApp                  `json:"tops"`
	Latest     []string                  `json:"latest"`
}

// appstoreApp is a single app entry from the appstore API.
// Field names mirror the JSON keys from the upstream API.
type appstoreApp struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Icon               string        `json:"icon"`
	Description        string        `json:"desc"`
	FullDescription    string        `json:"fullDescription"`
	UpgradeDescription string        `json:"upgradeDescription"`
	PromoteImage       []string      `json:"promoteImage"`
	PromoteVideo       string        `json:"promoteVideo"`
	SubCategory        string        `json:"subCategory"`
	Developer          string        `json:"developer"`
	Owner              string        `json:"owner"`
	UID                string        `json:"uid"`
	Title              string        `json:"title"`
	Target             string        `json:"target"`
	Version            string        `json:"version"`
	VersionName        string        `json:"versionName"`
	Categories         []string      `json:"categories"`
	Category           string        `json:"category"`
	Rating             float64       `json:"rating"`
	Namespace          string        `json:"namespace"`
	OnlyAdmin          bool          `json:"onlyAdmin"`
	RequiredMemory     string        `json:"requiredMemory"`
	RequiredDisk       string        `json:"requiredDisk"`
	RequiredGPU        string        `json:"requiredGpu"`
	RequiredCPU        string        `json:"requiredCpu"`
	LimitedMemory      string        `json:"limitedMemory"`
	LimitedCPU         string        `json:"limitedCpu"`
	SupportArch        []string      `json:"supportArch"`
	Status             string        `json:"status"`
	CfgType            string        `json:"cfgType"`
	Locale             []string      `json:"locale"`
	Doc                string        `json:"doc"`
	Website            string        `json:"website"`
	SourceCode         string        `json:"sourceCode"`
	License            []License     `json:"license"`
	InstallCount       int64         `json:"installCount"`
	LastUpdated        string        `json:"lastUpdated"`
	MobileSupported    bool          `json:"mobileSupported"`
	Entrances          []Entrance    `json:"entrances"`
	Permission         *AppPermission `json:"permission"`
	Dependencies       []Dependency  `json:"dependencies"`
}

// fetchFromAppstore fetches the full app catalog from the Olares appstore API.
// This is the preferred source: it returns 158+ apps with pre-computed
// categories, recommendations, rankings, and topics in a single request.
func (c *Catalog) fetchFromAppstore() ([]MarketApp, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", c.appstoreURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "packalares-market/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appstore request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("appstore API returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50 MB limit
	if err != nil {
		return nil, fmt.Errorf("read appstore response: %w", err)
	}

	var storeResp appstoreResponse
	if err := json.Unmarshal(body, &storeResp); err != nil {
		return nil, fmt.Errorf("parse appstore response: %w", err)
	}

	if len(storeResp.Data.Apps) == 0 {
		return nil, fmt.Errorf("appstore returned 0 apps")
	}

	// Convert appstore apps map into our MarketApp slice
	apps := make([]MarketApp, 0, len(storeResp.Data.Apps))
	for _, sa := range storeResp.Data.Apps {
		app := convertAppstoreApp(sa)
		apps = append(apps, app)
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})

	// Store the curated data from the appstore
	c.pages = storeResp.Data.Pages
	c.recommendations = storeResp.Data.Recommends
	c.topics = storeResp.Data.Topics
	c.tops = storeResp.Data.Tops
	c.latest = storeResp.Data.Latest

	return apps, nil
}

// convertAppstoreApp converts an appstore API app entry into our MarketApp.
func convertAppstoreApp(sa appstoreApp) MarketApp {
	app := MarketApp{
		Name:               sa.Name,
		CfgType:            sa.CfgType,
		ChartName:          sa.Name,
		Icon:               sa.Icon,
		Description:        sa.Description,
		FullDescription:    sa.FullDescription,
		UpgradeDescription: sa.UpgradeDescription,
		PromoteImage:       sa.PromoteImage,
		PromoteVideo:       sa.PromoteVideo,
		SubCategory:        sa.SubCategory,
		Developer:          sa.Developer,
		Owner:              sa.Owner,
		UID:                sa.UID,
		Title:              sa.Title,
		Target:             sa.Target,
		Version:            sa.Version,
		VersionName:        sa.VersionName,
		Categories:         sa.Categories,
		Rating:             sa.Rating,
		Namespace:          sa.Namespace,
		OnlyAdmin:          sa.OnlyAdmin,
		RequiredMemory:     sa.RequiredMemory,
		RequiredDisk:       sa.RequiredDisk,
		RequiredGPU:        sa.RequiredGPU,
		RequiredCPU:        sa.RequiredCPU,
		LimitedMemory:      sa.LimitedMemory,
		LimitedCPU:         sa.LimitedCPU,
		SupportArch:        sa.SupportArch,
		Status:             sa.Status,
		Locale:             sa.Locale,
		Doc:                sa.Doc,
		Website:            sa.Website,
		SourceCode:         sa.SourceCode,
		License:            sa.License,
		InstallCount:       sa.InstallCount,
		LastUpdated:        sa.LastUpdated,
		MobileSupported:    sa.MobileSupported,
		Entrances:          sa.Entrances,
		Permission:         sa.Permission,
		Dependencies:       sa.Dependencies,
		Source:             "olares",
	}

	if app.Name == "" {
		app.Name = sa.ID
	}
	if app.ChartName == "" {
		app.ChartName = app.Name
	}
	if app.Title == "" {
		app.Title = app.Name
	}

	// If categories is empty but category is set, use it
	if len(app.Categories) == 0 && sa.Category != "" {
		app.Categories = []string{sa.Category}
	}

	// Default type
	if app.CfgType == "" {
		app.CfgType = "app"
	}
	app.Type = app.CfgType

	return app
}

// fetchFromGitHub fetches app metadata from the beclab/apps GitHub repository.
// It lists all directories via the GitHub contents API, then fetches the
// OlaresManifest.yaml from each app directory.
func (c *Catalog) fetchFromGitHub() ([]MarketApp, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	// List repo contents to get app directories
	req, err := http.NewRequest("GET", c.githubURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "packalares-market/1.0")

	// Use GITHUB_TOKEN if available to avoid rate limits
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list repo contents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var entries []githubEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse directory listing: %w", err)
	}

	// Filter to directories only (each directory is an app)
	var appDirs []string
	for _, e := range entries {
		if e.Type == "dir" && !strings.HasPrefix(e.Name, ".") {
			appDirs = append(appDirs, e.Name)
		}
	}

	if len(appDirs) == 0 {
		return nil, fmt.Errorf("no app directories found in beclab/apps")
	}

	klog.Infof("found %d app directories in beclab/apps, fetching manifests...", len(appDirs))

	// Fetch manifests concurrently with bounded parallelism
	type result struct {
		app MarketApp
		ok  bool
	}

	results := make(chan result, len(appDirs))
	sem := make(chan struct{}, 10) // max 10 concurrent requests
	var wg sync.WaitGroup

	for _, dir := range appDirs {
		wg.Add(1)
		go func(appName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			app, err := c.fetchManifest(client, appName)
			if err != nil {
				klog.V(4).Infof("skip %s: %v", appName, err)
				results <- result{ok: false}
				return
			}
			results <- result{app: app, ok: true}
		}(dir)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var apps []MarketApp
	for r := range results {
		if r.ok {
			apps = append(apps, r.app)
		}
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})

	if len(apps) == 0 {
		return nil, fmt.Errorf("no valid manifests found in beclab/apps")
	}

	return apps, nil
}

// githubEntry represents a file/directory entry from the GitHub contents API.
type githubEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file" or "dir"
	Path string `json:"path"`
}

// githubFileResponse represents the response when fetching a single file.
type githubFileResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// fetchManifest fetches and parses an OlaresManifest.yaml for a single app.
func (c *Catalog) fetchManifest(client *http.Client, appName string) (MarketApp, error) {
	url := c.githubURL + "/" + appName + "/OlaresManifest.yaml"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return MarketApp{}, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "packalares-market/1.0")

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return MarketApp{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return MarketApp{}, fmt.Errorf("HTTP %d for %s", resp.StatusCode, appName)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return MarketApp{}, err
	}

	var fileResp githubFileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return MarketApp{}, fmt.Errorf("parse github response: %w", err)
	}

	yamlData, err := decodeBase64Content(fileResp.Content)
	if err != nil {
		return MarketApp{}, fmt.Errorf("decode base64: %w", err)
	}

	return parseOlaresManifest(yamlData, appName)
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

// saveCacheFile writes the fetched apps to a local cache file so subsequent
// starts are faster even if GitHub is unreachable.
func (c *Catalog) saveCacheFile(apps []MarketApp) {
	paths := []string{c.localPath, "/etc/packalares/catalog.json", "/tmp/packalares-catalog.json"}
	for _, p := range paths {
		if p == "" {
			continue
		}
		data, err := json.MarshalIndent(apps, "", "  ")
		if err != nil {
			continue
		}
		if err := os.WriteFile(p, data, 0644); err != nil {
			klog.V(4).Infof("could not cache catalog to %s: %v", p, err)
			continue
		}
		klog.Infof("cached catalog (%d apps) to %s", len(apps), p)
		return
	}
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
// When the appstore API was used, categories come from the pre-computed
// pages map which preserves the upstream ordering and app groupings.
func (c *Catalog) ListCategories() []Category {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	// If we have pages data from the appstore, build categories from it
	if len(c.pages) > 0 {
		cats := make([]Category, 0, len(c.pages))
		for name, ids := range c.pages {
			cats = append(cats, Category{Name: name, Count: len(ids)})
		}
		sort.Slice(cats, func(i, j int) bool {
			return cats[i].Name < cats[j].Name
		})
		return cats
	}

	result := make([]Category, len(c.categories))
	copy(result, c.categories)
	return result
}

// ListRecommendations returns app recommendation groups.
// Each group maps a name (e.g. "Community choices") to the resolved MarketApp
// objects for apps in that group.
func (c *Catalog) ListRecommendations() map[string][]MarketApp {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]MarketApp, len(c.recommendations))
	for groupName, appIDs := range c.recommendations {
		var apps []MarketApp
		for _, id := range appIDs {
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
