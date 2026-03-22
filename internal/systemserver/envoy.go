package systemserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	envoyImage         = "envoyproxy/envoy:v1.29-latest"
	envoyInitImage     = "openpolicyagent/iptables-init:v1.2.3"
	envoySidecarLabel  = "packalares.io/envoy-injected"
	envoyConfigMapName = "envoy-sidecar-config"
)

// injectEnvoySidecar patches an app's deployment to add an Envoy sidecar
// and iptables init container for per-pod auth enforcement.
func (s *Server) injectEnvoySidecar(app *Application) error {
	if app.Namespace == "" || app.Spec.Name == "" {
		return nil
	}

	// Find the deployment for this app
	deploys, err := s.kubeClient.AppsV1().Deployments(app.Namespace).List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", app.Spec.Name),
		},
	)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	if len(deploys.Items) == 0 {
		// Try without label selector — look for deployment with same name
		deploy, err := s.kubeClient.AppsV1().Deployments(app.Namespace).Get(
			context.Background(), app.Spec.Name, metav1.GetOptions{},
		)
		if err != nil {
			return nil // No deployment found, skip
		}
		deploys = &appsv1.DeploymentList{Items: []appsv1.Deployment{*deploy}}
	}

	for _, deploy := range deploys.Items {
		// Check if already injected
		if deploy.Labels != nil && deploy.Labels[envoySidecarLabel] == "true" {
			continue
		}

		// Check if any entrance is not public
		needsAuth := false
		appPort := int32(8080) // default
		for _, entrance := range app.Spec.Entrances {
			if entrance.AuthLevel != "public" {
				needsAuth = true
			}
			if entrance.Port > 0 {
				appPort = int32(entrance.Port)
			}
		}

		if !needsAuth {
			log.Printf("app %s has all public entrances, skipping envoy injection", app.Spec.Name)
			continue
		}

		log.Printf("injecting envoy sidecar into deployment %s/%s (port %d)", deploy.Namespace, deploy.Name, appPort)

		// Build the patch
		patch := buildEnvoyPatch(appPort)
		patchBytes, err := json.Marshal(patch)
		if err != nil {
			return fmt.Errorf("marshal patch: %w", err)
		}

		_, err = s.kubeClient.AppsV1().Deployments(deploy.Namespace).Patch(
			context.Background(),
			deploy.Name,
			types.StrategicMergePatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
		if err != nil {
			return fmt.Errorf("patch deployment %s: %w", deploy.Name, err)
		}

		log.Printf("envoy sidecar injected into %s/%s", deploy.Namespace, deploy.Name)
	}

	return nil
}

// buildEnvoyPatch creates the strategic merge patch to add Envoy sidecar.
func buildEnvoyPatch(appPort int32) map[string]interface{} {
	return map[string]interface{}{
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
							Image: envoyInitImage,
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"NET_ADMIN"},
								},
							},
							Command: []string{"sh", "-c"},
							Args: []string{
								fmt.Sprintf(`
iptables -t nat -N PACKALARES_INBOUND 2>/dev/null || true
iptables -t nat -N PACKALARES_IN_REDIRECT 2>/dev/null || true
iptables -t nat -A PACKALARES_IN_REDIRECT -p tcp -j REDIRECT --to-port 15001
iptables -t nat -A PACKALARES_INBOUND -p tcp --dport 15001 -j RETURN
iptables -t nat -A PACKALARES_INBOUND -p tcp --dport 15000 -j RETURN
iptables -t nat -A PACKALARES_INBOUND -p tcp -s 127.0.0.1/32 -j RETURN
iptables -t nat -A PACKALARES_INBOUND -p tcp -j PACKALARES_IN_REDIRECT
iptables -t nat -A PREROUTING -p tcp -j PACKALARES_INBOUND
echo "Envoy iptables configured for port %d"`, appPort),
							},
						},
					},
				},
			},
		},
	}
}
