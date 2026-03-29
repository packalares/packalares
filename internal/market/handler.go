package market

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

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
	catalog *Catalog
	dataDir string
}

// NewHandler creates a market HTTP handler.
func NewHandler(catalog *Catalog, dataDir string) *Handler {
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	return &Handler{catalog: catalog, dataDir: dataDir}
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
func (h *Handler) handleGetAppDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract name from path — handles both /market/v1/app/{name} and /api/v1/app/{name}
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

	writeJSON(w, http.StatusOK, AppDetailResponse{
		Response: Response{Code: 200},
		Data:     app,
	})
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

	filePath := filepath.Join(h.dataDir, chartsSubdir, filename)
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
