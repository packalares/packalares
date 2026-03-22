package market

import (
	"encoding/json"
	"net/http"
	"strings"

	"k8s.io/klog/v2"
)

// Handler implements the HTTP API for the market backend.
type Handler struct {
	catalog *Catalog
}

// NewHandler creates a market HTTP handler.
func NewHandler(catalog *Catalog) *Handler {
	return &Handler{catalog: catalog}
}

// RegisterRoutes adds all market routes to the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/market/v1/apps", h.handleListApps)
	mux.HandleFunc("/market/v1/apps/", h.handleGetApp)
	mux.HandleFunc("/market/v1/categories", h.handleCategories)
	mux.HandleFunc("/market/v1/search", h.handleSearch)
	mux.HandleFunc("/market/v1/recommendations", h.handleRecommendations)

	// Health check
	mux.HandleFunc("/market/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Compatibility: Olares market frontend expects these paths
	mux.HandleFunc("/api/v1/apps", h.handleListApps)
	mux.HandleFunc("/api/v1/apps/", h.handleGetApp)
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
