package appservice

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

// imageRef is a regexp that matches container image references in YAML values.
// Matches strings like "docker.io/library/nginx:1.25" or "ghcr.io/org/repo@sha256:abc...".
var imageRef = regexp.MustCompile(`^[a-zA-Z0-9][-a-zA-Z0-9.]*(?:/[-a-zA-Z0-9._]+)+(?::[a-zA-Z0-9][-a-zA-Z0-9._]*|@sha256:[0-9a-f]{64})$`)

// ExtractImagesFromChart walks a chart directory and extracts all container
// image references from values.yaml, templates/, and subchart values.
// It returns a deduplicated list.
func ExtractImagesFromChart(chartDir string) []string {
	seen := make(map[string]bool)

	// 1. Parse values.yaml and subchart values for image fields
	for _, vf := range findValuesFiles(chartDir) {
		for _, img := range extractImagesFromValuesFile(vf) {
			seen[img] = true
		}
	}

	// 2. Scan templates/ for "image:" lines
	templatesDir := filepath.Join(chartDir, "templates")
	if entries, err := os.ReadDir(templatesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			for _, img := range extractImagesFromTemplate(filepath.Join(templatesDir, e.Name())) {
				seen[img] = true
			}
		}
	}

	// 3. Scan subchart templates
	chartsDir := filepath.Join(chartDir, "charts")
	if subEntries, err := os.ReadDir(chartsDir); err == nil {
		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}
			subTemplates := filepath.Join(chartsDir, sub.Name(), "templates")
			if tEntries, err := os.ReadDir(subTemplates); err == nil {
				for _, e := range tEntries {
					if !e.IsDir() {
						for _, img := range extractImagesFromTemplate(filepath.Join(subTemplates, e.Name())) {
							seen[img] = true
						}
					}
				}
			}
		}
	}

	images := make([]string, 0, len(seen))
	for img := range seen {
		images = append(images, img)
	}
	return images
}

// findValuesFiles returns all values.yaml files in the chart and subcharts.
func findValuesFiles(chartDir string) []string {
	var files []string
	top := filepath.Join(chartDir, "values.yaml")
	if _, err := os.Stat(top); err == nil {
		files = append(files, top)
	}
	chartsDir := filepath.Join(chartDir, "charts")
	if subs, err := os.ReadDir(chartsDir); err == nil {
		for _, s := range subs {
			if s.IsDir() {
				vf := filepath.Join(chartsDir, s.Name(), "values.yaml")
				if _, err := os.Stat(vf); err == nil {
					files = append(files, vf)
				}
			}
		}
	}
	return files
}

// extractImagesFromValuesFile parses a values.yaml and looks for image
// references in common Helm patterns:
//   image: "nginx:1.25"
//   image:
//     repository: nginx
//     tag: "1.25"
func extractImagesFromValuesFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var vals map[string]interface{}
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil
	}

	var images []string
	collectImages(vals, &images)
	return images
}

// collectImages recursively walks a YAML map looking for image references.
func collectImages(m map[string]interface{}, out *[]string) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			if (k == "image" || k == "baseImage") && looksLikeImage(val) {
				*out = append(*out, normalizeImage(val))
			}
		case map[string]interface{}:
			// Check for repository+tag pattern
			if k == "image" || strings.HasSuffix(k, "Image") {
				repo, _ := val["repository"].(string)
				tag, _ := val["tag"].(string)
				if repo != "" {
					if tag == "" {
						tag = "latest"
					}
					*out = append(*out, normalizeImage(repo+":"+tag))
				}
			}
			collectImages(val, out)
		}
	}
}

// extractImagesFromTemplate scans a template file for "image:" lines and
// extracts static image references (skipping Helm template expressions).
func extractImagesFromTemplate(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var images []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// Match lines like:  image: "docker.io/library/nginx:1.25"
		// or:                 - image: ghcr.io/org/app:v1
		for _, prefix := range []string{"image:", "- image:"} {
			if strings.HasPrefix(line, prefix) {
				img := strings.TrimPrefix(line, prefix)
				img = strings.TrimSpace(img)
				img = strings.Trim(img, "\"'")
				// Skip helm template expressions
				if strings.Contains(img, "{{") {
					continue
				}
				if looksLikeImage(img) {
					images = append(images, normalizeImage(img))
				}
			}
		}
	}
	return images
}

// looksLikeImage returns true if s looks like a container image reference.
func looksLikeImage(s string) bool {
	if s == "" || strings.Contains(s, "{{") {
		return false
	}
	// Must contain at least one slash (registry/repo or repo/image)
	// OR be a well-known short name (e.g. "nginx:1.25")
	if strings.Contains(s, "/") {
		return imageRef.MatchString(s)
	}
	// Short names like "nginx:1.25" or "busybox"
	parts := strings.SplitN(s, ":", 2)
	name := parts[0]
	return len(name) > 0 && !strings.Contains(name, " ") && !strings.Contains(name, "=")
}

// normalizeImage ensures an image has a tag or digest.
func normalizeImage(img string) string {
	if strings.Contains(img, "@sha256:") {
		return img
	}
	if !strings.Contains(img, ":") {
		return img + ":latest"
	}
	return img
}

// ImagePullProgress tracks pull progress for a set of images.
type ImagePullProgress struct {
	TotalImages int
	Pulled      int64 // atomic
	Errors      int64 // atomic
}

// PullImagesWithProgress pulls images using crictl (CRI-compatible) and
// reports progress via the provided callback. It pulls up to 3 images
// concurrently. The callback receives (pulled, total, currentImage,
// bytesDownloaded, bytesTotal). While each crictl pull blocks, a background
// goroutine polls containerd for active download bytes and calls onProgress
// with real byte counts.
// Returns the number of errors (non-fatal -- install continues even if
// some images fail to pre-pull since kubelet will retry).
func PullImagesWithProgress(ctx context.Context, images []string, onProgress func(pulled, total int, currentImage string, bytesDownloaded, bytesTotal int64)) int {
	if len(images) == 0 {
		return 0
	}

	total := len(images)
	var pulled int64
	var errors int64
	var wg sync.WaitGroup

	// Track aggregate download bytes across all concurrent pulls
	var liveBytes int64 // atomic: current bytes being downloaded

	// Determine the pull tool: prefer crictl, fall back to ctr
	pullTool := "crictl"
	if _, err := exec.LookPath("crictl"); err != nil {
		pullTool = "ctr"
	}

	// Find ctr binary for progress polling
	ctrBin := "ctr"
	if p, err := exec.LookPath("ctr"); err == nil {
		ctrBin = p
	}
	containerdSock := "/run/containerd/containerd.sock"

	// Semaphore for concurrency limit
	sem := make(chan struct{}, 3)

	// Start a background goroutine that polls containerd for active download progress
	pollCtx, pollCancel := context.WithCancel(ctx)
	defer pollCancel()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				bytes := pollActiveDownloads(pollCtx, ctrBin, containerdSock)
				atomic.StoreInt64(&liveBytes, bytes)
				// Report progress with live byte counts
				p := int(atomic.LoadInt64(&pulled))
				onProgress(p, total, "", bytes, 0)
			}
		}
	}()

	for _, img := range images {
		wg.Add(1)
		go func(image string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			onProgress(int(atomic.LoadInt64(&pulled)), total, image, atomic.LoadInt64(&liveBytes), 0)

			var cmd *exec.Cmd
			if pullTool == "crictl" {
				cmd = exec.CommandContext(ctx, "crictl", "pull", image)
			} else {
				cmd = exec.CommandContext(ctx, "ctr", "-n", "k8s.io", "images", "pull", image)
			}
			cmd.Env = os.Environ()

			klog.V(2).Infof("pulling image: %s", image)
			out, err := cmd.CombinedOutput()
			if err != nil {
				atomic.AddInt64(&errors, 1)
				klog.V(2).Infof("pull image %s: %v: %s", image, err, strings.TrimSpace(string(out)))
			} else {
				klog.V(2).Infof("pulled image: %s", image)
			}

			n := int(atomic.AddInt64(&pulled, 1))
			onProgress(n, total, image, atomic.LoadInt64(&liveBytes), 0)
		}(img)
	}

	wg.Wait()
	pollCancel()
	return int(atomic.LoadInt64(&errors))
}

// pollActiveDownloads runs `ctr content active` and sums the sizes of all
// active downloads. Returns total bytes currently downloaded across all
// active content fetches.
func pollActiveDownloads(ctx context.Context, ctrBin, sock string) int64 {
	cmd := exec.CommandContext(ctx, ctrBin, "-a", sock, "-n", "k8s.io", "content", "active")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	var totalBytes int64
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		// Skip header line
		if strings.HasPrefix(line, "REF") || strings.TrimSpace(line) == "" {
			continue
		}
		// Format: REF\tSIZE\tAGE  (tab-separated)
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		totalBytes += parseSizeString(fields[1])
	}
	return totalBytes
}

// parseSizeString converts a human-readable size like "345.7MB", "1.2GB",
// "500KB", "1024B" into bytes.
func parseSizeString(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Find where the numeric part ends and the unit begins
	unitStart := len(s)
	for i, c := range s {
		if c != '.' && (c < '0' || c > '9') {
			unitStart = i
			break
		}
	}

	numStr := s[:unitStart]
	unit := strings.ToUpper(s[unitStart:])

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}

	switch unit {
	case "B", "":
		return int64(val)
	case "KB", "KIB", "K":
		return int64(val * 1024)
	case "MB", "MIB", "M":
		return int64(val * 1024 * 1024)
	case "GB", "GIB", "G":
		return int64(val * 1024 * 1024 * 1024)
	case "TB", "TIB", "T":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(val)
	}
}

// CheckImageExists returns true if the image is already present locally.
func CheckImageExists(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "crictl", "inspecti", "-q", image)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

// FilterMissingImages returns only images that are not already present locally.
func FilterMissingImages(ctx context.Context, images []string) []string {
	var missing []string
	for _, img := range images {
		if !CheckImageExists(ctx, img) {
			missing = append(missing, img)
		}
	}
	return missing
}

// EstimateImageSizes returns a rough byte estimate for pulling images.
// This is a heuristic: ~100MB average per image. Actual sizes vary widely
// but this gives the UI something to show.
func EstimateImageSizes(images []string) int64 {
	// 100MB per image as rough estimate
	return int64(len(images)) * 100 * 1024 * 1024
}

// PullImagesForChart extracts images from a chart, filters out already-present
// ones, and pulls the missing images with progress reporting via WebSocket.
// Returns the number of errors encountered.
func PullImagesForChart(ctx context.Context, appName, chartDir string, stepOffset, totalSteps int) int {
	images := ExtractImagesFromChart(chartDir)
	if len(images) == 0 {
		klog.V(2).Infof("no images found in chart %s, skipping pre-pull", appName)
		GetWSHub().BroadcastInstallProgress(appName, StateInstalling, stepOffset, totalSteps, "No images to pre-pull", 0, 0)
		return 0
	}

	klog.Infof("chart %s: found %d image(s), checking which need pulling...", appName, len(images))

	missing := FilterMissingImages(ctx, images)
	if len(missing) == 0 {
		klog.Infof("chart %s: all %d images already present", appName, len(images))
		GetWSHub().BroadcastInstallProgress(appName, StateInstalling, stepOffset, totalSteps, "All images cached", 0, 0)
		return 0
	}

	klog.Infof("chart %s: pulling %d/%d missing images", appName, len(missing), len(images))
	estimatedTotal := EstimateImageSizes(missing)

	errs := PullImagesWithProgress(ctx, missing, func(pulled, total int, currentImage string, bytesDownloaded, bytesTotal int64) {
		// Use live byte counts when available, fall back to step-based estimate
		dlBytes := bytesDownloaded
		totalBytes := bytesTotal
		if totalBytes <= 0 {
			totalBytes = estimatedTotal
		}
		if dlBytes <= 0 && total > 0 {
			dlBytes = totalBytes * int64(pulled) / int64(total)
		}

		// Shorten the image name for display
		short := currentImage
		if idx := strings.LastIndex(short, "/"); idx >= 0 {
			short = short[idx+1:]
		}

		detail := fmt.Sprintf("Pulling images (%d/%d) %s", pulled, total, short)
		GetWSHub().BroadcastInstallProgress(appName, StateInstalling, stepOffset, totalSteps, detail, dlBytes, totalBytes)
	})

	if errs > 0 {
		klog.Infof("chart %s: %d image pull errors (non-fatal, kubelet will retry)", appName, errs)
	}
	return errs
}

// ExtractImagesFromTemplateOutput runs helm template on a chart directory
// and extracts image references from the rendered output. This catches
// images that are constructed via Helm template logic. Falls back to
// static extraction if helm template fails.
func ExtractImagesFromTemplateOutput(ctx context.Context, chartDir, namespace string) []string {
	cmd := exec.CommandContext(ctx, "helm", "template", "extract-images", chartDir,
		"--namespace", namespace)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		klog.V(2).Infof("helm template for image extraction failed: %v, using static extraction", err)
		return nil
	}

	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		for _, prefix := range []string{"image:", "- image:"} {
			if strings.HasPrefix(line, prefix) {
				img := strings.TrimPrefix(line, prefix)
				img = strings.TrimSpace(img)
				img = strings.Trim(img, "\"'")
				if img != "" && !strings.Contains(img, "{{") && !seen[img] {
					seen[img] = true
				}
			}
		}
	}

	images := make([]string, 0, len(seen))
	for img := range seen {
		images = append(images, img)
	}
	return images
}
