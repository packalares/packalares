package systemserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	envoySidecarLabel = "packalares.io/envoy-injected"
)

// envoyInitImage returns the iptables init container image.
// Defaults to our own ghcr.io/packalares/envoy-init:latest.
func envoyInitImage() string {
	if v := os.Getenv("ENVOY_INIT_IMAGE"); v != "" {
		return v
	}
	return "ghcr.io/packalares/envoy-init:latest"
}

// injectEnvoySidecar patches an app's deployment to add an iptables
// init container that redirects inbound traffic through Envoy for auth.
func (s *Server) injectEnvoySidecar(app *Application) error {
	if app.Namespace == "" || app.Spec.Name == "" {
		return nil
	}

	// Find deployments for this app
	deploys, err := s.kubeClient.AppsV1().Deployments(app.Namespace).List(
		context.Background(),
		metav1.ListOptions{},
	)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	for _, deploy := range deploys.Items {
		// Skip if already injected
		if deploy.Labels != nil && deploy.Labels[envoySidecarLabel] == "true" {
			continue
		}

		// Check if any entrance needs auth
		needsAuth := false
		appPort := int32(8080)
		for _, ent := range app.Spec.Entrances {
			if ent.AuthLevel != "public" {
				needsAuth = true
			}
			if ent.Port > 0 {
				appPort = int32(ent.Port)
			}
		}
		if !needsAuth {
			continue
		}

		log.Printf("injecting envoy init into %s/%s (port %d)", deploy.Namespace, deploy.Name, appPort)

		patch := map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					envoySidecarLabel: "true",
				},
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							envoySidecarLabel: "true",
						},
					},
					"spec": map[string]interface{}{
						"initContainers": []corev1.Container{
							{
								Name:  "envoy-init",
								Image: envoyInitImage(),
								SecurityContext: &corev1.SecurityContext{
									Capabilities: &corev1.Capabilities{
										Add: []corev1.Capability{"NET_ADMIN"},
									},
								},
								Env: []corev1.EnvVar{
									{Name: "ENVOY_PORT", Value: "15001"},
								},
							},
						},
					},
				},
			},
		}

		patchBytes, err := json.Marshal(patch)
		if err != nil {
			return fmt.Errorf("marshal patch: %w", err)
		}

		_, err = s.kubeClient.AppsV1().Deployments(deploy.Namespace).Patch(
			context.Background(), deploy.Name,
			types.StrategicMergePatchType, patchBytes,
			metav1.PatchOptions{},
		)
		if err != nil {
			return fmt.Errorf("patch deployment %s: %w", deploy.Name, err)
		}

		log.Printf("envoy init injected into %s/%s", deploy.Namespace, deploy.Name)
	}

	return nil
}
