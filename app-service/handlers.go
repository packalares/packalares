package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Handlers groups the HTTP handler methods and shared state.
type Handlers struct {
	Config  Config
	Catalog []App
}

// ---------- helpers ----------

// helmEnv returns the environment slice for helm/kubectl subprocesses.
func (h *Handlers) helmEnv() []string {
	env := os.Environ()
	if h.Config.KubeConfig != "" {
		env = append(env, "KUBECONFIG="+h.Config.KubeConfig)
	}
	return env
}

func runCmd(name string, env []string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ---------- GET /api/status ----------

type PodInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Ready  string `json:"ready"`
	Age    string `json:"age"`
}

type StatusResponse struct {
	Activated bool      `json:"activated"`
	Namespace string    `json:"namespace"`
	Pods      []PodInfo `json:"pods"`
	Services  []string  `json:"services"`
	Error     string    `json:"error,omitempty"`
}

func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	resp := StatusResponse{
		Activated: true,
		Namespace: h.Config.Namespace,
	}

	env := h.helmEnv()

	// pods
	out, err := runCmd("kubectl", env, "get", "pods", "-n", h.Config.Namespace,
		"--no-headers", "-o", "custom-columns=NAME:.metadata.name,STATUS:.status.phase,READY:.status.containerStatuses[0].ready,AGE:.metadata.creationTimestamp")
	if err != nil {
		resp.Error = fmt.Sprintf("kubectl pods: %v: %s", err, out)
	} else if out != "" {
		scanner := bufio.NewScanner(strings.NewReader(out))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 2 {
				p := PodInfo{Name: fields[0], Status: fields[1]}
				if len(fields) >= 3 {
					p.Ready = fields[2]
				}
				if len(fields) >= 4 {
					p.Age = fields[3]
				}
				resp.Pods = append(resp.Pods, p)
			}
		}
	}
	if resp.Pods == nil {
		resp.Pods = []PodInfo{}
	}

	// services
	out, err = runCmd("kubectl", env, "get", "svc", "-n", h.Config.Namespace,
		"--no-headers", "-o", "custom-columns=NAME:.metadata.name")
	if err == nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			s := strings.TrimSpace(line)
			if s != "" {
				resp.Services = append(resp.Services, s)
			}
		}
	}
	if resp.Services == nil {
		resp.Services = []string{}
	}

	jsonResponse(w, http.StatusOK, resp)
}

// ---------- GET /api/metrics ----------

type MetricsResponse struct {
	CPU    CPUMetrics  `json:"cpu"`
	Memory MemMetrics  `json:"memory"`
	Disk   DiskMetrics `json:"disk"`
	GPU    *GPUMetrics `json:"gpu,omitempty"`
}

type CPUMetrics struct {
	UsagePercent float64 `json:"usage_percent"`
	Cores        int     `json:"cores"`
}

type MemMetrics struct {
	TotalMB     int64   `json:"total_mb"`
	UsedMB      int64   `json:"used_mb"`
	UsedPercent float64 `json:"used_percent"`
}

type DiskMetrics struct {
	TotalGB     float64 `json:"total_gb"`
	UsedGB      float64 `json:"used_gb"`
	UsedPercent float64 `json:"used_percent"`
}

type GPUMetrics struct {
	Name      string `json:"name"`
	MemoryMB  int    `json:"memory_mb"`
	UsedMB    int    `json:"used_mb"`
	TempC     int    `json:"temp_c"`
	Available bool   `json:"available"`
}

func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	resp := MetricsResponse{
		CPU:    readCPU(),
		Memory: readMemory(),
		Disk:   readDisk(),
		GPU:    readGPU(),
	}
	jsonResponse(w, http.StatusOK, resp)
}

// readCPU reads /proc/stat to compute overall CPU usage.
func readCPU() CPUMetrics {
	m := CPUMetrics{}

	data, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "processor") {
				m.Cores++
			}
		}
	}

	data, err = os.ReadFile("/proc/stat")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					user, _ := strconv.ParseFloat(fields[1], 64)
					nice, _ := strconv.ParseFloat(fields[2], 64)
					system, _ := strconv.ParseFloat(fields[3], 64)
					idle, _ := strconv.ParseFloat(fields[4], 64)
					total := user + nice + system + idle
					if total > 0 {
						m.UsagePercent = ((total - idle) / total) * 100
						m.UsagePercent = float64(int(m.UsagePercent*10)) / 10 // one decimal
					}
				}
				break
			}
		}
	}
	return m
}

// readMemory reads /proc/meminfo.
func readMemory() MemMetrics {
	m := MemMetrics{}

	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return m
	}

	values := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			key := strings.TrimSuffix(parts[0], ":")
			val, _ := strconv.ParseInt(parts[1], 10, 64)
			values[key] = val // in kB
		}
	}

	totalKB := values["MemTotal"]
	availKB := values["MemAvailable"]
	m.TotalMB = totalKB / 1024
	m.UsedMB = (totalKB - availKB) / 1024
	if m.TotalMB > 0 {
		m.UsedPercent = float64(m.UsedMB) / float64(m.TotalMB) * 100
		m.UsedPercent = float64(int(m.UsedPercent*10)) / 10
	}
	return m
}

// readDisk uses df to get root filesystem stats.
func readDisk() DiskMetrics {
	m := DiskMetrics{}
	out, err := runCmd("df", nil, "-BG", "--output=size,used,pcent", "/")
	if err != nil {
		return m
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return m
	}
	fields := strings.Fields(lines[1])
	if len(fields) >= 3 {
		sizeStr := strings.TrimSuffix(fields[0], "G")
		usedStr := strings.TrimSuffix(fields[1], "G")
		pctStr := strings.TrimSuffix(fields[2], "%")
		m.TotalGB, _ = strconv.ParseFloat(sizeStr, 64)
		m.UsedGB, _ = strconv.ParseFloat(usedStr, 64)
		m.UsedPercent, _ = strconv.ParseFloat(pctStr, 64)
	}
	return m
}

// readGPU tries nvidia-smi for GPU metrics.
func readGPU() *GPUMetrics {
	out, err := runCmd("nvidia-smi", nil,
		"--query-gpu=name,memory.total,memory.used,temperature.gpu",
		"--format=csv,noheader,nounits")
	if err != nil {
		return nil
	}
	fields := strings.Split(out, ", ")
	if len(fields) < 4 {
		return nil
	}
	g := &GPUMetrics{
		Name:      strings.TrimSpace(fields[0]),
		Available: true,
	}
	g.MemoryMB, _ = strconv.Atoi(strings.TrimSpace(fields[1]))
	g.UsedMB, _ = strconv.Atoi(strings.TrimSpace(fields[2]))
	g.TempC, _ = strconv.Atoi(strings.TrimSpace(fields[3]))
	return g
}

// ---------- GET /api/apps/available ----------

func (h *Handlers) AvailableApps(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, h.Catalog)
}

// ---------- GET /api/apps/installed ----------

type InstalledApp struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Revision  string `json:"revision"`
	Status    string `json:"status"`
	Chart     string `json:"chart"`
	AppVer    string `json:"app_version"`
}

func (h *Handlers) InstalledApps(w http.ResponseWriter, r *http.Request) {
	env := h.helmEnv()
	out, err := runCmd("helm", env, "list", "-n", h.Config.Namespace, "--no-headers")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("helm list: %v: %s", err, out))
		return
	}

	apps := []InstalledApp{}
	if out != "" {
		scanner := bufio.NewScanner(strings.NewReader(out))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 6 {
				a := InstalledApp{
					Name:      fields[0],
					Namespace: fields[1],
					Revision:  fields[2],
					// fields[3] = updated timestamp (skip, it spans multiple tokens)
					Status: fields[len(fields)-2],
					Chart:  fields[len(fields)-3],
				}
				a.AppVer = fields[len(fields)-1]
				apps = append(apps, a)
			}
		}
	}

	jsonResponse(w, http.StatusOK, apps)
}

// ---------- POST /api/apps/install ----------

func (h *Handlers) InstallApp(w http.ResponseWriter, r *http.Request) {
	var req InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Find the app in catalog
	var app *App
	for i := range h.Catalog {
		if h.Catalog[i].Name == req.Name {
			app = &h.Catalog[i]
			break
		}
	}
	if app == nil {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("app %q not found in catalog", req.Name))
		return
	}

	version := req.Version
	if version == "" {
		version = app.Version
	}

	releaseName := "packalares-" + app.Name
	env := h.helmEnv()

	log.Printf("Installing %s (version %s) as release %s", app.Name, version, releaseName)

	args := []string{
		"upgrade", "--install", releaseName, app.ChartURL,
		"--namespace", h.Config.Namespace,
		"--create-namespace",
		"--version", version,
		"--wait",
		"--timeout", "5m",
	}

	out, err := runCmd("helm", env, args...)
	if err != nil {
		log.Printf("Install failed for %s: %v: %s", app.Name, err, out)
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("helm install: %v: %s", err, out))
		return
	}

	log.Printf("Installed %s successfully", app.Name)
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "installed",
		"name":    app.Name,
		"release": releaseName,
		"version": version,
		"output":  out,
	})
}

// ---------- POST /api/apps/uninstall ----------

func (h *Handlers) UninstallApp(w http.ResponseWriter, r *http.Request) {
	var req InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}

	releaseName := "packalares-" + req.Name
	env := h.helmEnv()

	log.Printf("Uninstalling %s (release %s)", req.Name, releaseName)

	out, err := runCmd("helm", env, "uninstall", releaseName, "-n", h.Config.Namespace)
	if err != nil {
		log.Printf("Uninstall failed for %s: %v: %s", req.Name, err, out)
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("helm uninstall: %v: %s", err, out))
		return
	}

	log.Printf("Uninstalled %s successfully", req.Name)
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "uninstalled",
		"name":    req.Name,
		"release": releaseName,
		"output":  out,
	})
}
