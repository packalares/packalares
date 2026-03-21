package apps

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/packalares/packalares/core/config"
)

type InstallRequest struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Values  map[string]string `json:"values,omitempty"`
}

type UninstallRequest struct {
	Name string `json:"name"`
}

type HelmRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

func InstallApp(req InstallRequest) error {
	cfg := config.Load()

	entry, err := GetCatalogEntry(req.Name)
	if err != nil {
		return fmt.Errorf("app not found in catalog: %w", err)
	}

	releaseName := cfg.HelmPrefix + req.Name

	version := req.Version
	if version == "" {
		version = entry.Version
	}

	addCmd := exec.Command("helm", "repo", "add", req.Name, entry.HelmRepo)
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("helm repo add: %s: %w", string(out), err)
	}

	updateCmd := exec.Command("helm", "repo", "update", req.Name)
	if out, err := updateCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("helm repo update: %s: %w", string(out), err)
	}

	installArgs := []string{
		"install", releaseName, entry.HelmChart,
		"--namespace", cfg.AppNamespace,
		"--create-namespace",
		"--wait",
		"--timeout", "10m",
	}

	if version != "" {
		installArgs = append(installArgs, "--version", version)
	}

	for k, v := range req.Values {
		installArgs = append(installArgs, "--set", k+"="+v)
	}

	installCmd := exec.Command("helm", installArgs...)
	if out, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("helm install: %s: %w", string(out), err)
	}

	return nil
}

func UninstallApp(name string) error {
	cfg := config.Load()
	releaseName := cfg.HelmPrefix + name

	cmd := exec.Command("helm", "uninstall", releaseName, "--namespace", cfg.AppNamespace)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("helm uninstall: %s: %w", string(out), err)
	}

	return nil
}

func ListInstalled() ([]HelmRelease, error) {
	cfg := config.Load()

	cmd := exec.Command("helm", "list", "--namespace", cfg.AppNamespace, "--output", "json")
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

	var managed []HelmRelease
	for _, r := range releases {
		if strings.HasPrefix(r.Name, cfg.HelmPrefix) {
			managed = append(managed, r)
		}
	}

	return managed, nil
}

type AppStatus struct {
	Name       string    `json:"name"`
	Release    string    `json:"release"`
	Status     string    `json:"status"`
	Version    string    `json:"version"`
	AppVersion string    `json:"app_version"`
	Pods       []PodInfo `json:"pods"`
	UpdatedAt  string    `json:"updated_at"`
}

type PodInfo struct {
	Name   string `json:"name"`
	Ready  string `json:"ready"`
	Status string `json:"status"`
	Age    string `json:"age"`
}

func GetAppStatus(name string) (*AppStatus, error) {
	cfg := config.Load()
	releaseName := cfg.HelmPrefix + name

	cmd := exec.Command("helm", "status", releaseName, "--namespace", cfg.AppNamespace, "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("app %q not found or not installed", name)
	}

	var helmStatus struct {
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
	if err := json.Unmarshal(out, &helmStatus); err != nil {
		return nil, fmt.Errorf("parse helm status: %w", err)
	}

	pods := getPodsForRelease(releaseName, cfg.AppNamespace)

	return &AppStatus{
		Name:       name,
		Release:    releaseName,
		Status:     helmStatus.Info.Status,
		Version:    helmStatus.Chart.Metadata.Version,
		AppVersion: helmStatus.Chart.Metadata.AppVersion,
		Pods:       pods,
		UpdatedAt:  helmStatus.Info.LastDeployed.Format(time.RFC3339),
	}, nil
}

func getPodsForRelease(release, namespace string) []PodInfo {
	cmd := exec.Command("kubectl", "get", "pods",
		"--namespace", namespace,
		"-l", "app.kubernetes.io/instance="+release,
		"-o", "jsonpath={range .items[*]}{.metadata.name}|{.status.phase}|{.metadata.creationTimestamp}{\"\\n\"}{end}",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var pods []PodInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		created, _ := time.Parse(time.RFC3339, parts[2])
		age := time.Since(created).Truncate(time.Second).String()

		pods = append(pods, PodInfo{
			Name:   parts[0],
			Status: parts[1],
			Age:    age,
		})
	}

	return pods
}
