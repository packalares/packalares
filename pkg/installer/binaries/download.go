package binaries

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	K3sVersion        = "v1.29.3+k3s1"

	EtcdVersion       = "v3.5.12"
	HelmVersion       = "v3.14.3"

	CalicoVersion     = "v3.27.2"
	CrictlVersion     = "v1.29.0"
)

type binary struct {
	Name    string
	URL     func(arch string) string
	Extract func(baseDir, arch, downloadPath string) error
}

// DownloadAll downloads and installs all required binaries.
// It writes progress messages to w.
func DownloadAll(baseDir, arch string, w io.Writer) error {
	dlDir := filepath.Join(baseDir, "downloads")
	if err := os.MkdirAll(dlDir, 0755); err != nil {
		return fmt.Errorf("create download dir: %w", err)
	}

	bins := []binary{
		{
			Name: "k3s",
			URL: func(arch string) string {
				ver := strings.ReplaceAll(K3sVersion, "+", "%2B")
				suffix := ""
				if arch == "arm64" {
					suffix = "-arm64"
				}
				return fmt.Sprintf("https://github.com/k3s-io/k3s/releases/download/%s/k3s%s", ver, suffix)
			},
			Extract: func(baseDir, arch, downloadPath string) error {
				dest := "/usr/local/bin/k3s"
				return copyAndChmod(downloadPath, dest, 0755)
			},
		},
		{
			Name: "etcd",
			URL: func(arch string) string {
				return fmt.Sprintf("https://github.com/etcd-io/etcd/releases/download/%s/etcd-%s-linux-%s.tar.gz",
					EtcdVersion, EtcdVersion, arch)
			},
			Extract: func(baseDir, arch, downloadPath string) error {
				tmpDir := filepath.Join(baseDir, "downloads", "etcd-extract")
				os.MkdirAll(tmpDir, 0755)
				defer os.RemoveAll(tmpDir)

				if err := extractTarGz(downloadPath, tmpDir); err != nil {
					return err
				}

				dir := fmt.Sprintf("etcd-%s-linux-%s", EtcdVersion, arch)
				for _, bin := range []string{"etcd", "etcdctl", "etcdutl"} {
					src := filepath.Join(tmpDir, dir, bin)
					dst := filepath.Join("/usr/local/bin", bin)
					if err := copyAndChmod(src, dst, 0755); err != nil {
						return fmt.Errorf("install %s: %w", bin, err)
					}
				}
				return nil
			},
		},
		{
			Name: "crictl",
			URL: func(arch string) string {
				return fmt.Sprintf("https://github.com/kubernetes-sigs/cri-tools/releases/download/%s/crictl-%s-linux-%s.tar.gz",
					CrictlVersion, CrictlVersion, arch)
			},
			Extract: func(baseDir, arch, downloadPath string) error {
				return extractTarGz(downloadPath, "/usr/local/bin")
			},
		},
	}

	for _, b := range bins {
		fmt.Fprintf(w, "  Downloading %s ...\n", b.Name)
		url := b.URL(arch)
		dlPath := filepath.Join(dlDir, b.Name+filepath.Ext(url))
		if filepath.Ext(url) == "" || strings.Contains(url, "%2B") {
			dlPath = filepath.Join(dlDir, b.Name)
			if strings.HasSuffix(url, ".tar.gz") {
				dlPath += ".tar.gz"
			}
		}
		// Re-derive extension properly
		if strings.HasSuffix(url, ".tar.gz") {
			dlPath = filepath.Join(dlDir, b.Name+".tar.gz")
		} else {
			dlPath = filepath.Join(dlDir, b.Name)
		}

		if err := downloadFile(url, dlPath); err != nil {
			return fmt.Errorf("download %s: %w", b.Name, err)
		}

		fmt.Fprintf(w, "  Installing %s ...\n", b.Name)
		if err := b.Extract(baseDir, arch, dlPath); err != nil {
			return fmt.Errorf("extract %s: %w", b.Name, err)
		}
	}

	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{
		Timeout: 30 * time.Minute,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}

	return nil
}

func copyAndChmod(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return os.Chmod(dst, mode)
}

func extractTarGz(tarFile, destDir string) error {
	cmd := exec.Command("tar", "-xzf", tarFile, "-C", destDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extract: %s\n%w", string(out), err)
	}
	return nil
}
