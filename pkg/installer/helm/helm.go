package helm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/packalares/packalares/pkg/installer/binaries"
)

func Install(baseDir, arch string) error {
	// Check if helm already available
	if _, err := exec.LookPath("helm"); err == nil {
		fmt.Println("  Helm already installed")
		return nil
	}

	fmt.Println("  Downloading Helm ...")

	dlDir := filepath.Join(baseDir, "downloads")
	os.MkdirAll(dlDir, 0755)

	url := fmt.Sprintf("https://get.helm.sh/helm-%s-linux-%s.tar.gz",
		binaries.HelmVersion, arch)
	tarPath := filepath.Join(dlDir, "helm.tar.gz")

	// Download using curl
	cmd := exec.Command("curl", "-fsSL", "-o", tarPath, url)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("download helm: %s\n%w", string(out), err)
	}

	// Extract
	tmpDir := filepath.Join(dlDir, "helm-extract")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	cmd = exec.Command("tar", "-xzf", tarPath, "-C", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract helm: %s\n%w", string(out), err)
	}

	// Copy binary
	src := filepath.Join(tmpDir, fmt.Sprintf("linux-%s", arch), "helm")
	dst := "/usr/local/bin/helm"

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read helm binary: %w", err)
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		return fmt.Errorf("write helm binary: %w", err)
	}

	fmt.Println("  Helm installed")
	return nil
}
