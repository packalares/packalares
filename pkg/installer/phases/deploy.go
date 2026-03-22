package phases

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/config"
)

func kubectlApply(yamlContent string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yamlContent)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply: %s\n%w", string(out), err)
	}
	return nil
}

func helmInstallOrUpgrade(name, chart, namespace string, values map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	args := []string{
		"upgrade", "--install", name, chart,
		"--namespace", namespace,
		"--create-namespace",
		"--wait",
		"--timeout", "10m",
	}

	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install %s: %s\n%w", name, string(out), err)
	}
	return nil
}

// applyManifestFile reads a YAML file, replaces {{PLACEHOLDERS}} with
// values from config, and applies it via kubectl.
func applyManifestFile(path string, opts *InstallOptions) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	manifest := replaceConfigPlaceholders(string(data), opts)
	return kubectlApply(manifest)
}

// replaceConfigPlaceholders replaces all {{VARIABLE}} placeholders in a
// manifest string with values from the centralized config.
func replaceConfigPlaceholders(manifest string, opts *InstallOptions) string {
	replacements := map[string]string{
		"{{PLATFORM_NAMESPACE}}":  config.PlatformNamespace(),
		"{{FRAMEWORK_NAMESPACE}}": config.FrameworkNamespace(),
		"{{USER_NAMESPACE}}":      config.UserNamespace(opts.Username),
		"{{USERNAME}}":            config.Username(),
		"{{USER_ZONE}}":           config.UserZone(),
		"{{DOMAIN}}":              config.Domain(),
		"{{PG_USER}}":             config.CitusUser(),
		"{{PG_PASSWORD}}":         config.CitusPassword(),
		"{{REDIS_PASSWORD}}":      os.Getenv("REDIS_PASSWORD"),
		"{{JWT_SECRET}}":          os.Getenv("JWT_SECRET"),
		"{{SESSION_SECRET}}":      os.Getenv("SESSION_SECRET"),
		"{{LLDAP_ADMIN_PASSWORD}}": os.Getenv("LLDAP_ADMIN_PASSWORD"),
		"{{LLDAP_JWT_SECRET}}":    os.Getenv("LLDAP_JWT_SECRET"),
		"{{ENCRYPTION_KEY}}":      os.Getenv("ENCRYPTION_KEY"),
		"{{AUTH_SECRET}}":         os.Getenv("AUTH_SECRET"),
	}

	for placeholder, value := range replacements {
		manifest = strings.ReplaceAll(manifest, placeholder, value)
	}
	return manifest
}

func deployCRDsAndNamespaces(opts *InstallOptions) error {
	fmt.Println("  Creating namespaces, CRDs, and RBAC ...")

	if err := applyManifestFile("deploy/crds/crds-and-namespaces.yaml", opts); err != nil {
		return fmt.Errorf("apply CRDs: %w", err)
	}

	// Create user namespace
	userNS := config.UserNamespace(opts.Username)
	nsYaml := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", userNS)
	if err := kubectlApply(nsYaml); err != nil {
		fmt.Printf("  Warning: create namespace %s: %v\n", userNS, err)
	}

	fmt.Printf("  Namespaces: %s, %s, %s, %s\n",
		config.PlatformNamespace(), config.FrameworkNamespace(), "monitoring", userNS)
	return nil
}

func deployPlatformCharts(opts *InstallOptions) error {
	manifests := []string{
		"deploy/platform/citus.yaml",
		"deploy/platform/nats.yaml",
		"deploy/platform/lldap.yaml",
		"deploy/platform/infisical.yaml",
	}
	// KVRocks is deployed separately in Phase 9 (pkg/installer/redis)

	for _, path := range manifests {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "deploy/platform/"), ".yaml")
		fmt.Printf("  Deploying %s ...\n", name)
		if err := applyManifestFile(path, opts); err != nil {
			return fmt.Errorf("deploy %s: %w", name, err)
		}
	}
	return nil
}

func deployFrameworkCharts(opts *InstallOptions) error {
	manifests := []string{
		"deploy/framework/auth.yaml",
		"deploy/framework/bfl-deployment.yaml",
		"deploy/framework/appservice/deployment.yaml",
		"deploy/framework/systemserver.yaml",
		"deploy/framework/middleware.yaml",
		"deploy/framework/files/files-server.yaml",
		"deploy/framework/market/deployment.yaml",
		"deploy/framework/monitor/monitoring-server.yaml",
		"deploy/framework/mounts/mounts-server.yaml",
		"deploy/framework/kubesphere.yaml",
		"deploy/framework/samba/samba-server.yaml",
		"deploy/framework/tailscale.yaml",
		"deploy/framework/l4proxy-deployment.yaml",
		"deploy/proxy/proxy-deployment.yaml",
	}

	for _, path := range manifests {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "deploy/framework/"), ".yaml")
		fmt.Printf("  Deploying %s ...\n", name)
		if err := applyManifestFile(path, opts); err != nil {
			return fmt.Errorf("deploy %s: %w", name, err)
		}
	}
	return nil
}

func deployAppCharts(opts *InstallOptions) error {
	components := []struct {
		name      string
		namespace string
	}{
		{"desktop", config.UserNamespace(opts.Username)},
		{"wizard", config.UserNamespace(opts.Username)},
	}

	// Ensure user namespace exists
	userNS := config.UserNamespace(opts.Username)
	ensureNamespace(userNS)
	ensureNamespace("user-system")

	for _, comp := range components {
		fmt.Printf("  Deploying %s ...\n", comp.name)
		manifest := generateAppManifest(comp.name, comp.namespace, opts)
		if err := kubectlApply(manifest); err != nil {
			return fmt.Errorf("deploy %s: %w", comp.name, err)
		}
	}
	return nil
}

func deployMonitoring(opts *InstallOptions) error {
	fmt.Println("  Deploying Prometheus + node-exporter + kube-state-metrics ...")
	manifest := generateMonitoringManifest(opts.Registry)
	return kubectlApply(manifest)
}

func deployKubeBlocks(opts *InstallOptions) error {
	fmt.Println("  Deploying KubeBlocks ...")
	manifest := generateKubeBlocksManifest(opts.Registry)
	return kubectlApply(manifest)
}

func waitForAllPods() error {
	fmt.Println("  Waiting for all pods to be ready ...")

	namespaces := []string{config.PlatformNamespace(), "kube-system", "user-system"}

	maxRetries := 60
	for i := 0; i < maxRetries; i++ {
		allReady := true
		for _, ns := range namespaces {
			if !arePodsReady(ns) {
				allReady = false
				break
			}
		}
		if allReady {
			return nil
		}
		if i%10 == 0 {
			fmt.Printf("  Still waiting for pods (attempt %d/%d) ...\n", i+1, maxRetries)
		}
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timed out waiting for pods after %d attempts", maxRetries)
}

func arePodsReady(namespace string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "kubectl", "get", "pods",
		"-n", namespace,
		"--no-headers",
		"-o", "custom-columns=STATUS:.status.phase",
	).CombinedOutput()
	if err != nil {
		return false
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return true // no pods = ok
	}

	for _, line := range lines {
		status := strings.TrimSpace(line)
		if status != "Running" && status != "Succeeded" {
			return false
		}
	}
	return true
}

func ensureNamespace(name string) {
	exec.Command("kubectl", "create", "namespace", name, "--dry-run=client", "-o", "yaml").
		CombinedOutput()
	exec.Command("kubectl", "create", "namespace", name).CombinedOutput()
}
