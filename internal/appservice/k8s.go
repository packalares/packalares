package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// applicationGVR is the GroupVersionResource for the Olares Application CRD.
var applicationGVR = schema.GroupVersionResource{
	Group:    "app.bytetrade.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// K8sClient provides a simplified Kubernetes interface. It shells out to
// kubectl for simple operations and uses the dynamic client-go client for
// CRD operations to avoid broken hand-rolled YAML.
type K8sClient struct {
	dynClient dynamic.Interface
}

// NewK8sClient creates a new k8s client. The dynamic client is initialised
// from the in-cluster config; if that fails (e.g. running outside a cluster
// for development) we fall back to kubectl-only mode.
func NewK8sClient() *K8sClient {
	k := &K8sClient{}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		klog.V(2).Infof("not running in-cluster, dynamic client unavailable: %v", err)
		return k
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		klog.Warningf("failed to create dynamic k8s client: %v", err)
		return k
	}

	k.dynClient = dyn
	return k
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
	yamlOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("generate namespace yaml: %w", err)
	}
	return k.ApplyManifest(ctx, string(yamlOut))
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

// ApplyApplicationCRD creates or updates an Application CRD resource using
// the dynamic K8s client. This replaces the old ApplicationCRDManifest()
// approach which hand-built YAML via fmt.Sprintf and produced broken output.
func (k *K8sClient) ApplyApplicationCRD(ctx context.Context, rec *AppRecord) error {
	obj := buildApplicationObject(rec)

	if k.dynClient != nil {
		return k.applyApplicationDynamic(ctx, obj, rec.Namespace)
	}

	// Fallback: marshal to JSON and pipe through kubectl apply
	return k.applyApplicationKubectl(ctx, obj)
}

// DeleteApplicationCRD removes an Application CRD resource.
func (k *K8sClient) DeleteApplicationCRD(ctx context.Context, name, namespace string) error {
	if k.dynClient != nil {
		err := k.dynClient.Resource(applicationGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("delete Application %s/%s: %w", namespace, name, err)
		}
		return nil
	}

	// Fallback: kubectl
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "application.app.bytetrade.io",
		name, "--namespace", namespace, "--ignore-not-found")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl delete application: %s: %w", string(out), err)
	}
	return nil
}

// buildApplicationObject constructs an unstructured Application object from
// an AppRecord. This is the single source of truth for the Application CRD
// schema -- no more hand-rolled YAML templates.
func buildApplicationObject(rec *AppRecord) *unstructured.Unstructured {
	entrancesJSON, err := json.Marshal(rec.Entrances)
	if err != nil {
		klog.Errorf("marshal entrances for %s: %v", rec.Name, err)
		entrancesJSON = []byte("[]")
	}

	// Build spec.entrances as a list of maps
	specEntrances := make([]interface{}, 0, len(rec.Entrances))
	for _, e := range rec.Entrances {
		entry := map[string]interface{}{
			"name": e.Name,
			"host": e.Host,
			"port": int64(e.Port),
		}
		if e.Title != "" {
			entry["title"] = e.Title
		}
		if e.Icon != "" {
			entry["icon"] = e.Icon
		}
		if e.AuthLevel != "" {
			entry["authLevel"] = e.AuthLevel
		}
		if e.Invisible {
			entry["invisible"] = true
		}
		if e.OpenMethod != "" {
			entry["openMethod"] = e.OpenMethod
		}
		specEntrances = append(specEntrances, entry)
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "app.bytetrade.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      rec.ReleaseName,
				"namespace": rec.Namespace,
				"labels": map[string]interface{}{
					"applications.app.bytetrade.io/name":  rec.Name,
					"applications.app.bytetrade.io/owner": rec.Owner,
				},
				"annotations": map[string]interface{}{
					"applications.app.bytetrade.io/entrances": string(entrancesJSON),
					"applications.app.bytetrade.io/icon":      rec.Icon,
					"applications.app.bytetrade.io/title":     rec.Title,
					"applications.app.bytetrade.io/version":   rec.Version,
					"applications.app.bytetrade.io/source":    rec.Source,
				},
			},
			"spec": map[string]interface{}{
				"name":        rec.Name,
				"appid":       rec.AppID,
				"namespace":   rec.Namespace,
				"owner":       rec.Owner,
				"isSysApp":    rec.IsSysApp,
				"icon":        rec.Icon,
				"description": rec.Description,
				"entrances":   specEntrances,
			},
			"status": map[string]interface{}{
				"state": rec.State.String(),
			},
		},
	}

	return obj
}

// applyApplicationDynamic creates or updates the Application resource using
// the dynamic K8s client with server-side apply semantics.
func (k *K8sClient) applyApplicationDynamic(ctx context.Context, obj *unstructured.Unstructured, namespace string) error {
	client := k.dynClient.Resource(applicationGVR).Namespace(namespace)
	name := obj.GetName()

	// Try to get existing resource
	existing, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// Does not exist -- create
		_, createErr := client.Create(ctx, obj, metav1.CreateOptions{})
		if createErr != nil {
			return fmt.Errorf("create Application %s: %w", name, createErr)
		}
		klog.Infof("created Application CRD %s/%s", namespace, name)
		return nil
	}

	// Exists -- update (preserve resourceVersion for optimistic locking)
	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = client.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update Application %s: %w", name, err)
	}
	klog.Infof("updated Application CRD %s/%s", namespace, name)
	return nil
}

// applyApplicationKubectl is the fallback when the dynamic client is not
// available. It marshals the object to JSON and pipes through kubectl apply.
func (k *K8sClient) applyApplicationKubectl(ctx context.Context, obj *unstructured.Unstructured) error {
	data, err := json.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("marshal Application object: %w", err)
	}

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(data))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply Application: %s: %w", string(out), err)
	}
	return nil
}
