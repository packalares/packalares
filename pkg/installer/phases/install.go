package phases

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/packalares/packalares/pkg/installer/binaries"
	"github.com/packalares/packalares/pkg/installer/cni"
	"github.com/packalares/packalares/pkg/installer/etcd"
	"github.com/packalares/packalares/pkg/installer/helm"
	"github.com/packalares/packalares/pkg/installer/k3s"
	"github.com/packalares/packalares/pkg/installer/kernel"
	"github.com/packalares/packalares/pkg/installer/precheck"
	"github.com/packalares/packalares/pkg/installer/redis"
	"github.com/packalares/packalares/pkg/installer/storage"
)

type phase struct {
	Name string
	Fn   func() error
}

func RunInstall(opts *InstallOptions) error {
	opts.applyDefaults()
	if err := opts.validate(); err != nil {
		return err
	}

	// Ensure base directories exist
	for _, dir := range []string{
		opts.BaseDir,
		filepath.Join(opts.BaseDir, "installer"),
		filepath.Join(opts.BaseDir, "installer", "wizard"),
		"/etc/packalares",
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Resolve username/password
	if opts.Username == "" {
		opts.Username = "admin"
	}
	if opts.Password == "" {
		plain, _, err := generatePassword(12)
		if err != nil {
			return fmt.Errorf("generate password: %w", err)
		}
		opts.Password = plain
		fmt.Printf("[install] Generated admin password: %s\n", opts.Password)
	}

	// Write config.yaml so all config.*() functions return correct values
	if err := writeConfigYAML(opts); err != nil {
		return fmt.Errorf("write config.yaml: %w", err)
	}

	arch := getArch()

	phases := []phase{
		// Phase 1: Precheck
		{"Precheck", func() error {
			if opts.SkipPrecheck {
				fmt.Println("[install] Skipping precheck (--skip-precheck)")
				return nil
			}
			result := precheck.RunPrecheck()
			precheck.PrintReport(result)
			if !result.Passed {
				return fmt.Errorf("precheck failed")
			}
			return nil
		}},

		// Phase 2: Download binaries
		{"Download binaries", func() error {
			return binaries.DownloadAll(opts.BaseDir, arch)
		}},

		// Phase 3: Kernel modules and sysctl
		{"Configure kernel", func() error {
			if err := kernel.LoadModules(); err != nil {
				return err
			}
			return kernel.ApplySysctl()
		}},

		// Phase 4: Install etcd
		{"Install etcd", func() error {
			return etcd.Install(opts.BaseDir)
		}},

		// Phase 6: Install K3s
		{"Install K3s", func() error {
			return k3s.Install(opts.BaseDir, opts.Registry)
		}},

		// Phase 7: Deploy Calico CNI
		{"Deploy Calico CNI", func() error {
			return cni.DeployCalico(opts.Registry)
		}},

		// Phase 8: Deploy OpenEBS storage
		{"Deploy OpenEBS", func() error {
			return storage.DeployOpenEBS(opts.Registry)
		}},

		// Phase 9: Register CRDs, create namespaces, RBAC
		{"Setup Kubernetes management", func() error {
			return deployCRDsAndNamespaces(opts)
		}},

		// Phase 10: Generate system secrets (before deploying anything that needs them)
		{"Generate secrets", func() error {
			return GenerateSecrets(opts)
		}},

		// Phase 11: Deploy KVRocks (uses REDIS_PASSWORD from GenerateSecrets)
		{"Deploy KVRocks", func() error {
			return redis.Install(opts.BaseDir)
		}},

		// Phase 12: Install Helm
		{"Install Helm", func() error {
			return helm.Install(opts.BaseDir, arch)
		}},

		// Phase 13: Deploy platform charts (Citus, NATS, LLDAP, Infisical)
		{"Deploy platform services", func() error {
			return deployPlatformCharts(opts)
		}},

		// Phase 14: Deploy framework charts
		{"Deploy framework services", func() error {
			return deployFrameworkCharts(opts)
		}},

		// Phase 15: Generate TLS certificate (before proxy needs it)
		{"Generate TLS certificate", func() error {
			return generateTLSCert(opts)
		}},

		// Phase 16: Seed Infisical (pod has init containers that wait for PG+Redis)
		{"Seed Infisical", func() error {
			return SeedInfisical(opts)
		}},

		// Phase 13: Deploy user apps
		{"Deploy user apps", func() error {
			return deployAppCharts(opts)
		}},

		// Phase 15: Deploy monitoring
		{"Deploy monitoring", func() error {
			return deployMonitoring(opts)
		}},

		// Phase 16: GPU setup (if detected)
		{"Setup GPU", func() error {
			return InstallGPU(opts)
		}},

		// Phase 16: Wait for pods
		{"Wait for pods", func() error {
			return waitForAllPods()
		}},

		// Phase 17: Write release file
		{"Write release info", func() error {
			return writeReleaseFile(opts)
		}},
	}

	total := len(phases)
	for i, p := range phases {
		fmt.Printf("\n[%d/%d] %s ...\n", i+1, total, p.Name)
		start := time.Now()
		if err := p.Fn(); err != nil {
			return fmt.Errorf("phase %q failed: %w", p.Name, err)
		}
		fmt.Printf("[%d/%d] %s completed in %s\n", i+1, total, p.Name, time.Since(start).Round(time.Second))
	}

	return nil
}

func writeReleaseFile(opts *InstallOptions) error {
	content := fmt.Sprintf(
		"PACKALARES_VERSION=1.0.0\nPACKALARES_BASE_DIR=%s\nPACKALARES_NAME=%s@%s\n",
		opts.BaseDir, opts.Username, opts.Domain,
	)
	return os.WriteFile(ReleaseFile, []byte(content), 0644)
}
