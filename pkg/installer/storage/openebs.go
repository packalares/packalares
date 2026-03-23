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

	manifest := generateOpenEBSManifest(registry)
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	go func() {
		defer stdinPipe.Close()
		stdinPipe.Write([]byte(manifest))
	}()

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply openebs: %s\n%w", string(out), err)
	}

	// Wait for provisioner to start
	fmt.Println("  Waiting for OpenEBS provisioner ...")
	for i := 0; i < 30; i++ {
		check := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", "kube-system",
			"-l", "name=openebs-localpv-provisioner", "-o", "jsonpath={.items[0].status.phase}")
		out, err := check.Output()
		if err == nil && string(out) == "Running" {
			break
		}
		time.Sleep(3 * time.Second)
	}

	fmt.Println("  OpenEBS deployed with default StorageClass")
	return nil
}

func generateOpenEBSManifest(registry string) string {
	provisionerImg := "openebs/provisioner-localpv:3.3.0"
	linuxUtilsImg := "openebs/linux-utils:3.3.0"
	if registry != "" {
		provisionerImg = registry + "/" + provisionerImg
		linuxUtilsImg = registry + "/" + linuxUtilsImg
	}

	return fmt.Sprintf(`---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: local
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
    openebs.io/cas-type: local
    cas.openebs.io/config: |
      - name: StorageType
        value: "hostpath"
      - name: BasePath
        value: "/var/openebs/local/"
provisioner: openebs.io/local
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openebs-maya-operator
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: openebs-maya-operator
rules:
- apiGroups: ["*"]
  resources: ["nodes", "nodes/proxy"]
  verbs: ["*"]
- apiGroups: ["*"]
  resources: ["namespaces", "services", "pods", "pods/exec", "deployments", "deployments/finalizers", "replicationcontrollers", "replicasets", "events", "endpoints", "configmaps", "secrets", "jobs", "cronjobs"]
  verbs: ["*"]
- apiGroups: ["*"]
  resources: ["statefulsets", "daemonsets"]
  verbs: ["*"]
- apiGroups: ["*"]
  resources: ["resourcequotas", "limitranges"]
  verbs: ["list", "watch"]
- apiGroups: ["*"]
  resources: ["storageclasses", "persistentvolumeclaims", "persistentvolumes"]
  verbs: ["*"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list", "create", "update", "delete", "patch"]
- apiGroups: ["openebs.io"]
  resources: ["*"]
  verbs: ["*"]
- nonResourceURLs: ["/metrics"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: openebs-maya-operator
subjects:
- kind: ServiceAccount
  name: openebs-maya-operator
  namespace: kube-system
roleRef:
  kind: ClusterRole
  name: openebs-maya-operator
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openebs-localpv-provisioner
  namespace: kube-system
  labels:
    name: openebs-localpv-provisioner
    openebs.io/component-name: openebs-localpv-provisioner
    openebs.io/version: 3.3.0
spec:
  selector:
    matchLabels:
      name: openebs-localpv-provisioner
      openebs.io/component-name: openebs-localpv-provisioner
  replicas: 1
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        name: openebs-localpv-provisioner
        openebs.io/component-name: openebs-localpv-provisioner
        openebs.io/version: 3.3.0
    spec:
      serviceAccountName: openebs-maya-operator
      containers:
      - name: openebs-provisioner-hostpath
        imagePullPolicy: IfNotPresent
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
        - name: OPENEBS_SERVICE_ACCOUNT
          valueFrom:
            fieldRef:
              fieldPath: spec.serviceAccountName
        - name: OPENEBS_IO_ENABLE_ANALYTICS
          value: "false"
        - name: OPENEBS_IO_INSTALLER_TYPE
          value: "openebs-operator-lite"
        - name: OPENEBS_IO_HELPER_IMAGE
          value: "%s"
        livenessProbe:
          exec:
            command:
            - sh
            - -c
            - test $(pgrep -c "^provisioner-loc.*") = 1
          initialDelaySeconds: 30
          periodSeconds: 60
`, provisionerImg, linuxUtilsImg)
}
