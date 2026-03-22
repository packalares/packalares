package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// HelmClient wraps helm CLI operations for app install/uninstall/lifecycle.
type HelmClient struct {
	Namespace string
	RepoURL   string // default chart repo URL
}

// NewHelmClient creates a helm client that targets the given namespace.
func NewHelmClient(namespace, repoURL string) *HelmClient {
	return &HelmClient{
		Namespace: namespace,
		RepoURL:   repoURL,
	}
}

// Install installs a helm chart. If chartPath is a local directory it installs
// from that path; otherwise it treats it as repo/chart.
func (h *HelmClient) Install(ctx context.Context, releaseName, chartRef string, values map[string]string, version string) error {
	args := []string{
		"install", releaseName, chartRef,
		"--namespace", h.Namespace,
		"--create-namespace",
		"--wait",
		"--timeout", "10m",
	}

	if version != "" {
		args = append(args, "--version", version)
	}

	for k, v := range values {
		args = append(args, "--set", k+"="+v)
	}

	klog.Infof("helm install: %s", strings.Join(args, " "))
	return h.run(ctx, args...)
}

// InstallFromDir installs a helm chart from a local directory with no --wait,
// no --set, and no --timeout. The chart's values.yaml should already contain
// all needed values (injected by injectValuesYaml). The app may take time to
// become ready (e.g. waiting for the middleware operator to provision a DB),
// and that is fine -- pods will retry until dependencies are available.
func (h *HelmClient) InstallFromDir(ctx context.Context, releaseName, chartDir, namespace string) error {
	args := []string{
		"install", releaseName, chartDir,
		"--namespace", namespace,
		"--create-namespace",
	}
	klog.Infof("helm install (from dir): %s", strings.Join(args, " "))
	return h.run(ctx, args...)
}

// Uninstall removes a helm release.
func (h *HelmClient) Uninstall(ctx context.Context, releaseName string) error {
	args := []string{
		"uninstall", releaseName,
		"--namespace", h.Namespace,
	}
	klog.Infof("helm uninstall: %s", strings.Join(args, " "))
	return h.run(ctx, args...)
}

// Upgrade upgrades a helm release in place.
func (h *HelmClient) Upgrade(ctx context.Context, releaseName, chartRef string, values map[string]string, version string) error {
	args := []string{
		"upgrade", releaseName, chartRef,
		"--namespace", h.Namespace,
		"--wait",
		"--timeout", "10m",
	}

	if version != "" {
		args = append(args, "--version", version)
	}

	for k, v := range values {
		args = append(args, "--set", k+"="+v)
	}

	klog.Infof("helm upgrade: %s", strings.Join(args, " "))
	return h.run(ctx, args...)
}

// ListReleases returns all helm releases in the configured namespace.
func (h *HelmClient) ListReleases(ctx context.Context) ([]HelmRelease, error) {
	args := []string{
		"list",
		"--namespace", h.Namespace,
		"--output", "json",
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("helm list: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("helm list: %w", err)
	}

	var releases []HelmRelease
	if err := json.Unmarshal(out, &releases); err != nil {
		return nil, fmt.Errorf("parse helm output: %w", err)
	}

	return releases, nil
}

// GetStatus returns the helm status for a release.
func (h *HelmClient) GetStatus(ctx context.Context, releaseName string) (*HelmReleaseStatus, error) {
	args := []string{
		"status", releaseName,
		"--namespace", h.Namespace,
		"--output", "json",
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("release %q not found", releaseName)
	}

	var status HelmReleaseStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parse helm status: %w", err)
	}

	return &status, nil
}

// AddRepo adds a helm repository.
func (h *HelmClient) AddRepo(ctx context.Context, name, url string) error {
	return h.run(ctx, "repo", "add", name, url)
}

// UpdateRepo updates helm repositories.
func (h *HelmClient) UpdateRepo(ctx context.Context, name string) error {
	return h.run(ctx, "repo", "update", name)
}

func (h *HelmClient) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm %s: %s: %w", args[0], string(out), err)
	}
	return nil
}

// HelmRelease matches helm list JSON output.
type HelmRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

// HelmReleaseStatus matches helm status JSON output.
type HelmReleaseStatus struct {
	Name string `json:"name"`
	Info struct {
		Status       string    `json:"status"`
		LastDeployed time.Time `json:"last_deployed"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
}
