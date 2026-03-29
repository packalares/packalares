package hfdownload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultEndpoint  = "https://huggingface.co"
	defaultRef       = "main"
	defaultOutputDir = "/data"
	defaultDoneName  = ".done"
	maxRetries       = 3
)

// Config holds all configuration for a download run.
type Config struct {
	Repo      string // e.g. "openai/gpt-oss-20b"
	Ref       string // branch/ref, default "main"
	Token     string // optional Bearer token for gated models
	Endpoint  string // HuggingFace API base URL
	OutputDir string // where to save files
	DoneName  string // marker file name
}

// DefaultConfig returns config with defaults filled in.
func DefaultConfig() Config {
	return Config{
		Ref:       defaultRef,
		Endpoint:  defaultEndpoint,
		OutputDir: defaultOutputDir,
		DoneName:  defaultDoneName,
	}
}

// RepoFile is one entry from the HuggingFace tree API.
type RepoFile struct {
	Type string `json:"type"` // "file" or "directory"
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Client downloads model files from HuggingFace.
type Client struct {
	cfg        Config
	httpClient *http.Client
	progress   *ProgressReporter
}

// NewClient creates a HuggingFace download client.
func NewClient(cfg Config, progress *ProgressReporter) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 0}, // no timeout for large files
		progress:   progress,
	}
}

// ListFiles returns all files in the repository tree.
func (c *Client) ListFiles(ctx context.Context) ([]RepoFile, error) {
	url := fmt.Sprintf("%s/api/models/%s/tree/%s", c.cfg.Endpoint, c.cfg.Repo, c.cfg.Ref)

	var allFiles []RepoFile
	// The API may paginate; collect all entries.
	for url != "" {
		files, nextURL, err := c.listPage(ctx, url)
		if err != nil {
			return nil, err
		}
		allFiles = append(allFiles, files...)
		url = nextURL
	}

	return allFiles, nil
}

func (c *Client) listPage(ctx context.Context, url string) ([]RepoFile, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("list files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("list files: status %d: %s", resp.StatusCode, string(body))
	}

	var files []RepoFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, "", fmt.Errorf("decode file list: %w", err)
	}

	// Check Link header for pagination.
	nextURL := ""
	if link := resp.Header.Get("Link"); link != "" {
		// Simple parse: <url>; rel="next"
		if start := len("<"); start < len(link) {
			if end := indexOf(link, ">"); end > 0 {
				nextURL = link[1:end]
			}
		}
	}

	return files, nextURL, nil
}

func indexOf(s string, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// DownloadAll downloads all files in the repo and writes the done marker.
func (c *Client) DownloadAll(ctx context.Context) error {
	files, err := c.ListFiles(ctx)
	if err != nil {
		return fmt.Errorf("list repo files: %w", err)
	}

	// Filter to actual files only.
	var toDownload []RepoFile
	for _, f := range files {
		if f.Type == "file" {
			toDownload = append(toDownload, f)
		}
	}

	if len(toDownload) == 0 {
		return fmt.Errorf("no files found in %s (ref %s)", c.cfg.Repo, c.cfg.Ref)
	}

	for _, f := range toDownload {
		if err := c.downloadFile(ctx, f); err != nil {
			return fmt.Errorf("download %s: %w", f.Path, err)
		}
	}

	// Write done marker.
	donePath := filepath.Join(c.cfg.OutputDir, c.cfg.DoneName)
	if err := os.WriteFile(donePath, []byte("done\n"), 0644); err != nil {
		return fmt.Errorf("write done marker: %w", err)
	}

	c.progress.Complete()
	return nil
}

// downloadFile downloads a single file with retry logic.
func (c *Client) downloadFile(ctx context.Context, f RepoFile) error {
	destPath := filepath.Join(c.cfg.OutputDir, f.Path)

	// Skip if already exists with matching size.
	if info, err := os.Stat(destPath); err == nil && info.Size() == f.Size {
		c.progress.Downloading(f.Path, f.Size, f.Size)
		return nil
	}

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("create dir for %s: %w", f.Path, err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		lastErr = c.doDownload(ctx, f, destPath)
		if lastErr == nil {
			return nil
		}
		c.progress.Errorf(f.Path, fmt.Sprintf("attempt %d/%d: %v", attempt+1, maxRetries, lastErr))
	}

	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

func (c *Client) doDownload(ctx context.Context, f RepoFile, destPath string) error {
	url := fmt.Sprintf("%s/%s/resolve/%s/%s", c.cfg.Endpoint, c.cfg.Repo, c.cfg.Ref, f.Path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", f.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	total := resp.ContentLength
	if total <= 0 {
		total = f.Size
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	defer out.Close()

	var written int64
	buf := make([]byte, 256*1024) // 256KB buffer
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			nw, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("write %s: %w", destPath, writeErr)
			}
			written += int64(nw)

			// Report progress at most every 256KB to avoid flooding.
			c.progress.Downloading(f.Path, written, total)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read %s: %w", f.Path, readErr)
		}
	}

	return nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
}
