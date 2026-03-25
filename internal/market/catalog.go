package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// Catalog fetches, caches, and serves app catalog data.
// It pulls from the Olares appstore API first, falls back to GitHub
// (Olares community apps repo), then to a local catalog file, and finally uses a
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

	// Detail cache: enriched app data fetched from GitHub
	detailMu    sync.RWMutex
	detailCache map[string]*MarketApp // name -> enriched app

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
		detailCache:     make(map[string]*MarketApp),
		cacheTTL:        30 * time.Minute,
		marketURL:       marketURL,
		localPath:       localPath,
		githubURL:       "https://api.github.com/repos/beclab/apps/contents",
		appstoreURL:     "https://appstore-server-prod.bttcdn.com/api/v1/appstore/info?version=1.12.0",
	}
	return c
}

// Load reads the catalog from local files only. No remote requests.
// Use the sync API (POST /market/v1/sync) to fetch from external sources.
// Priority: synced catalog → user-specified path → default paths → built-in fallback.
func (c *Catalog) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Priority 1: synced catalog from chart sync manager
	localPaths := []string{
		"/data/market/catalog.json",
	}

	// Priority 2: user-specified local path
	if c.localPath != "" {
		localPaths = append(localPaths, c.localPath)
	}

	// Priority 3: default paths
	localPaths = append(localPaths,
		"/etc/packalares/catalog.json",
		"/app/catalog.json",
		"/tmp/packalares-catalog.json",
		"catalog.json",
	)

	for _, p := range localPaths {
		apps, err := c.loadLocal(p)
		if err == nil && len(apps) > 0 {
			c.setApps(apps)
			klog.Infof("loaded %d apps from local file %s", len(apps), p)
			return nil
		}
	}

	// Final fallback: built-in catalog (no network)
	apps := builtinCatalog()
	c.setApps(apps)
	klog.Infof("loaded %d apps from built-in catalog (run sync to populate)", len(apps))
	return nil
}

func (c *Catalog) setApps(apps []MarketApp) {
	c.apps = apps
	c.appsByName = make(map[string]*MarketApp, len(apps))
	catCount := make(map[string]int)

	for i := range apps {
		// Ensure categories are clean (no version suffixes)
		apps[i].Categories = cleanCategories(apps[i].Categories)
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
	Pages      map[string]json.RawMessage `json:"pages"`
	Tags       map[string]json.RawMessage `json:"tags"`
	Recommends map[string]json.RawMessage `json:"recommends"`
	Topics     map[string]json.RawMessage `json:"topics"`
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

	// Build a map from app name/id to index for efficient lookup
	appIndex := make(map[string]int, len(apps))
	for i := range apps {
		appIndex[apps[i].Name] = i
	}

	// Extract categories from pages and assign them to apps.
	// The pages structure maps category names (possibly with version suffixes
	// like "Productivity_v112") to lists of app IDs in that category.
	c.pages = make(map[string][]string)
	for catName, raw := range storeResp.Data.Pages {
		clean := catVersionSuffix.ReplaceAllString(catName, "")

		// Try to parse the page entry as a list of app IDs
		var appIDs []string
		if err := json.Unmarshal(raw, &appIDs); err != nil {
			// Might be a different structure; try as object with an "apps" field
			var page struct {
				Apps []string `json:"apps"`
			}
			if err := json.Unmarshal(raw, &page); err == nil {
				appIDs = page.Apps
			}
		}

		c.pages[clean] = appIDs

		// Assign this category to each app that appears in the page
		for _, id := range appIDs {
			if idx, ok := appIndex[id]; ok {
				apps[idx].Categories = appendUnique(apps[idx].Categories, clean)
			}
		}
	}

	// Extract recommendation group names and resolve app IDs
	c.recommendations = make(map[string][]string)
	for groupName, raw := range storeResp.Data.Recommends {
		var appIDs []string
		if err := json.Unmarshal(raw, &appIDs); err != nil {
			var group struct {
				Apps []string `json:"apps"`
			}
			if err := json.Unmarshal(raw, &group); err == nil {
				appIDs = group.Apps
			}
		}
		c.recommendations[groupName] = appIDs
	}

	c.topics = make(map[string][]string)
	for topicName, raw := range storeResp.Data.Topics {
		var appIDs []string
		if err := json.Unmarshal(raw, &appIDs); err != nil {
			var topic struct {
				Apps []string `json:"apps"`
			}
			if err := json.Unmarshal(raw, &topic); err == nil {
				appIDs = topic.Apps
			}
		}
		c.topics[topicName] = appIDs
	}
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

	// If description is empty, fall back to fullDescription or upgradeDescription
	if app.Description == "" && app.FullDescription != "" {
		// Use the first 200 chars of fullDescription as summary
		desc := app.FullDescription
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		app.Description = desc
	}
	if app.Description == "" && sa.UpgradeDescription != "" {
		app.Description = sa.UpgradeDescription
	}
	if app.Description == "" {
		app.Description = app.Title
	}

	// If categories is empty but category is set, use it
	if len(app.Categories) == 0 && sa.Category != "" {
		app.Categories = []string{sa.Category}
	}

	// Strip version suffixes like "_v112" and deduplicate categories
	app.Categories = cleanCategories(app.Categories)

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

// Refresh reloads the catalog from local files if cache has expired.
// No remote requests — only reads local catalog.json.
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
// Categories are always derived from actual app data to ensure correct counts.
// The version suffixes (e.g. "_v112") are stripped during app ingestion so
// categories like "Productivity_v112" merge into "Productivity".
func (c *Catalog) ListCategories() []Category {
	_ = c.Refresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

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

// GetAppDetail returns an app enriched with description data.
// If the app has no description or fullDescription, it fetches from GitHub.
// Results are cached so subsequent requests are fast.
func (c *Catalog) GetAppDetail(name string) (*MarketApp, bool) {
	// Check detail cache first
	c.detailMu.RLock()
	cached, hasCached := c.detailCache[name]
	c.detailMu.RUnlock()
	if hasCached {
		return cached, true
	}

	// Get the base app
	app, ok := c.GetApp(name)
	if !ok {
		return nil, false
	}

	// Copy to avoid mutating the catalog entry
	detail := *app

	// If description is missing, try to fetch from GitHub
	if detail.Description == "" || detail.FullDescription == "" {
		c.enrichFromGitHub(&detail)
	}

	// Cache the result
	c.detailMu.Lock()
	c.detailCache[name] = &detail
	c.detailMu.Unlock()

	return &detail, true
}

// enrichFromGitHub fetches OlaresManifest.yaml from GitHub and fills in missing fields.
func (c *Catalog) enrichFromGitHub(app *MarketApp) {
	rawURL := "https://raw.githubusercontent.com/beclab/apps/main/" + app.Name + "/OlaresManifest.yaml"

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		klog.V(4).Infof("enrich %s: create request: %v", app.Name, err)
		return
	}
	req.Header.Set("User-Agent", "packalares-market/1.0")

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		klog.V(4).Infof("enrich %s: fetch: %v", app.Name, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		klog.V(4).Infof("enrich %s: HTTP %d", app.Name, resp.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		klog.V(4).Infof("enrich %s: read: %v", app.Name, err)
		return
	}

	manifest, err := parseOlaresManifest(body, app.Name)
	if err != nil {
		klog.V(4).Infof("enrich %s: parse: %v", app.Name, err)
		return
	}

	// Fill in missing fields only
	if app.Description == "" && manifest.Description != "" {
		app.Description = manifest.Description
	}
	if app.FullDescription == "" && manifest.FullDescription != "" {
		app.FullDescription = manifest.FullDescription
	}
	if len(app.PromoteImage) == 0 && len(manifest.PromoteImage) > 0 {
		app.PromoteImage = manifest.PromoteImage
	}
	if app.Developer == "" && manifest.Developer != "" {
		app.Developer = manifest.Developer
	}

	klog.V(4).Infof("enriched %s from GitHub manifest", app.Name)
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

// appendUnique appends a value to a slice only if it is not already present.
func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
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
