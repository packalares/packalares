package appservice

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/packalares/packalares/pkg/config"
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
	mux.HandleFunc("/app-service/v1/stop", h.handleStop)
	mux.HandleFunc("/app-service/v1/start", h.handleStart)
	mux.HandleFunc("/app-service/v1/app-credentials/", h.handleAppCredentials)
	mux.HandleFunc("/app-service/v1/app-services/", h.handleAppServices)
	mux.HandleFunc("/app-service/v1/internet", h.handleInternet)

	// Model endpoints
	mux.HandleFunc("/app-service/v1/models/status", h.handleModelStatus)
	mux.HandleFunc("/app-service/v1/models/install", h.handleModelInstall)
	mux.HandleFunc("/app-service/v1/models/uninstall", h.handleModelUninstall)

	// WebSocket endpoint for desktop real-time notifications.
	// Auth is handled in-process (not via nginx auth_request) to avoid
	// issues with WebSocket upgrade requests.
	mux.Handle("/ws", AuthWebSocketHandler())

	// Health check
	mux.HandleFunc("/app-service/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Compatibility with Olares: /applications endpoint
	mux.HandleFunc("/app-service/v1/applications", h.handleListApps)

	// Desktop integration: /server/* endpoints for the Olares Vue.js desktop
	mux.HandleFunc("/server/init", h.handleServerInit)
	mux.HandleFunc("/server/myApps", h.handleServerMyApps)
	mux.HandleFunc("/server/updateConfig", h.handleServerUpdateConfig)
	mux.HandleFunc("/server/uninstall/", h.handleUninstall)
	mux.HandleFunc("/server/upgrade/state", h.handleUpgradeState)
	mux.HandleFunc("/server/query", h.handleServerQuery)
	mux.HandleFunc("/server/search", h.handleServerQuery)
	mux.HandleFunc("/api/device", h.handleDevice)
	mux.HandleFunc("/api/monitor/cluster", h.handleMonitorCluster)
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

// handleStop handles POST /app-service/v1/stop — alias for suspend.
func (h *Handler) handleStop(w http.ResponseWriter, r *http.Request) {
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
		klog.Errorf("stop %s: %v", req.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleStart handles POST /app-service/v1/start — alias for resume.
func (h *Handler) handleStart(w http.ResponseWriter, r *http.Request) {
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
		klog.Errorf("start %s: %v", req.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleModelStatus returns all installed models across all backends.
func (h *Handler) handleModelStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	models, err := h.svc.ListInstalledModels(r.Context())
	if err != nil {
		klog.Errorf("list installed models: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models)
}

// handleModelInstall starts installing a model on the specified backend.
func (h *Handler) handleModelInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var spec ModelSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if spec.Name == "" {
		writeError(w, http.StatusBadRequest, "model name is required")
		return
	}
	if spec.Backend == "" {
		spec.Backend = "ollama" // default backend
	}

	if err := h.svc.InstallModel(r.Context(), spec); err != nil {
		klog.Errorf("install model %s: %v", spec.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "installing",
		"name":   spec.Name,
	})
}

// handleModelUninstall removes a model from the specified backend.
func (h *Handler) handleModelUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var spec ModelSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if spec.Name == "" {
		writeError(w, http.StatusBadRequest, "model name is required")
		return
	}
	if spec.Backend == "" {
		spec.Backend = "ollama"
	}

	if err := h.svc.UninstallModel(r.Context(), spec); err != nil {
		klog.Errorf("uninstall model %s: %v", spec.Name, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "uninstalling",
		"name":   spec.Name,
	})
}

// handleServerInit returns desktop initialization data.
// Reads from User CRD and environment for real values.
func (h *Handler) handleServerInit(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("Remote-User")
	if username == "" {
		username = config.Username()
	}
	zone := os.Getenv("USER_ZONE")
	if zone == "" {
		zone = config.UserZone()
	}

	terminusName := username + "@" + strings.TrimPrefix(zone, username+".")
	avatar := ""
	wizardStatus := "completed"

	// Try reading user info from BFL service
	bflURL := os.Getenv("BFL_URL")
	if bflURL == "" {
		bflURL = "http://" + config.BFLDNS() + ":80"
	}
	resp2, err := http.Get(bflURL + "/bfl/backend/v1/user-info")
	if err == nil {
		defer resp2.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp2.Body, 65536))
		var bflResp struct {
			Data struct {
				Name         string `json:"name"`
				TerminusName string `json:"terminusName"`
				Zone         string `json:"zone"`
				WizardComplete bool `json:"wizard_complete"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &bflResp) == nil && bflResp.Data.Name != "" {
			terminusName = bflResp.Data.TerminusName
			if bflResp.Data.WizardComplete {
				wizardStatus = "completed"
			}
		}
	}

	// Include installed apps
	apps := h.svc.ListApps(r.Context())
	var desktopApps []map[string]interface{}
	for _, app := range apps {
		url := ""
		if len(app.Entrances) > 0 {
			url = app.Entrances[0].URL
		}
		desktopApps = append(desktopApps, map[string]interface{}{
			"name":   app.Name,
			"title":  app.Title,
			"icon":   app.Icon,
			"status": string(app.State),
			"url":    url,
		})
	}

	resp := map[string]interface{}{
		"terminus": map[string]interface{}{
			"terminusName":    terminusName,
			"wizardStatus":    wizardStatus,
			"selfhosted":      true,
			"osVersion":       "1.0.0",
			"loginBackground": "",
			"avatar":          avatar,
			"did":             "",
		},
		"config": map[string]interface{}{
			"apps":    []interface{}{},
			"dock":    []interface{}{},
			"bgIndex": 0,
		},
		"myApps": map[string]interface{}{
			"code": 200,
			"data": desktopApps,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleUpgradeState returns upgrade status (always up-to-date for self-hosted).
func (h *Handler) handleUpgradeState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"state":          "idle",
		"currentVersion": "1.0.0",
		"latestVersion":  "1.0.0",
		"upgradeAvail":   false,
	})
}

// handleDevice accepts device registration (stub).
func (h *Handler) handleDevice(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

// handleMonitorCluster returns cluster monitoring data from Prometheus.
func (h *Handler) handleMonitorCluster(w http.ResponseWriter, r *http.Request) {
	promURL := config.PrometheusURL()

	query := func(q string) float64 {
		resp, err := http.Get(promURL + "/api/v1/query?query=" + url.QueryEscape(q))
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
		var result struct {
			Data struct {
				Result []struct {
					Value []interface{} `json:"value"`
				} `json:"result"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &result) != nil || len(result.Data.Result) == 0 {
			return 0
		}
		if len(result.Data.Result[0].Value) < 2 {
			return 0
		}
		val, _ := strconv.ParseFloat(fmt.Sprintf("%v", result.Data.Result[0].Value[1]), 64)
		return val
	}

	cpuRatio := query(`1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m]))`)
	cpuCores := query(`count(node_cpu_seconds_total{mode="idle"})`)
	memTotal := query(`node_memory_MemTotal_bytes`)
	memAvail := query(`node_memory_MemAvailable_bytes`)
	diskTotal := query(`node_filesystem_size_bytes{mountpoint="/",fstype!="rootfs"}`)
	diskAvail := query(`node_filesystem_avail_bytes{mountpoint="/",fstype!="rootfs"}`)

	memUsed := memTotal - memAvail
	diskUsed := diskTotal - diskAvail
	memRatio := 0.0
	diskRatio := 0.0
	if memTotal > 0 {
		memRatio = memUsed / memTotal
	}
	if diskTotal > 0 {
		diskRatio = diskUsed / diskTotal
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cpu":    map[string]interface{}{"ratio": cpuRatio, "total": cpuCores, "usage": cpuRatio * cpuCores},
		"memory": map[string]interface{}{"ratio": memRatio, "total": memTotal, "usage": memUsed},
		"disk":   map[string]interface{}{"ratio": diskRatio, "total": diskTotal, "usage": diskUsed},
		"gpu":    map[string]interface{}{"ratio": 0, "total": 0, "usage": 0},
		"net":    map[string]interface{}{"received": 0, "transmitted": 0},
	})
}

// handleServerQuery handles search queries (stub).
func (h *Handler) handleServerQuery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 200,
		"data": []interface{}{},
	})
}

// handleServerMyApps returns installed apps for the desktop launcher.
func (h *Handler) handleServerMyApps(w http.ResponseWriter, r *http.Request) {
	apps := h.svc.ListApps(r.Context())

	// Convert to the format the desktop expects
	var desktopApps []map[string]interface{}
	for _, app := range apps {
		url := ""
		if len(app.Entrances) > 0 {
			url = app.Entrances[0].URL
		}
		desktopApps = append(desktopApps, map[string]interface{}{
			"name":   app.Name,
			"title":  app.Title,
			"icon":   app.Icon,
			"status": string(app.State),
			"url":    url,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 200,
		"data": desktopApps,
	})
}

// handleServerUpdateConfig saves desktop layout config (dock, bg, etc).
func (h *Handler) handleServerUpdateConfig(w http.ResponseWriter, r *http.Request) {
	// Accept and acknowledge but don't persist yet
	var config map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&config)
	writeJSON(w, http.StatusOK, config)
}

// writeJSON serializes data as JSON to the response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		klog.Errorf("write json: %v", err)
	}
}

// handleAppCredentials returns admin credentials for an installed app.
// GET /app-service/v1/app-credentials/<appName>
func (h *Handler) handleAppCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	appName := strings.TrimPrefix(r.URL.Path, "/app-service/v1/app-credentials/")
	appName = strings.TrimSuffix(appName, "/")
	if appName == "" {
		writeError(w, http.StatusBadRequest, "app name required")
		return
	}

	creds, err := h.svc.GetAppCredentials(r.Context(), appName)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(creds)
}

// handleAppServices returns live Kubernetes services for an installed app.
// GET /app-service/v1/app-services/<appName>
func (h *Handler) handleAppServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	appName := strings.TrimPrefix(r.URL.Path, "/app-service/v1/app-services/")
	appName = strings.TrimSuffix(appName, "/")
	if appName == "" {
		writeError(w, http.StatusBadRequest, "app name required")
		return
	}

	services := h.svc.k8s.GetServicesForApp(r.Context(), appName, h.svc.namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// handleInternet toggles internet access for an app.
func (h *Handler) handleInternet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Name    string `json:"name"`
		Blocked bool   `json:"blocked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	rec, exists := h.svc.store.Get(r.Context(), req.Name)
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", req.Name))
		return
	}

	rec.InternetBlocked = req.Blocked
	_ = h.svc.store.Put(r.Context(), rec)

	if req.Blocked {
		_ = h.svc.k8s.BlockAppInternet(r.Context(), rec.Namespace, rec.ReleaseName)
	} else {
		_ = h.svc.k8s.UnblockAppInternet(r.Context(), rec.Namespace, rec.ReleaseName)
	}

	GetWSHub().BroadcastAppState(req.Name, rec.State)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":    200,
		"message": fmt.Sprintf("internet %s for %s", map[bool]string{true: "blocked", false: "allowed"}[req.Blocked], req.Name),
	})
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
