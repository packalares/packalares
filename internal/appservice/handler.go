package appservice

import (
	"encoding/json"
	"net/http"
	"strings"

	"k8s.io/klog/v2"
)

// Handler implements the HTTP API for the app-service.
type Handler struct {
	svc *Service
}

// NewHandler creates an HTTP handler backed by the given service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes adds all app-service routes to the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/app-service/v1/install", h.handleInstall)
	mux.HandleFunc("/app-service/v1/uninstall", h.handleUninstall)
	mux.HandleFunc("/app-service/v1/apps", h.handleListApps)
	mux.HandleFunc("/app-service/v1/app/", h.handleGetApp)
	mux.HandleFunc("/app-service/v1/suspend", h.handleSuspend)
	mux.HandleFunc("/app-service/v1/resume", h.handleResume)

	// Health check
	mux.HandleFunc("/app-service/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Compatibility with Olares: /applications endpoint
	mux.HandleFunc("/app-service/v1/applications", h.handleListApps)
}

// handleInstall handles POST /app-service/v1/install
func (h *Handler) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	resp, err := h.svc.Install(r.Context(), &req)
	if err != nil {
		klog.Errorf("install %s: %v", req.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleUninstall handles POST /app-service/v1/uninstall
func (h *Handler) handleUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req UninstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	resp, err := h.svc.Uninstall(r.Context(), &req)
	if err != nil {
		klog.Errorf("uninstall %s: %v", req.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleListApps handles GET /app-service/v1/apps
func (h *Handler) handleListApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	apps := h.svc.ListApps(r.Context())
	writeJSON(w, http.StatusOK, AppListResponse{
		Response: Response{Code: 200},
		Data:     apps,
	})
}

// handleGetApp handles GET /app-service/v1/app/{name}
func (h *Handler) handleGetApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract app name from URL path
	name := strings.TrimPrefix(r.URL.Path, "/app-service/v1/app/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "app name is required")
		return
	}

	app, err := h.svc.GetApp(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, AppDetailResponse{
		Response: Response{Code: 200},
		Data:     app,
	})
}

// handleSuspend handles POST /app-service/v1/suspend
func (h *Handler) handleSuspend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req SuspendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	resp, err := h.svc.Suspend(r.Context(), &req)
	if err != nil {
		klog.Errorf("suspend %s: %v", req.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleResume handles POST /app-service/v1/resume
func (h *Handler) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req ResumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	resp, err := h.svc.Resume(r.Context(), &req)
	if err != nil {
		klog.Errorf("resume %s: %v", req.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// writeJSON serializes data as JSON to the response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		klog.Errorf("write json: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"code":    status,
		"message": message,
	}
	_ = json.NewEncoder(w).Encode(resp)
}
