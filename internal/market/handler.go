package market

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	// defaultDataDir is the base directory for market data (charts, icons, screenshots, catalog).
	defaultDataDir    = "/data/market"
	chartsSubdir      = "charts"
	iconsSubdir       = "icons"
	screenshotsSubdir = "screenshots"
)

// Handler implements the HTTP API for the market backend.
type Handler struct {
	catalog       *Catalog
	dataDir       string
	appServiceURL string
	httpClient    *http.Client
}

// NewHandler creates a market HTTP handler.
func NewHandler(catalog *Catalog, dataDir string) *Handler {
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	return &Handler{
		catalog:       catalog,
		dataDir:       dataDir,
		appServiceURL: "http://app-service:6755",
		httpClient:    &http.Client{Timeout: 3 * time.Second},
	}
}

// RegisterRoutes adds all market routes to the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/market/v1/apps", h.handleListApps)
	mux.HandleFunc("/market/v1/apps/", h.handleGetApp)
	mux.HandleFunc("/market/v1/app/", h.handleGetAppDetail)
	mux.HandleFunc("/market/v1/categories", h.handleCategories)
	mux.HandleFunc("/market/v1/search", h.handleSearch)
	mux.HandleFunc("/market/v1/recommendations", h.handleRecommendations)

	// Chart, icon, and screenshot file serving
	mux.HandleFunc("/charts/", h.handleServeChart)
	mux.HandleFunc("/icons/", h.handleServeIcon)
	mux.HandleFunc("/market/v1/icons/", h.handleServeIcon)
	mux.HandleFunc("/market/v1/screenshots/", h.handleServeScreenshot)

	// Health check
	mux.HandleFunc("/market/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Compatibility: Olares market frontend expects these paths
	mux.HandleFunc("/api/v1/apps", h.handleListApps)
	mux.HandleFunc("/api/v1/apps/", h.handleGetApp)
	mux.HandleFunc("/api/v1/app/", h.handleGetAppDetail)
	mux.HandleFunc("/api/v1/categories", h.handleCategories)
	mux.HandleFunc("/api/v1/search", h.handleSearch)
	mux.HandleFunc("/api/v1/recommendations", h.handleRecommendations)

	// Screenshot serving under /api/ prefix too
	mux.HandleFunc("/api/market/screenshots/", h.handleServeScreenshot)
	mux.HandleFunc("/api/market/icons/", h.handleServeIcon)
}

// handleListApps handles GET /market/v1/apps
func (h *Handler) handleListApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Support optional category filter
	category := r.URL.Query().Get("category")
	var apps []MarketApp

	if category != "" {
		all := h.catalog.ListApps()
		for _, app := range all {
			for _, cat := range app.Categories {
				if strings.EqualFold(cat, category) {
					apps = append(apps, app)
					break
				}
			}
		}
	} else {
		apps = h.catalog.ListApps()
	}

	// All apps have charts since they are shipped in the image
	for i := range apps {
		apps[i].HasChart = true
	}

	writeJSON(w, http.StatusOK, CatalogResponse{
		Response: Response{Code: 200},
		Data:     apps,
	})
}

// handleGetApp handles GET /market/v1/apps/{name}
func (h *Handler) handleGetApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract name from path — handles both /market/v1/apps/{name} and /api/v1/apps/{name}
	name := r.URL.Path
	for _, prefix := range []string{"/market/v1/apps/", "/api/v1/apps/"} {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}
	name = strings.TrimSuffix(name, "/")

	if name == "" {
		// If no name, return the full list
		h.handleListApps(w, r)
		return
	}

	app, ok := h.catalog.GetApp(name)
	if !ok {
		writeError(w, http.StatusNotFound, "app not found: "+name)
		return
	}

	writeJSON(w, http.StatusOK, AppDetailResponse{
		Response: Response{Code: 200},
		Data:     app,
	})
}

// handleGetAppDetail handles GET /market/v1/app/{name}
// Returns enriched data including volume mounts from the chart and credentials from app-service.
func (h *Handler) handleGetAppDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	name := r.URL.Path
	for _, prefix := range []string{"/market/v1/app/", "/api/v1/app/"} {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}
	name = strings.TrimSuffix(name, "/")

	if name == "" {
		writeError(w, http.StatusBadRequest, "app name required")
		return
	}

	app, ok := h.catalog.GetAppDetail(name)
	if !ok {
		writeError(w, http.StatusNotFound, "app not found: "+name)
		return
	}

	enriched := &AppDetailEnriched{MarketApp: *app}

	// Parse volume mounts from chart templates
	chartName := app.ChartName
	if chartName == "" {
		chartName = app.Name
	}
	enriched.VolumeMounts = h.parseChartVolumeMounts(chartName)
	enriched.Resources = h.parseChartResources(chartName)

	// Fetch credentials from app-service (best-effort, ignore errors)
	enriched.Credentials = h.fetchAppCredentials(name)

	writeJSON(w, http.StatusOK, AppDetailEnrichedResponse{
		Response: Response{Code: 200},
		Data:     enriched,
	})
}

// fetchAppCredentials calls app-service for credentials. Returns nil on any error.
func (h *Handler) fetchAppCredentials(appName string) *AppCredentials {
	resp, err := h.httpClient.Get(h.appServiceURL + "/app-service/v1/app-credentials/" + appName)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	var creds AppCredentials
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return nil
	}
	if creds.Username == "" && creds.Password == "" {
		return nil
	}
	return &creds
}

// parseChartVolumeMounts reads deployment templates from a chart directory
// and extracts volumeMounts + matching hostPath volumes.
func (h *Handler) parseChartVolumeMounts(chartName string) []VolumeMount {
	files := h.getChartTemplateFiles(chartName)
	var mounts []VolumeMount
	for _, filePath := range files {
		m := parseVolumeMountsFromYAML(filePath)
		mounts = append(mounts, m...)
	}
	return mounts
}

// parseVolumeMountsFromYAML does a simple line-based parse of a Kubernetes YAML
// to extract volumeMounts (mountPath + name) and match them to hostPath volumes.
func parseVolumeMountsFromYAML(path string) []VolumeMount {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	type rawMount struct {
		mountPath string
		name      string
	}

	var rawMounts []rawMount
	hostPaths := map[string]string{} // volume name -> hostPath

	scanner := bufio.NewScanner(f)
	var inVolumeMounts, inVolumes bool
	var currentMount rawMount
	var currentVolName, currentHostPath string
	var indent int

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track sections
		if strings.Contains(trimmed, "volumeMounts:") {
			inVolumeMounts = true
			inVolumes = false
			indent = len(line) - len(strings.TrimLeft(line, " "))
			continue
		}
		if strings.Contains(trimmed, "volumes:") && !strings.Contains(trimmed, "volumeMounts") {
			inVolumes = true
			inVolumeMounts = false
			indent = len(line) - len(strings.TrimLeft(line, " "))
			continue
		}

		lineIndent := len(line) - len(strings.TrimLeft(line, " "))

		if inVolumeMounts {
			if trimmed == "" || (lineIndent <= indent && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "mountPath") && !strings.HasPrefix(trimmed, "name")) {
				if currentMount.mountPath != "" {
					rawMounts = append(rawMounts, currentMount)
					currentMount = rawMount{}
				}
				inVolumeMounts = false
				continue
			}
			if strings.HasPrefix(trimmed, "- mountPath:") || strings.HasPrefix(trimmed, "- name:") {
				if currentMount.mountPath != "" {
					rawMounts = append(rawMounts, currentMount)
					currentMount = rawMount{}
				}
			}
			if strings.Contains(trimmed, "mountPath:") {
				currentMount.mountPath = extractYAMLValue(trimmed, "mountPath")
			}
			if strings.Contains(trimmed, "name:") && !strings.Contains(trimmed, "mountPath") {
				currentMount.name = extractYAMLValue(trimmed, "name")
			}
		}

		if inVolumes {
			if trimmed == "" || (lineIndent <= indent && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "name") && !strings.HasPrefix(trimmed, "hostPath") && !strings.HasPrefix(trimmed, "path") && !strings.HasPrefix(trimmed, "type")) {
				if currentVolName != "" && currentHostPath != "" {
					hostPaths[currentVolName] = currentHostPath
				}
				if trimmed != "" && lineIndent <= indent {
					inVolumes = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "- name:") {
				if currentVolName != "" && currentHostPath != "" {
					hostPaths[currentVolName] = currentHostPath
				}
				currentVolName = extractYAMLValue(trimmed, "name")
				currentHostPath = ""
			}
			if strings.Contains(trimmed, "path:") && !strings.HasPrefix(trimmed, "hostPath") {
				currentHostPath = extractYAMLValue(trimmed, "path")
			}
		}
	}

	// Flush last items
	if currentMount.mountPath != "" {
		rawMounts = append(rawMounts, currentMount)
	}
	if currentVolName != "" && currentHostPath != "" {
		hostPaths[currentVolName] = currentHostPath
	}

	// Match mounts to host paths
	var result []VolumeMount
	for _, rm := range rawMounts {
		vm := VolumeMount{
			MountPath: rm.mountPath,
			Name:      rm.name,
		}
		if hp, ok := hostPaths[rm.name]; ok {
			vm.HostPath = hp
		}
		result = append(result, vm)
	}
	return result
}

// parseChartResources reads deployment templates from a chart and extracts resource requests/limits.
func (h *Handler) parseChartResources(chartName string) []ContainerResources {
	files := h.getChartTemplateFiles(chartName)
	var all []ContainerResources
	for _, filePath := range files {
		res := parseResourcesFromYAML(filePath)
		all = append(all, res...)
	}
	return all
}

// getChartTemplateFiles returns paths to template YAML files for a chart.
// Tries raw directory first, then unpacks from .tgz.
func (h *Handler) getChartTemplateFiles(chartName string) []string {
	// Try raw directory
	templatesDir := filepath.Join(h.dataDir, chartsSubdir, chartName, "templates")
	if entries, err := os.ReadDir(templatesDir); err == nil {
		var files []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				files = append(files, filepath.Join(templatesDir, e.Name()))
			}
		}
		if len(files) > 0 {
			return files
		}
	}

	// Try .tgz
	chartsDir := filepath.Join(h.dataDir, chartsSubdir)
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		return nil
	}
	var tgzPath string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), chartName+"-") && strings.HasSuffix(e.Name(), ".tgz") {
			tgzPath = filepath.Join(chartsDir, e.Name())
			break
		}
	}
	if tgzPath == "" {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "chart-parse-")
	if err != nil {
		return nil
	}
	// Note: caller should ideally clean up, but these are small and short-lived
	if err := unpackTGZToDir(tgzPath, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return nil
	}

	// Find templates dir inside unpacked chart
	dirEntries, _ := os.ReadDir(tmpDir)
	for _, d := range dirEntries {
		if d.IsDir() {
			tDir := filepath.Join(tmpDir, d.Name(), "templates")
			if files, err := os.ReadDir(tDir); err == nil {
				var result []string
				for _, f := range files {
					if !f.IsDir() && strings.HasSuffix(f.Name(), ".yaml") {
						result = append(result, filepath.Join(tDir, f.Name()))
					}
				}
				return result
			}
		}
	}
	return nil
}

func unpackTGZToDir(tgzPath, destDir string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			out, err := os.Create(target)
			if err != nil {
				continue
			}
			io.Copy(out, tr)
			out.Close()
		}
	}
	return nil
}

// parseResourcesFromYAML extracts container names and their resource requests/limits.
func parseResourcesFromYAML(path string) []ContainerResources {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var results []ContainerResources
	var currentContainer string
	var inResources, inRequests, inLimits bool

	currentRes := ContainerResources{}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Detect container name
		if strings.HasPrefix(trimmed, "- name:") && !strings.Contains(trimmed, "mountPath") {
			lineIndent := len(line) - len(strings.TrimLeft(line, " "))
			// Container names are typically at indent 10-12 (under containers:)
			if lineIndent >= 8 && lineIndent <= 16 {
				// Save previous container if it had resources
				if currentContainer != "" && (currentRes.Requests != (ResMap{}) || currentRes.Limits != (ResMap{})) {
					currentRes.Container = currentContainer
					results = append(results, currentRes)
					currentRes = ContainerResources{}
				}
				currentContainer = extractYAMLValue(trimmed, "name")
			}
		}

		if strings.Contains(trimmed, "resources:") && !strings.HasPrefix(trimmed, "#") {
			inResources = true
			inRequests = false
			inLimits = false
			continue
		}

		if inResources {
			if trimmed == "requests:" {
				inRequests = true
				inLimits = false
				continue
			}
			if trimmed == "limits:" {
				inLimits = true
				inRequests = false
				continue
			}

			// Exit resources block
			lineIndent := len(line) - len(strings.TrimLeft(line, " "))
			if trimmed != "" && !strings.HasPrefix(trimmed, "cpu") && !strings.HasPrefix(trimmed, "memory") &&
				!strings.HasPrefix(trimmed, "nvidia") && !strings.HasPrefix(trimmed, "requests") &&
				!strings.HasPrefix(trimmed, "limits") && lineIndent <= 10 {
				inResources = false
				inRequests = false
				inLimits = false
				continue
			}

			if inRequests {
				if strings.HasPrefix(trimmed, "cpu:") {
					currentRes.Requests.CPU = extractYAMLValue(trimmed, "cpu")
				}
				if strings.HasPrefix(trimmed, "memory:") {
					currentRes.Requests.Memory = extractYAMLValue(trimmed, "memory")
				}
				if strings.Contains(trimmed, "nvidia.com/gpu") {
					currentRes.Requests.GPU = extractYAMLValue(trimmed, "nvidia.com/gpu")
				}
			}
			if inLimits {
				if strings.HasPrefix(trimmed, "cpu:") {
					currentRes.Limits.CPU = extractYAMLValue(trimmed, "cpu")
				}
				if strings.HasPrefix(trimmed, "memory:") {
					currentRes.Limits.Memory = extractYAMLValue(trimmed, "memory")
				}
				if strings.Contains(trimmed, "nvidia.com/gpu") {
					currentRes.Limits.GPU = extractYAMLValue(trimmed, "nvidia.com/gpu")
				}
			}
		}
	}

	// Flush last container
	if currentContainer != "" && (currentRes.Requests != (ResMap{}) || currentRes.Limits != (ResMap{})) {
		currentRes.Container = currentContainer
		results = append(results, currentRes)
	}

	return results
}

// extractYAMLValue extracts the value after "key:" from a YAML line, stripping quotes and template markers.
func extractYAMLValue(line, key string) string {
	idx := strings.Index(line, key+":")
	if idx < 0 {
		return ""
	}
	val := strings.TrimSpace(line[idx+len(key)+1:])
	// Strip leading "- " if present
	val = strings.TrimPrefix(val, "- ")
	val = strings.Trim(val, `"'`)
	return val
}

// handleCategories handles GET /market/v1/categories
func (h *Handler) handleCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cats := h.catalog.ListCategories()
	writeJSON(w, http.StatusOK, CategoriesResponse{
		Response: Response{Code: 200},
		Data:     cats,
	})
}

// handleSearch handles GET /market/v1/search?q=
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query().Get("q")
	results := h.catalog.Search(query)

	writeJSON(w, http.StatusOK, SearchResponse{
		Response: Response{Code: 200},
		Data:     results,
	})
}

// handleRecommendations handles GET /market/v1/recommendations
func (h *Handler) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	recs := h.catalog.ListRecommendations()
	writeJSON(w, http.StatusOK, RecommendationsResponse{
		Response: Response{Code: 200},
		Data:     recs,
	})
}

// handleServeChart serves chart .tgz files from the charts directory.
func (h *Handler) handleServeChart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/charts/")
	filename = filepath.Base(filename) // prevent directory traversal

	if filename == "" || filename == "." {
		writeError(w, http.StatusBadRequest, "filename required")
		return
	}

	// If the request is for index.yaml, generate it dynamically
	if filename == "index.yaml" {
		h.serveChartIndex(w, r)
		return
	}

	filePath := filepath.Join(h.dataDir, chartsSubdir, filename)

	// If exact file not found, try matching by app name (e.g. "ollama" matches "ollama-1.0.5.tgz")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		chartsDir := filepath.Join(h.dataDir, chartsSubdir)
		entries, _ := os.ReadDir(chartsDir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), filename+"-") && strings.HasSuffix(e.Name(), ".tgz") {
				filePath = filepath.Join(chartsDir, e.Name())
				break
			}
		}
	}

	http.ServeFile(w, r, filePath)
}

// handleServeIcon serves cached icon files from the icons directory.
func (h *Handler) handleServeIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Handle multiple path prefixes
	filename := r.URL.Path
	for _, prefix := range []string{"/api/market/icons/", "/market/v1/icons/", "/icons/"} {
		if strings.HasPrefix(filename, prefix) {
			filename = strings.TrimPrefix(filename, prefix)
			break
		}
	}
	filename = filepath.Base(filename) // prevent directory traversal

	if filename == "" || filename == "." {
		writeError(w, http.StatusBadRequest, "filename required")
		return
	}

	filePath := filepath.Join(h.dataDir, iconsSubdir, filename)
	http.ServeFile(w, r, filePath)
}

// handleServeScreenshot serves cached screenshot files from the screenshots directory.
// Path format: /api/market/screenshots/{appname}/{filename}
// or /market/v1/screenshots/{appname}/{filename}
func (h *Handler) handleServeScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract the path after the prefix
	pathSuffix := r.URL.Path
	for _, prefix := range []string{"/api/market/screenshots/", "/market/v1/screenshots/"} {
		if strings.HasPrefix(pathSuffix, prefix) {
			pathSuffix = strings.TrimPrefix(pathSuffix, prefix)
			break
		}
	}

	// pathSuffix should be "appname/filename"
	parts := strings.SplitN(pathSuffix, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "path format: /screenshots/{appname}/{filename}")
		return
	}

	appName := filepath.Base(parts[0])  // prevent traversal
	filename := filepath.Base(parts[1]) // prevent traversal

	filePath := filepath.Join(h.dataDir, screenshotsSubdir, appName, filename)
	http.ServeFile(w, r, filePath)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		klog.Errorf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"code":    status,
		"message": message,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// serveChartIndex dynamically generates a Helm repo index.yaml from chart files on disk.
func (h *Handler) serveChartIndex(w http.ResponseWriter, r *http.Request) {
	chartsDir := filepath.Join(h.dataDir, chartsSubdir)
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read charts directory")
		return
	}

	re := regexp.MustCompile(`^(.+?)-(\d+\.\d+\.\d+)\.tgz$`)
	type chartVersion struct {
		Version string   `yaml:"version" json:"version"`
		URLs    []string `yaml:"urls" json:"urls"`
	}
	index := make(map[string][]chartVersion)

	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		name, version := m[1], m[2]
		index[name] = append(index[name], chartVersion{
			Version: version,
			URLs:    []string{e.Name()},
		})
	}

	w.Header().Set("Content-Type", "text/yaml")
	fmt.Fprintf(w, "apiVersion: v1\nentries:\n")
	for name, versions := range index {
		fmt.Fprintf(w, "  %s:\n", name)
		for _, v := range versions {
			fmt.Fprintf(w, "  - version: %q\n    urls:\n    - %q\n", v.Version, v.URLs[0])
		}
	}
}
