package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// PodStatus represents a running pod's summary info.
type PodStatus struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Node      string `json:"node"`
	IP        string `json:"ip"`
	Ready     string `json:"ready"`
	Restarts  int32  `json:"restarts"`
	Age       string `json:"age"`
}

// StatusResponse is the JSON response for GET /api/status.
type StatusResponse struct {
	Pods []PodStatus `json:"pods"`
}

// getKubeClient creates a Kubernetes client, trying in-cluster config first,
// then falling back to the default kubeconfig.
func getKubeClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home, _ := os.UserHomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("cannot build kube config: %w", err)
		}
	}
	config.Timeout = 10 * time.Second
	return kubernetes.NewForConfig(config)
}

// collectPodStatus queries the Kubernetes API for all running pods.
func collectPodStatus() (*StatusResponse, error) {
	clientset, err := getKubeClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	var result []PodStatus
	now := time.Now()
	for _, pod := range pods.Items {
		// Count ready containers and total restarts
		readyCount := 0
		totalContainers := len(pod.Spec.Containers)
		var restarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
			restarts += cs.RestartCount
		}

		age := now.Sub(pod.CreationTimestamp.Time)

		result = append(result, PodStatus{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			Node:      pod.Spec.NodeName,
			IP:        pod.Status.PodIP,
			Ready:     fmt.Sprintf("%d/%d", readyCount, totalContainers),
			Restarts:  restarts,
			Age:       formatDuration(age),
		})
	}

	return &StatusResponse{Pods: result}, nil
}

// formatDuration returns a human-readable age string like "2d", "5h", "30m".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
