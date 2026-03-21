package storage

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func DeployOpenEBS(registry string) error {
	fmt.Println("  Deploying OpenEBS storage ...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create namespace
	exec.CommandContext(ctx, "kubectl", "create", "namespace", "openebs", "--dry-run=client", "-o", "yaml").CombinedOutput()
	exec.CommandContext(ctx, "kubectl", "create", "namespace", "openebs").CombinedOutput()

	// Deploy OpenEBS via manifest
	openEBSManifest := generateOpenEBSManifest(registry)
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	go func() {
		defer stdinPipe.Close()
		stdinPipe.Write([]byte(openEBSManifest))
	}()

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply openebs: %s\n%w", string(out), err)
	}

	// Wait briefly for the provisioner to start
	time.Sleep(10 * time.Second)

	// Make openebs-hostpath the default storage class
	defaultSCManifest := `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: openebs-hostpath
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: openebs.io/local
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
`

	cmd2 := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	stdinPipe2, err := cmd2.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	go func() {
		defer stdinPipe2.Close()
		stdinPipe2.Write([]byte(defaultSCManifest))
	}()

	if out, err := cmd2.CombinedOutput(); err != nil {
		return fmt.Errorf("apply default storageclass: %s\n%w", string(out), err)
	}

	fmt.Println("  OpenEBS deployed with openebs-hostpath as default StorageClass")
	return nil
}

func generateOpenEBSManifest(registry string) string {
	img := "openebs/provisioner-localpv:3.5.0"
	if registry != "" {
		img = registry + "/openebs/provisioner-localpv:3.5.0"
	}

	return fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  name: openebs
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openebs-localpv-provisioner
  namespace: openebs
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: openebs-localpv-provisioner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: openebs-localpv-provisioner
  namespace: openebs
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openebs-localpv-provisioner
  namespace: openebs
  labels:
    app: openebs-localpv-provisioner
spec:
  replicas: 1
  selector:
    matchLabels:
      app: openebs-localpv-provisioner
  template:
    metadata:
      labels:
        app: openebs-localpv-provisioner
    spec:
      serviceAccountName: openebs-localpv-provisioner
      containers:
      - name: provisioner
        image: %s
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: OPENEBS_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: OPENEBS_IO_BASE_PATH
          value: "/var/openebs/local"
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 256Mi
`, img)
}
