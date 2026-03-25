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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	// olaresRepoTarballURL is the single tarball download for the entire apps repo.
	// This avoids per-file GitHub API calls and their rate limits.
	olaresRepoTarballURL = "https://github.com/beclab/apps/archive/refs/heads/main.tar.gz"
)

// OlaresSource fetches catalog and charts from the upstream Olares ecosystem:
// appstore API for catalog, GitHub beclab/apps tarball for charts.
type OlaresSource struct {
	appstoreURL string
	githubAPI   string
	repo        string
	tarballURL  string
	httpClient  *http.Client
}

// NewOlaresSource creates an Olares source with default URLs.
func NewOlaresSource() *OlaresSource {
	return &OlaresSource{
		appstoreURL: "https://appstore-server-prod.bttcdn.com/api/v1/appstore/info?version=1.12.0",
		githubAPI:   "https://api.github.com",
		repo:        "beclab/apps",
		tarballURL:  olaresRepoTarballURL,
		httpClient:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (s *OlaresSource) Name() string { return "olares" }

// FetchCatalog fetches the full enriched catalog from the Olares appstore API.
// It reads ALL sections: apps, topics, recommends, tags, tops, latest, pages,
// topic_lists — and assembles the master app list from all of them.
func (s *OlaresSource) FetchCatalog(ctx context.Context) (*EnrichedCatalog, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.appstoreURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "packalares-market/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appstore request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("appstore API returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil, fmt.Errorf("read appstore response: %w", err)
	}

	var storeResp olaresSourceResponse
	if err := json.Unmarshal(body, &storeResp); err != nil {
		return nil, fmt.Errorf("parse appstore response: %w", err)
	}

	if len(storeResp.Data.Apps) == 0 {
		return nil, fmt.Errorf("appstore returned 0 apps")
	}

	// Step 1: Build master app map starting with data.apps
	appMap := make(map[string]*MarketApp, len(storeResp.Data.Apps))
	for id, sa := range storeResp.Data.Apps {
		app := convertOlaresSourceApp(sa)
		if app.Name == "" {
			app.Name = id
		}
		appMap[app.Name] = &app
	}
	klog.Infof("olares: %d apps from data.apps", len(appMap))

	// Step 2: Parse topics — extract app references and enrich with topic metadata
	topicAppsMapping := make(map[string]*olaresTopicData) // topic key -> parsed topic data
	for topicKey, topicRaw := range storeResp.Data.Topics {
		var topic olaresTopicEntry
		if err := json.Unmarshal(topicRaw, &topic); err != nil {
			klog.V(3).Infof("olares: skip topic %s: parse error: %v", topicKey, err)
			continue
		}

		enUS := topic.Data.EnUS
		topicAppsMapping[topicKey] = &enUS

		// The "apps" field is a comma-separated string of chart names
		if enUS.Apps != "" {
			for _, chartName := range strings.Split(enUS.Apps, ",") {
				chartName = strings.TrimSpace(chartName)
				if chartName == "" {
					continue
				}
				if _, exists := appMap[chartName]; !exists {
					// New app discovered from topic — create a stub
					app := &MarketApp{
						Name:      chartName,
						ChartName: chartName,
						Source:    "olares",
						Status:   "active",
						Type:     "app",
						CfgType:  "app",
					}
					// Enrich from topic metadata
					if enUS.Title != "" {
						app.Title = enUS.Title
					}
					if enUS.Des != "" {
						app.Description = enUS.Des
					}
					if enUS.IconImg != "" {
						app.FeaturedImage = enUS.IconImg
					}
					appMap[chartName] = app
					klog.V(2).Infof("olares: discovered app %q from topic %q", chartName, topicKey)
				} else {
					// App already exists — enrich it with topic metadata
					existing := appMap[chartName]
					if existing.FeaturedImage == "" && enUS.IconImg != "" {
						existing.FeaturedImage = enUS.IconImg
					}
					if existing.Title == existing.Name && enUS.Title != "" {
						existing.Title = enUS.Title
					}
				}
			}
		}
	}

	// Step 3: Parse recommends — add any new app references
	recommendations := make(map[string]RecommendGroup)
	for recKey, recRaw := range storeResp.Data.Recommends {
		var rec olaresRecommendEntry
		if err := json.Unmarshal(recRaw, &rec); err != nil {
			klog.V(3).Infof("olares: skip recommend %s: parse error: %v", recKey, err)
			continue
		}

		var appIDs []string
		if rec.Content != "" {
			for _, name := range strings.Split(rec.Content, ",") {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				appIDs = append(appIDs, name)
				if _, exists := appMap[name]; !exists {
					appMap[name] = &MarketApp{
						Name:      name,
						ChartName: name,
						Source:    "olares",
						Status:   "active",
						Type:     "app",
						CfgType:  "app",
					}
					klog.V(2).Infof("olares: discovered app %q from recommend %q", name, recKey)
				}
			}
		}

		rg := RecommendGroup{
			Name:        rec.Name,
			Description: rec.Description,
			AppIDs:      appIDs,
		}
		if rec.Data.Title.EnUS != "" || rec.Data.Title.ZhCN != "" {
			rg.Title = make(map[string]string)
			if rec.Data.Title.EnUS != "" {
				rg.Title["en-US"] = rec.Data.Title.EnUS
			}
			if rec.Data.Title.ZhCN != "" {
				rg.Title["zh-CN"] = rec.Data.Title.ZhCN
			}
		}
		recommendations[recKey] = rg
	}

	// Step 4: Parse tops — add any new app IDs
	var tops []TopApp
	for _, topRaw := range storeResp.Data.Tops {
		var top TopApp
		if err := json.Unmarshal(topRaw, &top); err != nil {
			continue
		}
		tops = append(tops, top)
		if top.AppID != "" {
			if _, exists := appMap[top.AppID]; !exists {
				appMap[top.AppID] = &MarketApp{
					Name:      top.AppID,
					ChartName: top.AppID,
					Source:    "olares",
					Status:   "active",
					Type:     "app",
					CfgType:  "app",
				}
				klog.V(2).Infof("olares: discovered app %q from tops", top.AppID)
			}
		}
	}

	// Step 5: Parse latest — add any new app IDs
	var latest []string
	for _, latestRaw := range storeResp.Data.Latest {
		var appID string
		if err := json.Unmarshal(latestRaw, &appID); err != nil {
			continue
		}
		latest = append(latest, appID)
		if _, exists := appMap[appID]; !exists {
			appMap[appID] = &MarketApp{
				Name:      appID,
				ChartName: appID,
				Source:    "olares",
				Status:   "active",
				Type:     "app",
				CfgType:  "app",
			}
			klog.V(2).Infof("olares: discovered app %q from latest", appID)
		}
	}

	// Step 6: Build categories from data.tags (the 8 real sidebar categories)
	var categories []Category
	for tagKey, tagRaw := range storeResp.Data.Tags {
		var tag olaresTagEntry
		if err := json.Unmarshal(tagRaw, &tag); err != nil {
			klog.V(3).Infof("olares: skip tag %s: parse error: %v", tagKey, err)
			continue
		}
		// Strip version suffixes like "_v112" from tag names
		tagName := catVersionSuffix.ReplaceAllString(tag.Name, "")
		cat := Category{
			Name: tagName,
			Icon: tag.Icon,
			Sort: tag.Sort,
		}
		if tag.Title.EnUS != "" || tag.Title.ZhCN != "" {
			cat.Title = make(map[string]string)
			if tag.Title.EnUS != "" {
				cat.Title["en-US"] = tag.Title.EnUS
			}
			if tag.Title.ZhCN != "" {
				cat.Title["zh-CN"] = tag.Title.ZhCN
			}
		}
		categories = append(categories, cat)
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Sort < categories[j].Sort
	})

	// Step 7: Parse topic_lists
	topicLists := make(map[string]TopicListEntry)
	for tlKey, tlRaw := range storeResp.Data.TopicLists {
		var tl olaresTopicListEntry
		if err := json.Unmarshal(tlRaw, &tl); err != nil {
			klog.V(3).Infof("olares: skip topic_list %s: parse error: %v", tlKey, err)
			continue
		}
		var topicIDs []string
		if tl.Content != "" {
			for _, id := range strings.Split(tl.Content, ",") {
				id = strings.TrimSpace(id)
				if id != "" {
					topicIDs = append(topicIDs, id)
				}
			}
		}
		entry := TopicListEntry{
			Name:        tl.Name,
			Type:        tl.Type,
			Description: tl.Description,
			TopicIDs:    topicIDs,
		}
		if tl.Title.EnUS != "" || tl.Title.ZhCN != "" {
			entry.Title = make(map[string]string)
			if tl.Title.EnUS != "" {
				entry.Title["en-US"] = tl.Title.EnUS
			}
			if tl.Title.ZhCN != "" {
				entry.Title["zh-CN"] = tl.Title.ZhCN
			}
		}
		topicLists[tlKey] = entry
	}

	// Step 8: Parse pages
	pages := make(map[string]PageLayout)
	for pageKey, pageRaw := range storeResp.Data.Pages {
		var page olaresPageEntry
		if err := json.Unmarshal(pageRaw, &page); err != nil {
			klog.V(3).Infof("olares: skip page %s: parse error: %v", pageKey, err)
			continue
		}
		pages[pageKey] = PageLayout{
			Category: page.Category,
			Content:  page.Content,
		}
	}

	// Step 9: Convert map to sorted slice
	apps := make([]MarketApp, 0, len(appMap))
	for _, app := range appMap {
		// Ensure defaults
		if app.Title == "" {
			app.Title = app.Name
		}
		if app.ChartName == "" {
			app.ChartName = app.Name
		}
		if app.Description == "" {
			app.Description = app.Title
		}
		apps = append(apps, *app)
	}
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})

	// Compute category counts from the app data
	catCount := make(map[string]int)
	for _, app := range apps {
		for _, cat := range app.Categories {
			catCount[cat]++
		}
	}
	for i := range categories {
		categories[i].Count = catCount[categories[i].Name]
	}

	klog.Infof("olares: total %d apps (%d from data.apps + %d discovered), %d categories, %d recommendations, %d tops, %d latest",
		len(apps), len(storeResp.Data.Apps), len(apps)-len(storeResp.Data.Apps),
		len(categories), len(recommendations), len(tops), len(latest))

	return &EnrichedCatalog{
		Apps:            apps,
		Categories:      categories,
		Recommendations: recommendations,
		TopicLists:      topicLists,
		Tops:            tops,
		Latest:          latest,
		Pages:           pages,
	}, nil
}

// DownloadAll downloads the entire beclab/apps repository as a single tarball
// and extracts it into destDir. This creates destDir/apps-main/ with all app
// directories inside. One HTTP request, no API rate limiting.
func (s *OlaresSource) DownloadAll(ctx context.Context, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest dir %s: %w", destDir, err)
	}

	klog.Infof("olares: downloading repo tarball from %s", s.tarballURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.tarballURL, nil)
	if err != nil {
		return fmt.Errorf("create tarball request: %w", err)
	}
	req.Header.Set("User-Agent", "packalares-market/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download tarball: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("tarball download returned %d: %s", resp.StatusCode, string(body))
	}

	// Stream-extract the tarball directly (no temp file needed)
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	fileCount := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// Sanitize path to prevent directory traversal
		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", target, err)
			}
			outFile, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}
			outFile.Close()
			fileCount++
		}
	}

	klog.Infof("olares: extracted %d files from tarball into %s", fileCount, destDir)
	return nil
}

// DownloadChart downloads a single chart directory from the GitHub beclab/apps repo.
// This is the per-app fallback — prefer DownloadAll for bulk sync.
func (s *OlaresSource) DownloadChart(ctx context.Context, appName string, destDir string) error {
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest dir %s: %w", destDir, err)
	}

	klog.V(2).Infof("olares: downloading chart %s from %s", appName, s.repo)
	return s.downloadDir(ctx, appName, destDir)
}

// downloadDir recursively downloads all files from a GitHub directory.
func (s *OlaresSource) downloadDir(ctx context.Context, repoPath, localDir string) error {
	url := fmt.Sprintf("%s/repos/%s/contents/%s", s.githubAPI, s.repo, repoPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "packalares-market/1.0")

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("chart %q not found in %s (404)", repoPath, s.repo)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("GitHub API returned %d for %s: %s", resp.StatusCode, repoPath, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response for %s: %w", repoPath, err)
	}

	var contents []olaresGithubContent
	if err := json.Unmarshal(body, &contents); err != nil {
		return fmt.Errorf("parse GitHub response for %s: %w", repoPath, err)
	}

	for _, entry := range contents {
		localPath := filepath.Join(localDir, entry.Name)

		switch entry.Type {
		case "file":
			if err := s.downloadFile(ctx, entry.DownloadURL, localPath); err != nil {
				return fmt.Errorf("download %s: %w", entry.Path, err)
			}
		case "dir":
			if err := os.MkdirAll(localPath, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", localPath, err)
			}
			if err := s.downloadDir(ctx, entry.Path, localPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// downloadFile downloads a single file from its raw download URL.
func (s *OlaresSource) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch file %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write %s: %w", destPath, err)
	}

	return nil
}

// --- internal types mirroring the appstore API ---

type olaresGithubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

// olaresSourceResponse is the top-level API response envelope.
type olaresSourceResponse struct {
	Data olaresSourceData `json:"data"`
}

// olaresSourceData holds ALL nested data fields from the appstore API.
type olaresSourceData struct {
	Apps       map[string]olaresSourceApp    `json:"apps"`
	Topics     map[string]json.RawMessage    `json:"topics"`
	Recommends map[string]json.RawMessage    `json:"recommends"`
	Tags       map[string]json.RawMessage    `json:"tags"`
	Pages      map[string]json.RawMessage    `json:"pages"`
	TopicLists map[string]json.RawMessage    `json:"topic_lists"`
	Tops       []json.RawMessage             `json:"tops"`
	Latest     []json.RawMessage             `json:"latest"`
}

type olaresSourceApp struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Icon               string          `json:"icon"`
	Description        string          `json:"desc"`
	FullDescription    string          `json:"fullDescription"`
	UpgradeDescription string          `json:"upgradeDescription"`
	PromoteImage       []string        `json:"promoteImage"`
	PromoteVideo       string          `json:"promoteVideo"`
	SubCategory        string          `json:"subCategory"`
	Developer          string          `json:"developer"`
	Owner              string          `json:"owner"`
	UID                string          `json:"uid"`
	Title              string          `json:"title"`
	Target             string          `json:"target"`
	Version            string          `json:"version"`
	VersionName        string          `json:"versionName"`
	Categories         []string        `json:"categories"`
	Category           string          `json:"category"`
	Rating             float64         `json:"rating"`
	Namespace          string          `json:"namespace"`
	OnlyAdmin          bool            `json:"onlyAdmin"`
	RequiredMemory     string          `json:"requiredMemory"`
	RequiredDisk       string          `json:"requiredDisk"`
	RequiredGPU        string          `json:"requiredGpu"`
	RequiredCPU        string          `json:"requiredCpu"`
	LimitedMemory      string          `json:"limitedMemory"`
	LimitedCPU         string          `json:"limitedCpu"`
	SupportArch        []string        `json:"supportArch"`
	Status             string          `json:"status"`
	CfgType            string          `json:"cfgType"`
	Locale             []string        `json:"locale"`
	Doc                string          `json:"doc"`
	Website            string          `json:"website"`
	SourceCode         string          `json:"sourceCode"`
	License            []License       `json:"license"`
	InstallCount       int64           `json:"installCount"`
	LastUpdated        string          `json:"lastUpdated"`
	MobileSupported    bool            `json:"mobileSupported"`
	Entrances          []Entrance      `json:"entrances"`
	Permission         *AppPermission  `json:"permission"`
	Dependencies       []Dependency    `json:"dependencies"`
}

// Topic-related types for parsing data.topics
type olaresTopicEntry struct {
	Name string `json:"name"`
	Data struct {
		EnUS olaresTopicData `json:"en-US"`
	} `json:"data"`
}

type olaresTopicData struct {
	Title     string `json:"title"`
	Des       string `json:"des"`
	IconImg   string `json:"iconimg"`
	DetailImg string `json:"detailimg"`
	Apps      string `json:"apps"` // comma-separated chart names (e.g. "vllmqwen330ba3binstruct4bitv2")
	RichText  string `json:"richtext"`
}

// Recommend-related types for parsing data.recommends
type olaresRecommendEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"` // comma-separated app names
	Data        struct {
		Title struct {
			EnUS string `json:"en-US"`
			ZhCN string `json:"zh-CN"`
		} `json:"title"`
	} `json:"data"`
}

// Tag type for parsing data.tags (the real sidebar categories)
type olaresTagEntry struct {
	Name  string `json:"name"`
	Title struct {
		EnUS string `json:"en-US"`
		ZhCN string `json:"zh-CN"`
	} `json:"title"`
	Icon string `json:"icon"`
	Sort int    `json:"sort"`
}

// TopicList type for parsing data.topic_lists
type olaresTopicListEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Content     string `json:"content"` // comma-separated topic IDs
	Title       struct {
		EnUS string `json:"en-US"`
		ZhCN string `json:"zh-CN"`
	} `json:"title"`
}

// Page type for parsing data.pages
type olaresPageEntry struct {
	Category string          `json:"category"`
	Content  json.RawMessage `json:"content"` // JSON array of {type, id}
}

func convertOlaresSourceApp(sa olaresSourceApp) MarketApp {
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
	if app.Description == "" && app.FullDescription != "" {
		desc := app.FullDescription
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		app.Description = desc
	}
	if app.Description == "" {
		app.Description = app.Title
	}
	if len(app.Categories) == 0 && sa.Category != "" {
		app.Categories = []string{sa.Category}
	}
	// Strip version suffixes from categories
	app.Categories = cleanCategories(app.Categories)

	if app.CfgType == "" {
		app.CfgType = "app"
	}
	app.Type = app.CfgType

	return app
}
