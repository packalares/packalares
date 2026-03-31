// Package redis deploys KVRocks (Redis-compatible, SSD-backed) in Kubernetes.
// Replaces the old host-based Redis systemd installation.
// KVRocks is used for auth sessions, app caching, and shared state.
// Infisical gets its own dedicated Redis pod via middleware operator.
package redis

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/config"
)

const kvrocksManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: kvrocks
  namespace: {{NAMESPACE}}
  labels:
    app: kvrocks
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kvrocks
  template:
    metadata:
      labels:
        app: kvrocks
    spec:
      containers:
      - name: kvrocks
        image: ghcr.io/packalares/kvrocks:2.15.0
        ports:
        - containerPort: 6666
        command: ["kvrocks"]
        args:
        - --bind
        - "0.0.0.0"
        - --port
        - "6666"
        - --requirepass
        - "{{PASSWORD}}"
        - --dir
        - /data
        - --log-dir
        - stdout
        volumeMounts:
        - name: data
          mountPath: /data
        resources:
          requests:
            cpu: 50m
            memory: 32Mi
          limits:
            cpu: 500m
            memory: 256Mi
        readinessProbe:
          tcpSocket:
            port: 6666
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: kvrocks-svc
  namespace: {{NAMESPACE}}
spec:
  selector:
    app: kvrocks
  ports:
  - port: 6379
    targetPort: 6666
`

// Install deploys KVRocks in Kubernetes using the REDIS_PASSWORD
// from GenerateSecrets (must be called after GenerateSecrets).
func Install(baseDir string, w io.Writer) error {
	ns := config.PlatformNamespace()

	// Read password from env (set by GenerateSecrets)
	password := os.Getenv("REDIS_PASSWORD")
	if password == "" {
		return fmt.Errorf("REDIS_PASSWORD not set — GenerateSecrets must run before KVRocks deploy")
	}

	// Build manifest
	manifest := kvrocksManifest
	manifest = strings.ReplaceAll(manifest, "{{NAMESPACE}}", ns)
	manifest = strings.ReplaceAll(manifest, "{{PASSWORD}}", password)

	// Apply via kubectl
	fmt.Fprintln(w, "  Deploying KVRocks in Kubernetes ...")
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("deploy kvrocks: %s\n%w", string(out), err)
	}

	// Wait for pod to be ready
	fmt.Fprintln(w, "  Waiting for KVRocks to be ready ...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "get", "pods", "-n", ns,
			"-l", "app=kvrocks", "-o", "jsonpath={.items[0].status.phase}")
		out, err := cmd.Output()
		if err == nil && string(out) == "Running" {
			fmt.Fprintln(w, "  KVRocks deployed and running")
			return nil
		}
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("kvrocks did not become ready in 90s")
}
