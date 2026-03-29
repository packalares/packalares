package appservice

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// VLLMBackend implements ModelBackend for vLLM-based model serving.
// vLLM models are deployed as pods via Helm, so Install/Uninstall delegate
// to the existing Helm flow. The model card UI just categorizes them differently.
type VLLMBackend struct {
	helm      *HelmClient
	namespace string
}

// NewVLLMBackend creates a VLLMBackend.
func NewVLLMBackend(helm *HelmClient, namespace string) *VLLMBackend {
	return &VLLMBackend{helm: helm, namespace: namespace}
}

// Install deploys a vLLM model via Helm. For now this is a stub — the actual
// chart install is handled through the normal app install flow since vLLM
// models require pods.
func (v *VLLMBackend) Install(ctx context.Context, model ModelSpec, wsHub *WSHub) error {
	klog.Infof("vllm install: model %s — use app install flow for pod-based models", model.Name)
	wsHub.BroadcastInstallProgress(model.Name, StateInstalling, 1, 1, "vLLM models use app install flow", 0, 0)
	return fmt.Errorf("vllm models should be installed via the app install flow (they require pods)")
}

// Uninstall removes a vLLM model's Helm release.
func (v *VLLMBackend) Uninstall(ctx context.Context, model ModelSpec) error {
	return v.helm.Uninstall(ctx, model.Name)
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
			models = append(models, InstalledModel{
				Name:     r.Name,
				Size:     0,
				Modified: r.Updated,
			})
		}
	}
	return models, nil
}
