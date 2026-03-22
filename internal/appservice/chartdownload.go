package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

const (
	// defaultGitHubAPI is the base URL for the GitHub contents API.
	defaultGitHubAPI = "https://api.github.com"
	// defaultChartsRepo is the GitHub owner/repo for beclab community apps.
	defaultChartsRepo = "beclab/apps"
	// chartCacheDir is where downloaded charts are stored.
	chartCacheDir = "/tmp/charts"
)

// ChartDownloader fetches Helm charts from a GitHub repository that stores
// raw chart directories (Chart.yaml, values.yaml, templates/, etc.).
type ChartDownloader struct {
	githubAPI  string
	repo       string
	httpClient *http.Client
}

// NewChartDownloader creates a downloader targeting the beclab/apps GitHub repo.
func NewChartDownloader() *ChartDownloader {
	return &ChartDownloader{
		githubAPI: defaultGitHubAPI,
		repo:      defaultChartsRepo,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// githubContent represents one entry in the GitHub Contents API response.
type githubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"` // "file" or "dir"
	DownloadURL string `json:"download_url"`
	Size        int    `json:"size"`
}

// DownloadChart downloads an app's chart directory from GitHub and saves it
// locally to /tmp/charts/{appname}/. Returns the local path to the chart.
func (d *ChartDownloader) DownloadChart(ctx context.Context, appName string) (string, error) {
	destDir := filepath.Join(chartCacheDir, appName)

	// Clean up any previous download
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create chart dir %s: %w", destDir, err)
	}

	klog.Infof("downloading chart for %s from %s/%s", appName, d.repo, appName)

	if err := d.downloadDir(ctx, appName, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return "", fmt.Errorf("download chart %s: %w", appName, err)
	}

	// Verify Chart.yaml exists
	chartYaml := filepath.Join(destDir, "Chart.yaml")
	if _, err := os.Stat(chartYaml); os.IsNotExist(err) {
		_ = os.RemoveAll(destDir)
		return "", fmt.Errorf("downloaded chart for %s is missing Chart.yaml", appName)
	}

	klog.Infof("chart for %s downloaded to %s", appName, destDir)
	return destDir, nil
}

// downloadDir recursively downloads all files from a GitHub directory.
func (d *ChartDownloader) downloadDir(ctx context.Context, repoPath, localDir string) error {
	url := fmt.Sprintf("%s/repos/%s/contents/%s", d.githubAPI, d.repo, repoPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Use GitHub token if available to avoid rate limits
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("chart %q not found in %s (404)", repoPath, d.repo)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("GitHub API returned %d for %s: %s", resp.StatusCode, repoPath, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response for %s: %w", repoPath, err)
	}

	var contents []githubContent
	if err := json.Unmarshal(body, &contents); err != nil {
		return fmt.Errorf("parse GitHub response for %s: %w", repoPath, err)
	}

	for _, entry := range contents {
		localPath := filepath.Join(localDir, entry.Name)

		switch entry.Type {
		case "file":
			if err := d.downloadFile(ctx, entry.DownloadURL, localPath); err != nil {
				return fmt.Errorf("download %s: %w", entry.Path, err)
			}

		case "dir":
			if err := os.MkdirAll(localPath, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", localPath, err)
			}
			if err := d.downloadDir(ctx, entry.Path, localPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// downloadFile downloads a single file from its raw download URL.
func (d *ChartDownloader) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch file %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write %s: %w", destPath, err)
	}

	return nil
}

// ParseOlaresManifest reads and parses the OlaresManifest.yaml (or
// TerminusManifest.yaml as a fallback) from a chart directory.
func ParseOlaresManifest(chartDir string) (*AppConfiguration, error) {
	// Try OlaresManifest.yaml first, then TerminusManifest.yaml (legacy name)
	candidates := []string{
		filepath.Join(chartDir, "OlaresManifest.yaml"),
		filepath.Join(chartDir, "TerminusManifest.yaml"),
	}

	var data []byte
	var readErr error
	for _, path := range candidates {
		data, readErr = os.ReadFile(path)
		if readErr == nil {
			break
		}
	}

	if readErr != nil {
		return nil, fmt.Errorf("no OlaresManifest.yaml or TerminusManifest.yaml in %s", chartDir)
	}

	var cfg AppConfiguration
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return &cfg, nil
}

// ParseChartMetadata reads Chart.yaml from a chart directory and returns
// the chart version and app version.
func ParseChartMetadata(chartDir string) (chartVersion, appVersion string, err error) {
	data, err := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
	if err != nil {
		return "", "", fmt.Errorf("read Chart.yaml: %w", err)
	}

	var chart struct {
		Version    string `yaml:"version"`
		AppVersion string `yaml:"appVersion"`
	}
	if err := yaml.Unmarshal(data, &chart); err != nil {
		return "", "", fmt.Errorf("parse Chart.yaml: %w", err)
	}

	return chart.Version, chart.AppVersion, nil
}

// CleanupChart removes a downloaded chart directory.
func CleanupChart(appName string) {
	dir := filepath.Join(chartCacheDir, appName)
	if err := os.RemoveAll(dir); err != nil {
		klog.V(2).Infof("cleanup chart %s: %v", dir, err)
	}
}

// BuildEntrancesFromManifest converts OlaresManifest entrances into the
// Entrance type used by the app record, filling in host names based on
// the app name and owner.
func BuildEntrancesFromManifest(manifest *AppConfiguration, appName, owner, namespace string) []Entrance {
	if manifest == nil || len(manifest.Entrances) == 0 {
		return nil
	}

	entrances := make([]Entrance, 0, len(manifest.Entrances))
	for _, e := range manifest.Entrances {
		entrance := Entrance{
			Name:       e.Name,
			Port:       e.Port,
			Icon:       e.Icon,
			Title:      e.Title,
			AuthLevel:  e.AuthLevel,
			Invisible:  e.Invisible,
			OpenMethod: e.OpenMethod,
		}

		// Build the host from the entrance name and app release
		if e.Host != "" {
			entrance.Host = e.Host
		} else {
			entrance.Host = fmt.Sprintf("%s-svc.%s", appName, namespace)
		}

		// Build the URL: the desktop uses this to navigate
		host := strings.ReplaceAll(entrance.Host, "{{ .Values.bfl.username }}", owner)
		if entrance.Port > 0 {
			entrance.URL = fmt.Sprintf("http://%s:%d", host, entrance.Port)
		} else {
			entrance.URL = fmt.Sprintf("http://%s", host)
		}

		entrances = append(entrances, entrance)
	}

	return entrances
}
