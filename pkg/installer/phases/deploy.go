package phases

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/packalares/packalares/deploy"
	"github.com/packalares/packalares/pkg/config"
	nginxbuilder "github.com/packalares/packalares/pkg/nginx"
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

// applyManifestFile reads a YAML file from the embedded deploy/ filesystem,
// replaces {{PLACEHOLDERS}} with values from config, and applies via kubectl.
func applyManifestFile(path string, opts *InstallOptions) error {
	// Strip "deploy/" prefix since embed.FS is rooted at deploy/
	embedPath := strings.TrimPrefix(path, "deploy/")
	data, err := deploy.Manifests.ReadFile(embedPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	manifest := replaceConfigPlaceholders(string(data), opts)
	return kubectlApply(manifest)
}

// replaceConfigPlaceholders replaces all {{VARIABLE}} placeholders in a
// manifest string with values from the centralized config.
func replaceConfigPlaceholders(manifest string, opts *InstallOptions) string {
	// Generate hostctl token (base64-encoded) for the K8s Secret
	hostctlToken := os.Getenv("HOSTCTL_TOKEN")
	hostctlTokenB64 := ""
	if hostctlToken != "" {
		hostctlTokenB64 = base64.StdEncoding.EncodeToString([]byte(hostctlToken))
	}

	replacements := map[string]string{
		"{{API_GROUP}}":            config.APIGroup(),
		"{{TLS_SECRET_NAME}}":     config.TLSSecretName(),
		"{{PLATFORM_NAMESPACE}}":  config.PlatformNamespace(),
		"{{FRAMEWORK_NAMESPACE}}": config.FrameworkNamespace(),
		"{{MONITORING_NAMESPACE}}": config.MonitoringNamespace(),
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
		"{{SAMBA_PASSWORD}}":      os.Getenv("SAMBA_PASSWORD"),
		"{{SERVER_IP}}":           os.Getenv("SERVER_IP"),
		"{{COREDNS_CLUSTER_IP}}":  os.Getenv("COREDNS_CLUSTER_IP"),
		"{{HOSTCTL_TOKEN_B64}}":   hostctlTokenB64,
		"{{TAPR_AUTH_TOKEN}}":     os.Getenv("TAPR_AUTH_TOKEN"),
	}

	for placeholder, value := range replacements {
		manifest = strings.ReplaceAll(manifest, placeholder, value)
	}
	return manifest
}

// generateHostctlToken generates a random 32-byte hex token for hostctl auth.
func generateHostctlToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen
		return "fallback-token-replace-me"
	}
	return hex.EncodeToString(b)
}

func deployCRDsAndNamespaces(opts *InstallOptions, w io.Writer) error {
	fmt.Fprintln(w, "  Creating namespaces, CRDs, and RBAC ...")

	if err := applyManifestFile("deploy/crds/crds-and-namespaces.yaml", opts); err != nil {
		return fmt.Errorf("apply CRDs: %w", err)
	}

	// Create user namespace
	userNS := config.UserNamespace(opts.Username)
	nsYaml := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", userNS)
	if err := kubectlApply(nsYaml); err != nil {
		fmt.Fprintf(w, "  Warning: create namespace %s: %v\n", userNS, err)
	}

	// Create User CRD for admin user
	userCRD := fmt.Sprintf(`apiVersion: iam.kubesphere.io/v1alpha2
kind: User
metadata:
  name: %s
  annotations:
    %s/owner-role: platform-admin
    %s/zone: %s
spec:
  displayName: %s
  email: %s@%s
  lang: en
  groups:
  - admins
`, opts.Username, config.APIGroup(), config.APIGroup(), config.UserZone(),
		opts.Username, opts.Username, config.Domain())
	if err := kubectlApply(userCRD); err != nil {
		fmt.Fprintf(w, "  Warning: create User CRD: %v\n", err)
	}

	fmt.Fprintf(w, "  Namespaces: %s, %s, %s, %s\n",
		config.PlatformNamespace(), config.FrameworkNamespace(), "monitoring", userNS)
	return nil
}

func deployPlatformCharts(opts *InstallOptions, w io.Writer) error {
	manifests := []string{
		"deploy/platform/citus.yaml",
		"deploy/platform/nats.yaml",
		"deploy/platform/lldap.yaml",
		"deploy/platform/infisical.yaml",
	}
	// KVRocks is deployed separately in Phase 9 (pkg/installer/redis)

	for _, path := range manifests {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "deploy/platform/"), ".yaml")
		fmt.Fprintf(w, "  Deploying %s ...\n", name)
		if err := applyManifestFile(path, opts); err != nil {
			return fmt.Errorf("deploy %s: %w", name, err)
		}
	}
	return nil
}

// deployProxyConfig creates proxy-config-template and proxy-config ConfigMaps,
// then applies the proxy DaemonSet.
func deployProxyConfig(opts *InstallOptions, w io.Writer) error {
	ns := config.FrameworkNamespace()

	// Read template files from embed
	templateFiles := map[string]string{
		"nginx.conf": "proxy/nginx.conf.tmpl",
	}
	includeFiles := []string{
		"proxy/includes/upstreams.conf",
		"proxy/includes/auth-subrequest.conf",
		"proxy/includes/public-api.conf",
		"proxy/includes/protected-api.conf",
		"proxy/includes/websocket.conf",
		"proxy/includes/static-assets.conf",
		"proxy/includes/cors.conf",
	}

	// Build template data (raw templates with placeholders)
	tmplData := make(map[string]string)
	for key, path := range templateFiles {
		data, err := deploy.Manifests.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		tmplData[key] = string(data)
	}
	for _, path := range includeFiles {
		data, err := deploy.Manifests.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// Key is the filename without path prefix
		key := strings.TrimPrefix(path, "proxy/includes/")
		tmplData["includes/"+key] = string(data)
	}

	// Create proxy-config-template ConfigMap (raw templates)
	fmt.Fprintln(w, "  Creating proxy-config-template ...")
	tmplYAML := buildConfigMapYAML("proxy-config-template", ns, tmplData)
	if err := kubectlApply(tmplYAML); err != nil {
		return fmt.Errorf("apply proxy-config-template: %w", err)
	}

	// Build final config using nginx builder
	params := nginxbuilder.Params{
		Zone:        config.UserZone(),
		ServerIP:    os.Getenv("SERVER_IP"),
		FrameworkNS: config.FrameworkNamespace(),
		UserNS:      config.UserNamespace(opts.Username),
		Resolver:    getInstallerClusterDNS(),
	}

	finalData := make(map[string]string)
	for key, tmpl := range tmplData {
		finalData[key] = nginxbuilder.Build(tmpl, params)
	}

	// Create proxy-config ConfigMap (final, for nginx to use)
	fmt.Fprintln(w, "  Creating proxy-config ...")
	finalYAML := buildConfigMapYAML("proxy-config", ns, finalData)
	if err := kubectlApply(finalYAML); err != nil {
		return fmt.Errorf("apply proxy-config: %w", err)
	}

	// Apply the proxy DaemonSet
	fmt.Fprintln(w, "  Deploying proxy ...")
	return applyManifestFile("deploy/proxy/proxy-deployment.yaml", opts)
}

// buildConfigMapYAML generates a ConfigMap YAML string from a map of key→value.
func buildConfigMapYAML(name, namespace string, data map[string]string) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: v1\nkind: ConfigMap\nmetadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n  namespace: %s\n", name, namespace))
	sb.WriteString("data:\n")
	for key, value := range data {
		sb.WriteString(fmt.Sprintf("  %s: |\n", key))
		for _, line := range strings.Split(value, "\n") {
			sb.WriteString("    " + line + "\n")
		}
	}
	return sb.String()
}

func getInstallerClusterDNS() string {
	out, err := exec.Command("kubectl", "get", "svc", "-n", "kube-system", "kube-dns",
		"-o", "jsonpath={.spec.clusterIP}").Output()
	if err == nil && len(out) > 0 {
		return strings.TrimSpace(string(out))
	}
	return "10.233.0.10"
}

func deployFrameworkCharts(opts *InstallOptions, w io.Writer) error {
	// Generate hostctl token if not already set
	if os.Getenv("HOSTCTL_TOKEN") == "" {
		token := generateHostctlToken()
		os.Setenv("HOSTCTL_TOKEN", token)
		fmt.Fprintln(w, "  Generated hostctl authentication token")
	}

	manifests := []string{
		"deploy/framework/hostctl.yaml",
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
		"deploy/framework/mdns.yaml",
	}

	for _, path := range manifests {
		// Skip tailscale if no auth key configured
		if path == "deploy/framework/tailscale.yaml" && opts.TailscaleAuthKey == "" {
			fmt.Fprintln(w, "  Skipping tailscale (no auth key configured)")
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "deploy/framework/"), ".yaml")
		fmt.Fprintf(w, "  Deploying %s ...\n", name)
		if err := applyManifestFile(path, opts); err != nil {
			return fmt.Errorf("deploy %s: %w", name, err)
		}
	}

	// Deploy proxy (template ConfigMaps + DaemonSet)
	if err := deployProxyConfig(opts, w); err != nil {
		return fmt.Errorf("deploy proxy: %w", err)
	}

	return nil
}

func deployAppCharts(opts *InstallOptions, w io.Writer) error {
	// Ensure user namespace exists
	userNS := config.UserNamespace(opts.Username)
	ensureNamespace(userNS)

	manifests := []string{
		"deploy/apps/desktop.yaml",
		"deploy/apps/wizard.yaml",
	}

	for _, path := range manifests {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "deploy/apps/"), ".yaml")
		fmt.Fprintf(w, "  Deploying %s ...\n", name)
		if err := applyManifestFile(path, opts); err != nil {
			return fmt.Errorf("deploy %s: %w", name, err)
		}
	}
	return nil
}

func deployMonitoring(opts *InstallOptions, w io.Writer) error {
	manifests := []struct {
		name string
		path string
	}{
		{"Prometheus + node-exporter + kube-state-metrics", "deploy/framework/monitoring.yaml"},
		{"Loki (log aggregation)", "deploy/framework/loki.yaml"},
		{"Promtail (log collector)", "deploy/framework/promtail.yaml"},
	}

	for _, m := range manifests {
		fmt.Fprintf(w, "  Deploying %s ...\n", m.name)
		if err := applyManifestFile(m.path, opts); err != nil {
			return fmt.Errorf("deploy %s: %w", m.name, err)
		}
	}
	return nil
}

func deployKubeBlocks(opts *InstallOptions, w io.Writer) error {
	fmt.Fprintln(w, "  Deploying KubeBlocks ...")
	manifest := generateKubeBlocksManifest(opts.Registry)
	return kubectlApply(manifest)
}

func waitForAllPods(w io.Writer) error {
	fmt.Fprintln(w, "  Waiting for all pods to be ready ...")

	namespaces := []string{
		config.PlatformNamespace(),
		config.FrameworkNamespace(),
		config.MonitoringNamespace(),
		config.UserNamespace(config.Username()),
		"kube-system",
	}

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
			fmt.Fprintf(w, "  Still waiting for pods (attempt %d/%d) ...\n", i+1, maxRetries)
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
