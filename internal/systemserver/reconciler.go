package systemserver

import (
	"context"
	"fmt"
	"log"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ConsumerReconciler watches for provider changes and updates consumer
// deployments with the correct provider endpoint environment variables.
type ConsumerReconciler struct {
	kubeClient kubernetes.Interface
	registry   *ProviderRegistry
}

// NewConsumerReconciler creates a new reconciler.
func NewConsumerReconciler(kubeClient kubernetes.Interface, registry *ProviderRegistry) *ConsumerReconciler {
	return &ConsumerReconciler{
		kubeClient: kubeClient,
		registry:   registry,
	}
}

// Reconcile updates a single consumer app's deployment with the correct
// provider endpoint environment variables based on its permission.sysData entries.
func (cr *ConsumerReconciler) Reconcile(consumer *Application, allApps map[string]*Application) {
	if consumer.Spec.Permission == nil || len(consumer.Spec.Permission.SysData) == 0 {
		return
	}

	// Collect all env vars this consumer needs from providers
	envVars := cr.buildProviderEnvVars(consumer)
	if len(envVars) == 0 {
		return
	}

	ns := consumer.Spec.Namespace
	if ns == "" {
		ns = consumer.Namespace
	}

	// Find and patch deployments for this app
	if err := cr.patchDeploymentEnvVars(ns, consumer.Spec.Name, envVars); err != nil {
		log.Printf("reconciler: failed to patch deployment for %s: %v", consumer.Spec.Name, err)
	}
}

// ReconcileAll reconciles all consumer apps.
func (cr *ConsumerReconciler) ReconcileAll(allApps map[string]*Application) {
	for _, app := range allApps {
		if app.Spec.Permission != nil && len(app.Spec.Permission.SysData) > 0 {
			cr.Reconcile(app, allApps)
		}
	}
}

// OnProviderChanged is called when a provider app is added, updated, or removed.
// It finds all consumers that depend on the provider's groups and reconciles them.
func (cr *ConsumerReconciler) OnProviderChanged(provider *Application, allApps map[string]*Application) {
	if len(provider.Spec.SharedEntrances) == 0 {
		return
	}

	// Collect all groups this provider offers
	groups := make(map[string]bool)
	for _, se := range provider.Spec.SharedEntrances {
		if se.Name != "" {
			groups[se.Name] = true
		}
	}

	// Find and reconcile all consumers that depend on these groups
	reconciled := 0
	for group := range groups {
		consumers := cr.registry.FindConsumers(allApps, group)
		for _, consumer := range consumers {
			cr.Reconcile(consumer, allApps)
			reconciled++
		}
	}

	if reconciled > 0 {
		log.Printf("reconciler: reconciled %d consumers after provider %q changed", reconciled, provider.Spec.Name)
	}
}

// buildProviderEnvVars builds the environment variables a consumer needs
// based on its permission.sysData entries and the current provider registry.
//
// For each sysData entry with a group like "api.ollama", we look up all
// providers for that group and build a comma-separated URL list:
//
//	OLLAMA_BASE_URLS=http://svc1.ns.svc.cluster.local:11434,http://svc2.ns.svc.cluster.local:11434
//
// The env var name is derived from the group: take the part after "api.",
// uppercase it, and append "_BASE_URLS".
func (cr *ConsumerReconciler) buildProviderEnvVars(consumer *Application) map[string]string {
	envVars := make(map[string]string)

	for _, sd := range consumer.Spec.Permission.SysData {
		if sd.Group == "" {
			continue
		}

		providers := cr.registry.GetProviders(sd.Group)
		if len(providers) == 0 {
			// If the sysData entry has explicit svc/port info, use that as a
			// direct endpoint (static provider, not from registry).
			if sd.Svc != "" && sd.Port > 0 {
				ns := consumer.Spec.Namespace
				if ns == "" {
					ns = consumer.Namespace
				}
				// Use the appName namespace if specified via sysData
				if sd.AppName != "" {
					ns = consumer.Spec.Namespace // consumer's own namespace
				}
				url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", sd.Svc, ns, sd.Port)
				envName := groupToEnvVarName(sd.Group)
				envVars[envName] = url
			}
			continue
		}

		// Build comma-separated URL list from all provider endpoints
		var urls []string
		for _, ep := range providers {
			url := fmt.Sprintf("http://%s:%d", ep.Host, ep.Port)
			urls = append(urls, url)
		}

		envName := groupToEnvVarName(sd.Group)
		envVars[envName] = strings.Join(urls, ",")
	}

	// Also process permission.provider entries
	if consumer.Spec.Permission != nil {
		for _, pe := range consumer.Spec.Permission.Provider {
			if pe.ProviderName == "" {
				continue
			}
			// Look up providers with the providerName as group
			providers := cr.registry.GetProviders(pe.ProviderName)
			if len(providers) == 0 {
				continue
			}
			var urls []string
			for _, ep := range providers {
				url := fmt.Sprintf("http://%s:%d", ep.Host, ep.Port)
				urls = append(urls, url)
			}
			envName := groupToEnvVarName(pe.ProviderName)
			envVars[envName] = strings.Join(urls, ",")
		}
	}

	return envVars
}

// groupToEnvVarName converts a group like "api.ollama" to "OLLAMA_BASE_URLS".
// If the group has no dot prefix, the whole string is uppercased.
func groupToEnvVarName(group string) string {
	// Strip common prefixes like "api."
	name := group
	if idx := strings.LastIndex(group, "."); idx >= 0 {
		name = group[idx+1:]
	}

	// Uppercase and sanitize for env var usage
	name = strings.ToUpper(name)
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")

	return name + "_BASE_URLS"
}

// patchDeploymentEnvVars finds deployments for an app by label selector and
// patches them to include/update the given environment variables.
func (cr *ConsumerReconciler) patchDeploymentEnvVars(namespace, appName string, envVars map[string]string) error {
	ctx := context.Background()

	// List deployments matching this app's label
	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s", appName)
	deployments, err := cr.kubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("list deployments for %s: %w", appName, err)
	}

	// Try multiple label selectors (different charts use different labeling)
	fallbackSelectors := []string{
		fmt.Sprintf("app=%s", appName),
		fmt.Sprintf("io.kompose.service=%s", appName),
	}
	for _, sel := range fallbackSelectors {
		if len(deployments.Items) > 0 {
			break
		}
		deployments, err = cr.kubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: sel,
		})
		if err != nil {
			return fmt.Errorf("list deployments for %s (%s): %w", appName, sel, err)
		}
	}

	if len(deployments.Items) == 0 {
		log.Printf("reconciler: no deployments found for app %s in namespace %s", appName, namespace)
		return nil
	}

	for i := range deployments.Items {
		deploy := &deployments.Items[i]
		if cr.updateDeploymentEnv(deploy, envVars) {
			_, err := cr.kubeClient.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
			if err != nil {
				log.Printf("reconciler: failed to update deployment %s/%s: %v", namespace, deploy.Name, err)
				continue
			}
			log.Printf("reconciler: updated deployment %s/%s with %d provider env vars", namespace, deploy.Name, len(envVars))
		}
	}

	return nil
}

// updateDeploymentEnv updates the environment variables in all containers
// of a deployment. Returns true if any changes were made.
func (cr *ConsumerReconciler) updateDeploymentEnv(deploy *appsv1.Deployment, envVars map[string]string) bool {
	changed := false

	for ci := range deploy.Spec.Template.Spec.Containers {
		container := &deploy.Spec.Template.Spec.Containers[ci]
		changed = mergeEnvVars(container, envVars) || changed
	}

	return changed
}

// mergeEnvVars adds or updates environment variables in a container.
// Returns true if any changes were made.
func mergeEnvVars(container *corev1.Container, envVars map[string]string) bool {
	changed := false

	// Build a map of existing env vars for quick lookup
	existingIdx := make(map[string]int)
	for i, ev := range container.Env {
		existingIdx[ev.Name] = i
	}

	for name, value := range envVars {
		if idx, exists := existingIdx[name]; exists {
			// Update existing var if value changed
			if container.Env[idx].Value != value {
				container.Env[idx].Value = value
				changed = true
			}
		} else {
			// Add new env var
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  name,
				Value: value,
			})
			changed = true
		}
	}

	return changed
}
