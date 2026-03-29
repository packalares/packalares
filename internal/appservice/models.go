package appservice

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// ModelBackend handles model installation for a specific inference engine.
type ModelBackend interface {
	// Install pulls/deploys the model. Should broadcast progress via wsHub.
	Install(ctx context.Context, model ModelSpec, wsHub *WSHub) error
	// Uninstall removes the model.
	Uninstall(ctx context.Context, model ModelSpec) error
	// InstalledModels returns model IDs currently available on this backend.
	InstalledModels(ctx context.Context) ([]InstalledModel, error)
}

// ModelSpec describes a model to install or uninstall.
type ModelSpec struct {
	Name    string `json:"name"`              // catalog name, e.g. "gemma3-27b"
	ModelID string `json:"modelId"`           // backend-specific ID, e.g. "gemma3:27b" for ollama
	Backend string `json:"backend"`           // "ollama" or "vllm"
	Title   string `json:"title,omitempty"`
	HFRepo  string `json:"hfRepo,omitempty"`  // for vllm: HuggingFace repo
	HFRef   string `json:"hfRef,omitempty"`   // for vllm: branch/ref

	// vLLM-specific fields (optional, used by VLLMBackend)
	GPUMemoryUtilization string `json:"gpuMemoryUtilization,omitempty"`
	MaxModelLen          string `json:"maxModelLen,omitempty"`
	TensorParallelSize   int    `json:"tensorParallelSize,omitempty"`
	TiktokenFiles        string `json:"tiktokenFiles,omitempty"` // comma-separated
	StoragePath          string `json:"storagePath,omitempty"`   // host path for model data
}

// InstalledModel represents a model available on a backend.
type InstalledModel struct {
	Name     string `json:"name"`     // model name/tag
	Size     int64  `json:"size"`     // size in bytes
	Modified string `json:"modified"` // last modified time
}

// ---------------------------------------------------------------------------
// OllamaBackend — talks to a running Ollama instance via its HTTP API
// ---------------------------------------------------------------------------

// OllamaBackend implements ModelBackend for Ollama.
type OllamaBackend struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllamaBackend creates an OllamaBackend targeting the given base URL.
func NewOllamaBackend(baseURL string) *OllamaBackend {
	return &OllamaBackend{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			// No timeout — model pulls can take a long time.
			Timeout: 0,
		},
	}
}

// Install pulls a model via POST /api/pull with streaming progress.
func (o *OllamaBackend) Install(ctx context.Context, model ModelSpec, wsHub *WSHub) error {
	modelID := model.ModelID
	if modelID == "" {
		modelID = model.Name
	}

	body, err := json.Marshal(map[string]interface{}{
		"name":   modelID,
		"stream": true,
	})
	if err != nil {
		return fmt.Errorf("marshal pull request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/pull", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create pull request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama pull %s: %w", modelID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ollama pull %s: status %d: %s", modelID, resp.StatusCode, string(respBody))
	}

	// Read streaming JSON lines for progress.
	// Track cumulative bytes across all layers/digests to show accurate total progress.
	layerTotals := map[string]int64{}
	layerCompleted := map[string]int64{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var progress struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &progress); err != nil {
			klog.V(3).Infof("ollama pull: skip non-JSON line: %s", line)
			continue
		}

		if progress.Error != "" {
			return fmt.Errorf("ollama pull %s: %s", modelID, progress.Error)
		}

		// Track per-layer progress
		if progress.Digest != "" {
			layerTotals[progress.Digest] = progress.Total
			layerCompleted[progress.Digest] = progress.Completed
		}

		// Sum across all layers for cumulative progress
		var cumCompleted, cumTotal int64
		for d, t := range layerTotals {
			cumTotal += t
			cumCompleted += layerCompleted[d]
		}

		detail := progress.Status
		if progress.Digest != "" && len(progress.Digest) > 12 {
			detail = progress.Status + " " + progress.Digest[:12]
		}

		wsHub.BroadcastInstallProgress(
			model.Name,
			StateDownloading,
			1, 1,
			detail,
			cumCompleted,
			cumTotal,
		)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ollama pull %s: read stream: %w", modelID, err)
	}

	// Broadcast success
	wsHub.BroadcastAppState(model.Name, StateRunning)
	wsHub.BroadcastInstallProgress(model.Name, StateRunning, 1, 1, "Model ready", 0, 0)

	klog.Infof("ollama model %s (%s) pulled successfully", model.Name, modelID)
	return nil
}

// Uninstall removes a model via POST /api/delete.
func (o *OllamaBackend) Uninstall(ctx context.Context, model ModelSpec) error {
	modelID := model.ModelID
	if modelID == "" {
		modelID = model.Name
	}

	body, err := json.Marshal(map[string]string{"name": modelID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, o.baseURL+"/api/delete", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama delete %s: %w", modelID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ollama delete %s: status %d: %s", modelID, resp.StatusCode, string(respBody))
	}

	klog.Infof("ollama model %s (%s) deleted", model.Name, modelID)
	return nil
}

// InstalledModels lists models via GET /api/tags.
func (o *OllamaBackend) InstalledModels(ctx context.Context) ([]InstalledModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	// Short timeout for listing
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama tags: status %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama tags decode: %w", err)
	}

	models := make([]InstalledModel, 0, len(result.Models))
	for _, m := range result.Models {
		models = append(models, InstalledModel{
			Name:     m.Name,
			Size:     m.Size,
			Modified: m.ModifiedAt,
		})
	}
	return models, nil
}

// ---------------------------------------------------------------------------
// VLLMBackend — deploys vLLM models as Helm releases (pods)
// ---------------------------------------------------------------------------

const (
	// defaultVLLMChartPath is where the generic vLLM chart is installed.
	// In the appservice container this is copied from deploy/charts/vllm-model/.
	defaultVLLMChartPath = "/usr/share/packalares/charts/vllm-model"
	// defaultModelStorageBase is the base host path for model data.
	defaultModelStorageBase = "/packalares/data/Huggingface"
)

// VLLMBackend implements ModelBackend for vLLM-based model serving.
// It deploys a pod with a vLLM server and an HF downloader sidecar
// using the generic vllm-model Helm chart.
type VLLMBackend struct {
	helm      *HelmClient
	namespace string
	chartPath string // path to the generic vllm-model chart
}

// NewVLLMBackend creates a VLLMBackend.
func NewVLLMBackend(helm *HelmClient, namespace string) *VLLMBackend {
	chartPath := os.Getenv("VLLM_CHART_PATH")
	if chartPath == "" {
		chartPath = defaultVLLMChartPath
	}
	return &VLLMBackend{helm: helm, namespace: namespace, chartPath: chartPath}
}

// Install deploys a vLLM model by copying the generic chart to a temp directory,
// writing model-specific values, and running helm install.
func (v *VLLMBackend) Install(ctx context.Context, model ModelSpec, wsHub *WSHub) error {
	if model.HFRepo == "" {
		return fmt.Errorf("vllm install %s: hfRepo is required", model.Name)
	}

	// Check server has enough memory for the vLLM model
	memRequired := parseResourceSize(v.getResourceValue(model, "memory"))
	if memRequired > 0 {
		availMem := getAvailableMemory()
		if availMem > 0 && availMem < memRequired {
			return fmt.Errorf("insufficient memory: need %s, available %s",
				formatResourceSize(memRequired), formatResourceSize(availMem))
		}
	}

	wsHub.BroadcastInstallProgress(model.Name, StateInstalling, 1, 3, "Preparing chart...", 0, 0)

	// Build values override from the model spec.
	values := v.buildValues(model)

	// Copy chart to a temp directory so we can write values without
	// modifying the original.
	tmpDir, err := os.MkdirTemp("", "vllm-chart-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	chartDir := filepath.Join(tmpDir, "vllm-model")
	if err := copyDir(v.chartPath, chartDir); err != nil {
		return fmt.Errorf("copy chart: %w", err)
	}

	// Write the values override into the chart's values.yaml.
	if err := injectValuesYaml(chartDir, values); err != nil {
		return fmt.Errorf("inject values: %w", err)
	}

	// Helm install.
	wsHub.BroadcastInstallProgress(model.Name, StateInstalling, 2, 3, "Installing helm release...", 0, 0)

	releaseName := "vllm-" + model.Name
	if err := v.helm.InstallFromDir(ctx, releaseName, chartDir, v.namespace); err != nil {
		return fmt.Errorf("helm install: %w", err)
	}

	wsHub.BroadcastInstallProgress(model.Name, StateDownloading, 3, 3, "Waiting for pod to start...", 0, 0)

	// Monitor hf-downloader logs for download progress until .done file appears.
	// This blocks until download completes or context is cancelled.
	v.monitorDownload(ctx, model, releaseName, wsHub)

	// Register the vLLM endpoint in OpenWebUI so it auto-discovers the model
	vllmURL := fmt.Sprintf("http://%s-api.%s:8000/v1", releaseName, v.namespace)
	v.addOpenWebUIEndpoint(vllmURL)

	wsHub.BroadcastAppState(model.Name, StateRunning)
	klog.Infof("vllm model %s deployed (release=%s, repo=%s)", model.Name, releaseName, model.HFRepo)
	return nil
}

// monitorDownload tails the hf-downloader container logs and broadcasts progress.
// Blocks until the download completes (container exits) or context is cancelled.
func (v *VLLMBackend) monitorDownload(ctx context.Context, model ModelSpec, releaseName string, wsHub *WSHub) {
	// Wait for pod to be ready
	podName := ""
	for i := 0; i < 60; i++ {
		cmd := exec.CommandContext(ctx, "kubectl", "get", "pods",
			"-n", v.namespace,
			"-l", "app.kubernetes.io/instance="+releaseName,
			"-o", "jsonpath={.items[0].metadata.name}",
		)
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			podName = strings.TrimSpace(string(out))
			break
		}
		time.Sleep(2 * time.Second)
	}
	if podName == "" {
		klog.Warningf("vllm %s: could not find pod for progress monitoring", model.Name)
		return
	}

	// Tail the hf-downloader logs
	cmd := exec.CommandContext(ctx, "kubectl", "logs",
		"-n", v.namespace,
		podName,
		"-c", "hf-downloader",
		"-f", // follow
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		klog.Warningf("vllm %s: could not tail logs: %v", model.Name, err)
		return
	}
	if err := cmd.Start(); err != nil {
		klog.Warningf("vllm %s: could not start log tail: %v", model.Name, err)
		return
	}

	// Track cumulative progress across all files
	fileTotals := map[string]int64{}
	fileCompleted := map[string]int64{}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		var progress struct {
			Status    string `json:"status"`
			File      string `json:"file"`
			Completed int64  `json:"completed"`
			Total     int64  `json:"total"`
			Error     string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &progress); err != nil {
			continue
		}

		if progress.Error != "" {
			klog.Warningf("vllm %s download error: %s", model.Name, progress.Error)
			continue
		}

		if progress.File != "" && progress.Total > 0 {
			fileTotals[progress.File] = progress.Total
			fileCompleted[progress.File] = progress.Completed
		}

		// Sum across all files
		var cumCompleted, cumTotal int64
		for f, t := range fileTotals {
			cumTotal += t
			cumCompleted += fileCompleted[f]
		}

		// Short file name for display
		short := progress.File
		if idx := strings.LastIndex(short, "/"); idx >= 0 {
			short = short[idx+1:]
		}

		detail := fmt.Sprintf("Downloading %s (%d/%d files)", short, len(fileCompleted), len(fileTotals))
		wsHub.BroadcastInstallProgress(model.Name, StateDownloading, 1, 1, detail, cumCompleted, cumTotal)
	}

	// Wait for the log tail to finish (container exited)
	_ = cmd.Wait()
	klog.Infof("vllm %s: hf-downloader finished", model.Name)
}

// buildValues creates the Helm values map for the vllm-model chart.
func (v *VLLMBackend) buildValues(model ModelSpec) map[string]interface{} {
	hfRef := model.HFRef
	if hfRef == "" {
		hfRef = "main"
	}

	storagePath := model.StoragePath
	if storagePath == "" {
		storagePath = filepath.Join(defaultModelStorageBase, model.Name)
	}

	gpuMem := model.GPUMemoryUtilization
	if gpuMem == "" {
		gpuMem = "0.9"
	}

	maxLen := model.MaxModelLen
	if maxLen == "" {
		maxLen = "auto"
	}

	tps := model.TensorParallelSize
	if tps <= 0 {
		tps = 1
	}

	hfToken := os.Getenv("OLARES_USER_HUGGINGFACE_TOKEN")
	hfEndpoint := os.Getenv("HF_ENDPOINT")
	if hfEndpoint == "" {
		hfEndpoint = "https://huggingface.co"
	}

	vals := map[string]interface{}{
		"model": map[string]interface{}{
			"name":        model.Name,
			"hfRepo":      model.HFRepo,
			"hfRef":       hfRef,
			"doneName":    ".done",
			"storagePath": storagePath,
		},
		"vllm": map[string]interface{}{
			"gpuMemoryUtilization": gpuMem,
			"maxModelLen":          maxLen,
			"tensorParallelSize":   tps,
		},
		"hf": map[string]interface{}{
			"token":    hfToken,
			"endpoint": hfEndpoint,
		},
	}

	if model.TiktokenFiles != "" {
		vals["tiktoken"] = map[string]interface{}{
			"enabled": true,
			"files":   model.TiktokenFiles,
			"dir":     "/data/tiktoken",
		}
	}

	return vals
}

// Uninstall removes a vLLM model's Helm release and cleans up OpenWebUI config.
func (v *VLLMBackend) Uninstall(ctx context.Context, model ModelSpec) error {
	releaseName := "vllm-" + model.Name
	if err := v.helm.Uninstall(ctx, releaseName); err != nil {
		return err
	}

	// Remove this model's endpoint from OpenWebUI
	vllmURL := fmt.Sprintf("http://%s-api.%s:8000/v1", releaseName, v.namespace)
	v.removeOpenWebUIEndpoint(vllmURL)

	// Clean up model data on disk
	storagePath := model.StoragePath
	if storagePath == "" {
		storagePath = filepath.Join(defaultModelStorageBase, model.Name)
	}
	if err := os.RemoveAll(storagePath); err != nil {
		klog.Warningf("vllm %s: failed to clean model data at %s: %v", model.Name, storagePath, err)
	} else {
		klog.Infof("vllm %s: cleaned model data at %s", model.Name, storagePath)
	}

	return nil
}

// InstalledModels lists vLLM models by checking Helm releases with a model label.
func (v *VLLMBackend) InstalledModels(ctx context.Context) ([]InstalledModel, error) {
	releases, err := v.helm.ListReleases(ctx)
	if err != nil {
		return nil, err
	}

	var models []InstalledModel
	for _, r := range releases {
		// Look for releases whose chart name contains "vllm" as a heuristic
		if strings.Contains(r.Chart, "vllm") {
			// Strip "vllm-" prefix to match catalog modelId
			name := r.Name
			if strings.HasPrefix(name, "vllm-") {
				name = strings.TrimPrefix(name, "vllm-")
			}
			// Check if model is still downloading (hf-downloader container running)
			state := "ready"
			podStatus := exec.CommandContext(ctx, "kubectl", "get", "pods",
				"-n", v.namespace,
				"-l", "app.kubernetes.io/instance="+r.Name,
				"-o", "jsonpath={.items[0].status.containerStatuses[?(@.name==\"hf-downloader\")].state}")
			if out, err := podStatus.Output(); err == nil {
				s := string(out)
				if strings.Contains(s, "running") || strings.Contains(s, "waiting") {
					state = "downloading"
				}
			}
			models = append(models, InstalledModel{
				Name:     name,
				Size:     0,
				Modified: state, // reuse Modified field for status
			})
		}
	}
	return models, nil
}

// addOpenWebUIEndpoint adds a vLLM URL to OpenWebUI's OPENAI_API_BASE_URLS env var.
// If the URL is already present, this is a no-op.
func (v *VLLMBackend) addOpenWebUIEndpoint(url string) {
	ctx := context.Background()
	current := v.getOpenWebUIEnv(ctx, "OPENAI_API_BASE_URLS")
	urls := splitNonEmpty(current, ";")

	for _, u := range urls {
		if u == url {
			return // already present
		}
	}
	urls = append(urls, url)
	newVal := strings.Join(urls, ";")

	cmd := exec.CommandContext(ctx, "kubectl", "set", "env",
		fmt.Sprintf("deploy/openwebui"), fmt.Sprintf("OPENAI_API_BASE_URLS=%s", newVal),
		"-n", v.namespace)
	if out, err := cmd.CombinedOutput(); err != nil {
		klog.Warningf("failed to add vLLM endpoint to OpenWebUI: %v: %s", err, string(out))
	} else {
		klog.Infof("added vLLM endpoint %s to OpenWebUI", url)
	}
}

// removeOpenWebUIEndpoint removes a vLLM URL from OpenWebUI's OPENAI_API_BASE_URLS.
func (v *VLLMBackend) removeOpenWebUIEndpoint(url string) {
	ctx := context.Background()
	current := v.getOpenWebUIEnv(ctx, "OPENAI_API_BASE_URLS")
	urls := splitNonEmpty(current, ";")

	var filtered []string
	for _, u := range urls {
		if u != url {
			filtered = append(filtered, u)
		}
	}
	newVal := strings.Join(filtered, ";")

	cmd := exec.CommandContext(ctx, "kubectl", "set", "env",
		"deploy/openwebui", fmt.Sprintf("OPENAI_API_BASE_URLS=%s", newVal),
		"-n", v.namespace)
	if out, err := cmd.CombinedOutput(); err != nil {
		klog.Warningf("failed to remove vLLM endpoint from OpenWebUI: %v: %s", err, string(out))
	} else {
		klog.Infof("removed vLLM endpoint %s from OpenWebUI", url)
	}
}

// getOpenWebUIEnv reads an env var from the OpenWebUI deployment.
func (v *VLLMBackend) getOpenWebUIEnv(ctx context.Context, envName string) string {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "deploy", "openwebui",
		"-n", v.namespace,
		"-o", fmt.Sprintf("jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"%s\")].value}", envName))
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// splitNonEmpty splits s by sep and returns non-empty parts.
func splitNonEmpty(s, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// getResourceValue returns the memory request value for a model.
func (v *VLLMBackend) getResourceValue(model ModelSpec, resource string) string {
	if resource == "memory" {
		return "20Gi" // default vLLM memory request
	}
	return ""
}

// parseResourceSize parses Kubernetes resource strings like "20Gi", "500Mi" to bytes.
func parseResourceSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var multiplier int64 = 1
	if strings.HasSuffix(s, "Gi") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "Gi")
	} else if strings.HasSuffix(s, "Mi") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "Mi")
	} else if strings.HasSuffix(s, "Ki") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "Ki")
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}

// formatResourceSize formats bytes as human-readable.
func formatResourceSize(bytes int64) string {
	if bytes >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	}
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.0f MB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%d bytes", bytes)
}

// getAvailableMemory reads available memory from /proc/meminfo.
func getAvailableMemory() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return val * 1024 // kB to bytes
				}
			}
		}
	}
	return 0
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
				return err
			}
		}
	}

	return nil
}
