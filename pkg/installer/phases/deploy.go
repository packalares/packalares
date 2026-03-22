package phases

import (
	"context"
	"fmt"
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

func deployCRDsAndNamespaces(opts *InstallOptions) error {
	// Apply the CRDs + namespaces + RBAC manifest
	// This must run before any services so CRDs exist for controllers to watch
	fmt.Println("  Creating namespaces, CRDs, and RBAC ...")

	// Also create user namespace
	userNS := config.UserNamespace(opts.Username)

	manifests := []string{
		"deploy/crds/crds-and-namespaces.yaml",
	}

	for _, path := range manifests {
		cmd := exec.CommandContext(context.Background(), "kubectl", "apply", "-f", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("apply %s: %s\n%w", path, string(out), err)
		}
	}

	// Create user namespace
	nsYaml := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", userNS)
	if err := kubectlApply(nsYaml); err != nil {
		fmt.Printf("  Warning: create namespace %s: %v\n", userNS, err)
	}

	fmt.Printf("  Namespaces: %s, %s, %s, %s\n",
		config.PlatformNamespace(), config.FrameworkNamespace(), "monitoring", userNS)
	fmt.Println("  CRDs: users.iam.kubesphere.io, applications.app.bytetrade.io, middlewarerequests.apr.bytetrade.io")
	return nil
}

func deployPlatformCharts(opts *InstallOptions) error {
	// Platform services: Citus (PostgreSQL), KVRocks, NATS, LLDAP, Infisical
	components := []struct {
		name      string
		namespace string
	}{
		{"citus", config.PlatformNamespace()},
		{"kvrocks", config.PlatformNamespace()},
		{"nats", config.PlatformNamespace()},
		{"lldap", config.PlatformNamespace()},
		{"infisical", config.PlatformNamespace()},
	}

	for _, comp := range components {
		fmt.Printf("  Deploying %s ...\n", comp.name)
		values := make(map[string]string)
		if opts.Registry != "" {
			values["global.imageRegistry"] = opts.Registry
		}
		manifest := generatePlatformManifest(comp.name, comp.namespace, opts.Registry)
		if err := kubectlApply(manifest); err != nil {
			return fmt.Errorf("deploy %s: %w", comp.name, err)
		}
	}
	return nil
}

func deployFrameworkCharts(opts *InstallOptions) error {
	components := []struct {
		name      string
		namespace string
	}{
		{"auth", config.FrameworkNamespace()},
		{"bfl", config.FrameworkNamespace()},
		{"appservice", config.FrameworkNamespace()},
		{"systemserver", config.FrameworkNamespace()},
		{"middleware", config.FrameworkNamespace()},
		{"files", config.FrameworkNamespace()},
		{"market", config.FrameworkNamespace()},
		{"monitor", config.FrameworkNamespace()},
		{"mounts", config.FrameworkNamespace()},
		{"kubesphere", config.FrameworkNamespace()},
		{"samba", config.FrameworkNamespace()},
	}

	for _, comp := range components {
		fmt.Printf("  Deploying %s ...\n", comp.name)
		manifest := generateFrameworkManifest(comp.name, comp.namespace, opts)
		if err := kubectlApply(manifest); err != nil {
			return fmt.Errorf("deploy %s: %w", comp.name, err)
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
