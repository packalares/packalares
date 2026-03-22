package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// K8sClient provides a simplified Kubernetes interface that shells out to kubectl.
// In production this would use client-go, but for Packalares we keep it simple
// and avoid the heavy CRD code-generation that Olares requires.
type K8sClient struct{}

// NewK8sClient creates a new k8s client.
func NewK8sClient() *K8sClient {
	return &K8sClient{}
}

// GetPodsForApp returns pod info for pods matching an app's release label.
func (k *K8sClient) GetPodsForApp(ctx context.Context, releaseName, namespace string) []PodInfo {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "pods",
		"--namespace", namespace,
		"-l", "app.kubernetes.io/instance="+releaseName,
		"-o", "jsonpath={range .items[*]}{.metadata.name}|{.status.phase}|{.metadata.creationTimestamp}{\"\\n\"}{end}",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var pods []PodInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		created, _ := time.Parse(time.RFC3339, parts[2])
		age := time.Since(created).Truncate(time.Second).String()

		pods = append(pods, PodInfo{
			Name:   parts[0],
			Status: parts[1],
			Age:    age,
		})
	}

	return pods
}

// ScaleDeployment scales deployments in a namespace for an app.
func (k *K8sClient) ScaleDeployment(ctx context.Context, namespace, labelSelector string, replicas int) error {
	// Find deployments matching the label
	cmd := exec.CommandContext(ctx, "kubectl", "get", "deployments",
		"--namespace", namespace,
		"-l", labelSelector,
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}",
	)

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("get deployments: %w", err)
	}

	names := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		scaleCmd := exec.CommandContext(ctx, "kubectl", "scale", "deployment", name,
			"--namespace", namespace,
			"--replicas", fmt.Sprintf("%d", replicas),
		)
		if scaleOut, err := scaleCmd.CombinedOutput(); err != nil {
			klog.Warningf("scale deployment %s: %s: %v", name, string(scaleOut), err)
		}
	}

	return nil
}

// ScaleStatefulSet scales statefulsets in a namespace for an app.
func (k *K8sClient) ScaleStatefulSet(ctx context.Context, namespace, labelSelector string, replicas int) error {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "statefulsets",
		"--namespace", namespace,
		"-l", labelSelector,
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}",
	)

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("get statefulsets: %w", err)
	}

	names := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		scaleCmd := exec.CommandContext(ctx, "kubectl", "scale", "statefulset", name,
			"--namespace", namespace,
			"--replicas", fmt.Sprintf("%d", replicas),
		)
		if scaleOut, err := scaleCmd.CombinedOutput(); err != nil {
			klog.Warningf("scale statefulset %s: %s: %v", name, string(scaleOut), err)
		}
	}

	return nil
}

// ApplyManifest applies a YAML manifest via kubectl.
func (k *K8sClient) ApplyManifest(ctx context.Context, manifest string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply: %s: %w", string(out), err)
	}
	return nil
}

// DeleteManifest deletes resources described by a YAML manifest.
func (k *K8sClient) DeleteManifest(ctx context.Context, manifest string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "-f", "-", "--ignore-not-found")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl delete: %s: %w", string(out), err)
	}
	return nil
}

// CreateNamespace creates a namespace if it does not exist.
func (k *K8sClient) CreateNamespace(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "create", "namespace", name, "--dry-run=client", "-o", "yaml")
	yaml, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("generate namespace yaml: %w", err)
	}
	return k.ApplyManifest(ctx, string(yaml))
}

// GetNamespaces returns all namespace names.
func (k *K8sClient) GetNamespaces(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "namespaces",
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var ns []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ns = append(ns, line)
		}
	}
	return ns, nil
}

// ApplicationCRDManifest generates an Application CRD manifest compatible with Olares.
// Entrances are stored as a JSON string in an annotation (which is the Olares convention)
// and as proper YAML list items in the spec.
func ApplicationCRDManifest(rec *AppRecord) string {
	entrancesJSON, err := json.Marshal(rec.Entrances)
	if err != nil {
		klog.Errorf("marshal entrances for %s: %v", rec.Name, err)
		entrancesJSON = []byte("[]")
	}

	// Build entrances as proper YAML list items for the spec section
	var entrancesYAML string
	if len(rec.Entrances) == 0 {
		entrancesYAML = "  entrances: []"
	} else {
		var lines []string
		lines = append(lines, "  entrances:")
		for _, e := range rec.Entrances {
			lines = append(lines, fmt.Sprintf("  - name: %s", yamlQuote(e.Name)))
			lines = append(lines, fmt.Sprintf("    host: %s", yamlQuote(e.Host)))
			lines = append(lines, fmt.Sprintf("    port: %d", e.Port))
			if e.Title != "" {
				lines = append(lines, fmt.Sprintf("    title: %s", yamlQuote(e.Title)))
			}
			if e.Icon != "" {
				lines = append(lines, fmt.Sprintf("    icon: %s", yamlQuote(e.Icon)))
			}
			if e.AuthLevel != "" {
				lines = append(lines, fmt.Sprintf("    authLevel: %s", yamlQuote(e.AuthLevel)))
			}
			if e.Invisible {
				lines = append(lines, "    invisible: true")
			}
			if e.OpenMethod != "" {
				lines = append(lines, fmt.Sprintf("    openMethod: %s", yamlQuote(e.OpenMethod)))
			}
		}
		entrancesYAML = strings.Join(lines, "\n")
	}

	return fmt.Sprintf(`apiVersion: app.bytetrade.io/v1alpha1
kind: Application
metadata:
  name: %s
  namespace: %s
  labels:
    applications.app.bytetrade.io/name: %s
    applications.app.bytetrade.io/owner: %s
  annotations:
    applications.app.bytetrade.io/entrances: %s
    applications.app.bytetrade.io/icon: %s
    applications.app.bytetrade.io/title: %s
    applications.app.bytetrade.io/version: %s
    applications.app.bytetrade.io/source: %s
spec:
  name: %s
  appid: %s
  namespace: %s
  owner: %s
  isSysApp: %v
  icon: %s
  description: %s
%s
status:
  state: %s
`,
		rec.ReleaseName,
		rec.Namespace,
		rec.Name,
		rec.Owner,
		yamlQuote(string(entrancesJSON)),
		yamlQuote(rec.Icon),
		yamlQuote(rec.Title),
		yamlQuote(rec.Version),
		yamlQuote(rec.Source),
		rec.Name,
		rec.AppID,
		rec.Namespace,
		rec.Owner,
		rec.IsSysApp,
		yamlQuote(rec.Icon),
		yamlQuote(rec.Description),
		entrancesYAML,
		rec.State.String(),
	)
}

// yamlQuote wraps a string in double quotes, escaping internal double quotes and backslashes.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
