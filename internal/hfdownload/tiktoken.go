package hfdownload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	tiktokenBaseURL = "https://openaipublic.blob.core.windows.net/encodings"
	defaultTikDir   = "/data/tiktoken"
)

// TiktokenConfig holds settings for tiktoken file downloads.
type TiktokenConfig struct {
	Files []string // file names, e.g. ["o200k_base.tiktoken"]
	Dir   string   // output directory
}

// ParseTiktokenFiles splits a comma-separated list of tiktoken file names.
func ParseTiktokenFiles(csv string) []string {
	if csv == "" {
		return nil
	}
	var files []string
	for _, f := range strings.Split(csv, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

// DownloadTiktoken downloads tiktoken encoding files.
func DownloadTiktoken(ctx context.Context, cfg TiktokenConfig, progress *ProgressReporter) error {
	if len(cfg.Files) == 0 {
		return nil
	}

	dir := cfg.Dir
	if dir == "" {
		dir = defaultTikDir
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create tiktoken dir: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}

	for _, name := range cfg.Files {
		destPath := filepath.Join(dir, name)

		// Skip if already exists.
		if _, err := os.Stat(destPath); err == nil {
			progress.Downloading(name, 1, 1)
			continue
		}

		url := tiktokenBaseURL + "/" + name
		if err := downloadWithRetry(ctx, client, url, destPath, name, progress); err != nil {
			return fmt.Errorf("tiktoken %s: %w", name, err)
		}
	}

	return nil
}

func downloadWithRetry(ctx context.Context, client *http.Client, url, destPath, name string, progress *ProgressReporter) error {
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

		lastErr = doSimpleDownload(ctx, client, url, destPath, name, progress)
		if lastErr == nil {
			return nil
		}
		progress.Errorf(name, fmt.Sprintf("attempt %d/%d: %v", attempt+1, maxRetries, lastErr))
	}
	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

func doSimpleDownload(ctx context.Context, client *http.Client, url, destPath, name string, progress *ProgressReporter) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer out.Close()

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 64*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			nw, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("write: %w", writeErr)
			}
			written += int64(nw)
			progress.Downloading(name, written, total)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read: %w", readErr)
		}
	}

	return nil
}
