package market

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"
)

// Handler implements the HTTP API for the market backend.
type Handler struct {
	catalog  *Catalog
	syncMgr  *ChartSyncManager
}

// NewHandler creates a market HTTP handler.
func NewHandler(catalog *Catalog) *Handler {
	return &Handler{catalog: catalog}
}

// SetSyncManager attaches the chart sync manager for sync endpoints.
func (h *Handler) SetSyncManager(mgr *ChartSyncManager) {
	h.syncMgr = mgr
}

// RegisterRoutes adds all market routes to the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/market/v1/apps", h.handleListApps)
	mux.HandleFunc("/market/v1/apps/", h.handleGetApp)
	mux.HandleFunc("/market/v1/app/", h.handleGetAppDetail)
	mux.HandleFunc("/market/v1/categories", h.handleCategories)
	mux.HandleFunc("/market/v1/search", h.handleSearch)
	mux.HandleFunc("/market/v1/recommendations", h.handleRecommendations)

	// Chart sync endpoints
	mux.HandleFunc("/market/v1/sync", h.handleSync)
	mux.HandleFunc("/market/v1/sync/status", h.handleSyncStatus)

	// Chart and icon file serving
	mux.HandleFunc("/charts/", h.handleServeChart)
	mux.HandleFunc("/icons/", h.handleServeIcon)

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
// Returns the app enriched with description/screenshots from GitHub if needed.
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

// syncRequest is the POST body for /market/v1/sync.
// Supports both {"source": "olares"} (single) and {"sources": ["olares"]} (multiple).
type syncRequest struct {
	Source  string   `json:"source"`
	Sources []string `json:"sources"`
}

// handleSync handles POST /market/v1/sync
// Starts a background chart sync from the specified sources.
// Body: {"source": "olares"} or {"source": "all"} or {"sources": ["olares"]}
// Returns immediately with 200, sync runs in background.
func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.syncMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "sync manager not configured")
		return
	}

	if h.syncMgr.IsRunning() {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"code":    409,
			"message": "sync already running",
			"status":  h.syncMgr.Status(),
		})
		return
	}

	var req syncRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	// Build the source list from the request
	var sourceNames []string
	if req.Source == "all" || req.Source == "" {
		// Sync all sources
		sourceNames = nil
	} else {
		sourceNames = []string{req.Source}
	}
	// Merge in any sources from the array field
	if len(req.Sources) > 0 {
		sourceNames = req.Sources
	}

	// Start sync in background with detached context (survives HTTP response)
	go h.syncMgr.SyncAll(context.Background(), sourceNames)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":    200,
		"message": "sync started",
	})
}

// handleSyncStatus handles GET /market/v1/sync/status
func (h *Handler) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.syncMgr == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 200,
			"data": SyncStatus{State: "idle", LastSync: map[string]string{}},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 200,
		"data": h.syncMgr.Status(),
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

	dataDir := defaultDataDir
	if h.syncMgr != nil {
		dataDir = h.syncMgr.DataDir()
	}

	filePath := filepath.Join(dataDir, chartsSubdir, filename)
	http.ServeFile(w, r, filePath)
}

// handleServeIcon serves cached icon files from the icons directory.
func (h *Handler) handleServeIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/icons/")
	filename = filepath.Base(filename) // prevent directory traversal

	if filename == "" || filename == "." {
		writeError(w, http.StatusBadRequest, "filename required")
		return
	}

	dataDir := defaultDataDir
	if h.syncMgr != nil {
		dataDir = h.syncMgr.DataDir()
	}

	filePath := filepath.Join(dataDir, iconsSubdir, filename)
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
