package appservice

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

const (
	// chartCacheDir is where downloaded charts are stored.
	chartCacheDir = "/tmp/charts"
	// defaultLocalChartRepo is the in-cluster market-backend chart server.
	defaultLocalChartRepo = "http://market-backend:6756"
)

// ChartDownloader fetches Helm charts from the local market-backend service.
type ChartDownloader struct {
	httpClient     *http.Client
	localChartRepo string
}

// NewChartDownloader creates a downloader targeting the local market-backend.
func NewChartDownloader() *ChartDownloader {
	localRepo := os.Getenv("LOCAL_CHART_REPO")
	if localRepo == "" {
		localRepo = defaultLocalChartRepo
	}
	return &ChartDownloader{
		localChartRepo: localRepo,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// DownloadChart downloads an app's chart from the local market-backend.
// All charts are baked into the market image — no external fallback.
func (d *ChartDownloader) DownloadChart(ctx context.Context, appName string) (string, error) {
	destDir := filepath.Join(chartCacheDir, appName)

	localPath, err := d.DownloadChartFromRepo(ctx, d.localChartRepo, appName, destDir)
	if err != nil {
		return "", fmt.Errorf("chart %s not found in local catalog: %w", appName, err)
	}

	klog.Infof("chart for %s downloaded from local repo to %s", appName, localPath)
	return localPath, nil
}

// DownloadChartFromRepo downloads a pre-packaged .tgz chart from a Helm chart
// repository and unpacks it into destDir. It first fetches the repo's index.yaml
// to find the chart URL, then downloads and extracts the .tgz.
func (d *ChartDownloader) DownloadChartFromRepo(ctx context.Context, chartRepoURL, appName, destDir string) (string, error) {
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create chart dir %s: %w", destDir, err)
	}

	chartURL, err := d.findChartInIndex(ctx, chartRepoURL, appName)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return "", fmt.Errorf("find chart %s in repo index: %w", appName, err)
	}

	if !strings.HasPrefix(chartURL, "http") {
		chartURL = strings.TrimSuffix(chartRepoURL, "/") + "/charts/" + chartURL
	}

	tgzPath := filepath.Join(destDir, appName+".tgz")
	if err := d.downloadFile(ctx, chartURL, tgzPath); err != nil {
		_ = os.RemoveAll(destDir)
		return "", fmt.Errorf("download chart tgz %s: %w", appName, err)
	}

	if err := unpackTGZ(tgzPath, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return "", fmt.Errorf("unpack chart %s: %w", appName, err)
	}

	_ = os.Remove(tgzPath)

	chartDir, err := findChartDir(destDir)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return "", fmt.Errorf("find chart dir for %s: %w", appName, err)
	}

	return chartDir, nil
}

// repoIndex is a minimal representation of a Helm repo index.yaml.
type repoIndex struct {
	Entries map[string][]repoChartVersion `yaml:"entries"`
}

type repoChartVersion struct {
	Name    string   `yaml:"name"`
	Version string   `yaml:"version"`
	URLs    []string `yaml:"urls"`
}

func (d *ChartDownloader) findChartInIndex(ctx context.Context, repoURL, appName string) (string, error) {
	indexURL := strings.TrimSuffix(repoURL, "/") + "/charts/index.yaml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch index.yaml: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("index.yaml returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("read index.yaml: %w", err)
	}

	var idx repoIndex
	if err := yaml.Unmarshal(body, &idx); err != nil {
		return "", fmt.Errorf("parse index.yaml: %w", err)
	}

	for entryName, versions := range idx.Entries {
		if entryName == appName || strings.HasPrefix(entryName, appName+"-") {
			if len(versions) > 0 && len(versions[0].URLs) > 0 {
				return versions[0].URLs[0], nil
			}
		}
	}

	return "", fmt.Errorf("chart %q not found in repo index", appName)
}

func (d *ChartDownloader) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
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

func unpackTGZ(tgzPath, destDir string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			continue // prevent directory traversal
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

func findChartDir(destDir string) (string, error) {
	if _, err := os.Stat(filepath.Join(destDir, "Chart.yaml")); err == nil {
		return destDir, nil
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subDir := filepath.Join(destDir, entry.Name())
			if _, err := os.Stat(filepath.Join(subDir, "Chart.yaml")); err == nil {
				return subDir, nil
			}
		}
	}

	return "", fmt.Errorf("no Chart.yaml found in %s or its subdirectories", destDir)
}

// ParseOlaresManifest reads the OlaresManifest.yaml from a chart directory.
func ParseOlaresManifest(chartDir string) (*AppConfiguration, error) {
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

	// Strip UTF-8 BOM
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	// Strip Helm template directives
	elseBlockRE := regexp.MustCompile(`(?s)\{\{-?\s*else\s*-?\}\}.*?\{\{-?\s*end\s*-?\}\}`)
	directiveRE := regexp.MustCompile(`\{\{-?\s*.*?\s*-?\}\}`)
	cleaned := elseBlockRE.ReplaceAll(data, nil)
	cleaned = directiveRE.ReplaceAll(cleaned, nil)

	var cfg AppConfiguration
	_ = yaml.Unmarshal(cleaned, &cfg)

	if cfg.Metadata.Name == "" {
		cfg.Metadata.Name = filepath.Base(chartDir)
	}

	return &cfg, nil
}

// ParseChartMetadata reads Chart.yaml and returns chart version and app version.
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

// BuildEntrancesFromManifest converts OlaresManifest entrances into Entrance types.
func BuildEntrancesFromManifest(manifest *AppConfiguration, appName, owner, namespace string) []Entrance {
	if manifest == nil || len(manifest.Entrances) == 0 {
		return nil
	}

	entrances := make([]Entrance, 0, len(manifest.Entrances))
	for _, e := range manifest.Entrances {
		icon := e.Icon
		if strings.HasPrefix(icon, "http") {
			icon = "/api/market/icons/" + appName + ".png"
		}
		entrance := Entrance{
			Name:       e.Name,
			Port:       e.Port,
			Icon:       icon,
			Title:      e.Title,
			AuthLevel:  e.AuthLevel,
			Invisible:  e.Invisible,
			OpenMethod: e.OpenMethod,
		}

		if e.Host != "" {
			entrance.Host = e.Host
		} else {
			entrance.Host = fmt.Sprintf("%s-svc.%s", appName, namespace)
		}

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
