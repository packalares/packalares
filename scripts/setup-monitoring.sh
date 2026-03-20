#!/bin/bash
# setup-monitoring.sh — Deploy Prometheus monitoring stack for Packalares
# Part of the Packalares self-hosted personal cloud OS
#
# Deploys Prometheus, Node Exporter, and kube-state-metrics.
# Can be run standalone or called from setup-k3s.sh.
#
# This script is idempotent: re-running it skips already-completed steps.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration — all from environment, with sane defaults
# ---------------------------------------------------------------------------
NODE_IP="${NODE_IP:-$(hostname -I | awk '{print $1}')}"
MONITORING_NAMESPACE="${MONITORING_NAMESPACE:-packalares-monitoring}"
DATA_PATH="${DATA_PATH:-/packalares/data}"

# Image versions
PROMETHEUS_VERSION="${PROMETHEUS_VERSION:-v2.34.0}"
NODE_EXPORTER_VERSION="${NODE_EXPORTER_VERSION:-v1.8.2}"
KUBE_STATE_METRICS_VERSION="${KUBE_STATE_METRICS_VERSION:-v2.10.1}"

# Prometheus config
PROMETHEUS_RETENTION="${PROMETHEUS_RETENTION:-15d}"
PROMETHEUS_STORAGE="${PROMETHEUS_STORAGE:-10Gi}"

KUBECONFIG_PATH="/etc/rancher/k3s/k3s.yaml"
export KUBECONFIG="$KUBECONFIG_PATH"

# ---------------------------------------------------------------------------
# Logging (same style as setup-k3s.sh)
# ---------------------------------------------------------------------------
log()  { echo "[$(date '+%H:%M:%S')] $*"; }
info() { log "INFO  $*"; }
warn() { log "WARN  $*"; }
die()  { log "FATAL $*"; exit 1; }

# ---------------------------------------------------------------------------
# Wait helper
# ---------------------------------------------------------------------------
POD_TIMEOUT="${POD_TIMEOUT:-300}"

wait_for_pods() {
    local namespace="$1"
    local label="${2:-}"
    local timeout=$POD_TIMEOUT
    local interval=10
    local elapsed=0

    local selector_flag=""
    [[ -n "$label" ]] && selector_flag="-l $label"

    info "Waiting for pods in '$namespace' ${label:+(label: $label)} (timeout: ${timeout}s)..."

    while (( elapsed < timeout )); do
        local not_ready
        not_ready=$(kubectl get pods -n "$namespace" $selector_flag --no-headers 2>/dev/null \
            | grep -cvE '(Running|Completed|Succeeded)' || true)

        if [[ "$not_ready" -eq 0 ]]; then
            local total
            total=$(kubectl get pods -n "$namespace" $selector_flag --no-headers 2>/dev/null | wc -l)
            if [[ "$total" -gt 0 ]]; then
                info "All $total pod(s) in '$namespace' ${label:+(label: $label)} are ready"
                return 0
            fi
        fi

        sleep "$interval"
        elapsed=$(( elapsed + interval ))
    done

    warn "Some pods in '$namespace' not ready after ${timeout}s — continuing anyway"
    kubectl get pods -n "$namespace" $selector_flag --no-headers 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Step 1: Create namespace
# ---------------------------------------------------------------------------
setup_namespace() {
    kubectl create namespace "$MONITORING_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    info "Namespace $MONITORING_NAMESPACE ready"
}

# ---------------------------------------------------------------------------
# Step 2: Prometheus
# ---------------------------------------------------------------------------
install_prometheus() {
    if kubectl get deployment prometheus-server -n "$MONITORING_NAMESPACE" >/dev/null 2>&1; then
        info "Prometheus already installed — skipping"
        return 0
    fi

    info "Installing Prometheus $PROMETHEUS_VERSION..."

    local prom_data="${DATA_PATH}/prometheus"
    mkdir -p "$prom_data"

    cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: prometheus
  namespace: ${MONITORING_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus
rules:
  - apiGroups: [""]
    resources: ["nodes", "nodes/proxy", "nodes/metrics", "services", "endpoints", "pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["extensions", "networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get", "list", "watch"]
  - nonResourceURLs: ["/metrics"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: prometheus
subjects:
  - kind: ServiceAccount
    name: prometheus
    namespace: ${MONITORING_NAMESPACE}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: ${MONITORING_NAMESPACE}
data:
  prometheus.yml: |
    global:
      scrape_interval: 30s
      evaluation_interval: 30s

    scrape_configs:
      - job_name: "prometheus"
        static_configs:
          - targets: ["localhost:9090"]

      - job_name: "node-exporter"
        kubernetes_sd_configs:
          - role: pod
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_label_app]
            regex: node-exporter
            action: keep
          - source_labels: [__meta_kubernetes_pod_ip]
            target_label: __address__
            replacement: "\${1}:9100"

      - job_name: "kube-state-metrics"
        static_configs:
          - targets: ["kube-state-metrics.${MONITORING_NAMESPACE}.svc:8080"]

      - job_name: "kubernetes-apiservers"
        kubernetes_sd_configs:
          - role: endpoints
        scheme: https
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
        relabel_configs:
          - source_labels: [__meta_kubernetes_namespace, __meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
            action: keep
            regex: default;kubernetes;https

      - job_name: "kubernetes-nodes"
        scheme: https
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
        kubernetes_sd_configs:
          - role: node
        relabel_configs:
          - action: labelmap
            regex: __meta_kubernetes_node_label_(.+)

      - job_name: "kubernetes-cadvisor"
        scheme: https
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
        kubernetes_sd_configs:
          - role: node
        relabel_configs:
          - action: labelmap
            regex: __meta_kubernetes_node_label_(.+)
          - target_label: __metrics_path__
            replacement: /metrics/cadvisor

      - job_name: "kubernetes-service-endpoints"
        kubernetes_sd_configs:
          - role: endpoints
        relabel_configs:
          - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_scrape]
            action: keep
            regex: true
          - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_path]
            action: replace
            target_label: __metrics_path__
            regex: (.+)
          - source_labels: [__address__, __meta_kubernetes_service_annotation_prometheus_io_port]
            action: replace
            regex: ([^:]+)(?::\d+)?;(\d+)
            replacement: "\${1}:\${2}"
            target_label: __address__
          - action: labelmap
            regex: __meta_kubernetes_service_label_(.+)
          - source_labels: [__meta_kubernetes_namespace]
            action: replace
            target_label: kubernetes_namespace
          - source_labels: [__meta_kubernetes_service_name]
            action: replace
            target_label: kubernetes_name
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-server
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      serviceAccountName: prometheus
      containers:
        - name: prometheus
          image: prom/prometheus:${PROMETHEUS_VERSION}
          imagePullPolicy: IfNotPresent
          args:
            - "--config.file=/etc/prometheus/prometheus.yml"
            - "--storage.tsdb.path=/prometheus"
            - "--storage.tsdb.retention.time=${PROMETHEUS_RETENTION}"
            - "--web.enable-lifecycle"
          ports:
            - containerPort: 9090
              name: http
          volumeMounts:
            - name: config
              mountPath: /etc/prometheus
            - name: data
              mountPath: /prometheus
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
          readinessProbe:
            httpGet:
              path: /-/ready
              port: 9090
            initialDelaySeconds: 10
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /-/healthy
              port: 9090
            initialDelaySeconds: 30
            periodSeconds: 15
      volumes:
        - name: config
          configMap:
            name: prometheus-config
        - name: data
          hostPath:
            path: ${prom_data}
            type: DirectoryOrCreate
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: prometheus
spec:
  type: ClusterIP
  ports:
    - port: 9090
      targetPort: 9090
      name: http
  selector:
    app: prometheus
EOF

    wait_for_pods "$MONITORING_NAMESPACE" "app=prometheus"

    info "Prometheus installed"
}

# ---------------------------------------------------------------------------
# Step 3: Node Exporter (DaemonSet)
# ---------------------------------------------------------------------------
install_node_exporter() {
    if kubectl get daemonset node-exporter -n "$MONITORING_NAMESPACE" >/dev/null 2>&1; then
        info "Node Exporter already installed — skipping"
        return 0
    fi

    info "Installing Node Exporter $NODE_EXPORTER_VERSION..."

    cat <<EOF | kubectl apply -f -
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-exporter
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: node-exporter
spec:
  selector:
    matchLabels:
      app: node-exporter
  template:
    metadata:
      labels:
        app: node-exporter
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9100"
    spec:
      hostPID: true
      hostNetwork: true
      containers:
        - name: node-exporter
          image: prom/node-exporter:${NODE_EXPORTER_VERSION}
          imagePullPolicy: IfNotPresent
          args:
            - "--path.procfs=/host/proc"
            - "--path.sysfs=/host/sys"
            - "--path.rootfs=/host/root"
            - "--collector.filesystem.mount-points-exclude=^/(dev|proc|sys|var/lib/docker/.+|var/lib/kubelet/.+)($|/)"
          ports:
            - containerPort: 9100
              hostPort: 9100
              name: metrics
          volumeMounts:
            - name: proc
              mountPath: /host/proc
              readOnly: true
            - name: sys
              mountPath: /host/sys
              readOnly: true
            - name: root
              mountPath: /host/root
              readOnly: true
              mountPropagation: HostToContainer
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 200m
              memory: 128Mi
      tolerations:
        - effect: NoSchedule
          operator: Exists
      volumes:
        - name: proc
          hostPath:
            path: /proc
        - name: sys
          hostPath:
            path: /sys
        - name: root
          hostPath:
            path: /
---
apiVersion: v1
kind: Service
metadata:
  name: node-exporter
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: node-exporter
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9100"
spec:
  type: ClusterIP
  clusterIP: None
  ports:
    - port: 9100
      targetPort: 9100
      name: metrics
  selector:
    app: node-exporter
EOF

    wait_for_pods "$MONITORING_NAMESPACE" "app=node-exporter"

    info "Node Exporter installed"
}

# ---------------------------------------------------------------------------
# Step 4: kube-state-metrics
# ---------------------------------------------------------------------------
install_kube_state_metrics() {
    if kubectl get deployment kube-state-metrics -n "$MONITORING_NAMESPACE" >/dev/null 2>&1; then
        info "kube-state-metrics already installed — skipping"
        return 0
    fi

    info "Installing kube-state-metrics $KUBE_STATE_METRICS_VERSION..."

    cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-state-metrics
  namespace: ${MONITORING_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-state-metrics
rules:
  - apiGroups: [""]
    resources:
      - configmaps
      - secrets
      - nodes
      - pods
      - services
      - serviceaccounts
      - resourcequotas
      - replicationcontrollers
      - limitranges
      - persistentvolumeclaims
      - persistentvolumes
      - namespaces
      - endpoints
    verbs: ["list", "watch"]
  - apiGroups: ["apps"]
    resources:
      - statefulsets
      - daemonsets
      - deployments
      - replicasets
    verbs: ["list", "watch"]
  - apiGroups: ["batch"]
    resources:
      - cronjobs
      - jobs
    verbs: ["list", "watch"]
  - apiGroups: ["autoscaling"]
    resources:
      - horizontalpodautoscalers
    verbs: ["list", "watch"]
  - apiGroups: ["authentication.k8s.io"]
    resources:
      - tokenreviews
    verbs: ["create"]
  - apiGroups: ["authorization.k8s.io"]
    resources:
      - subjectaccessreviews
    verbs: ["create"]
  - apiGroups: ["policy"]
    resources:
      - poddisruptionbudgets
    verbs: ["list", "watch"]
  - apiGroups: ["certificates.k8s.io"]
    resources:
      - certificatesigningrequests
    verbs: ["list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources:
      - storageclasses
      - volumeattachments
    verbs: ["list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources:
      - networkpolicies
      - ingresses
    verbs: ["list", "watch"]
  - apiGroups: ["coordination.k8s.io"]
    resources:
      - leases
    verbs: ["list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-state-metrics
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-state-metrics
subjects:
  - kind: ServiceAccount
    name: kube-state-metrics
    namespace: ${MONITORING_NAMESPACE}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-state-metrics
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: kube-state-metrics
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-state-metrics
  template:
    metadata:
      labels:
        app: kube-state-metrics
    spec:
      serviceAccountName: kube-state-metrics
      containers:
        - name: kube-state-metrics
          image: registry.k8s.io/kube-state-metrics/kube-state-metrics:${KUBE_STATE_METRICS_VERSION}
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
              name: http-metrics
            - containerPort: 8081
              name: telemetry
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: kube-state-metrics
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: kube-state-metrics
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
      name: http-metrics
    - port: 8081
      targetPort: 8081
      name: telemetry
  selector:
    app: kube-state-metrics
EOF

    wait_for_pods "$MONITORING_NAMESPACE" "app=kube-state-metrics"

    info "kube-state-metrics installed"
}

# ---------------------------------------------------------------------------
# Step 5: ServiceMonitor CRDs (if prometheus-operator is available)
# ---------------------------------------------------------------------------
install_service_monitors() {
    # Only create ServiceMonitors if the CRD exists (prometheus-operator)
    if ! kubectl get crd servicemonitors.monitoring.coreos.com >/dev/null 2>&1; then
        info "ServiceMonitor CRD not found (no prometheus-operator) — skipping ServiceMonitors"
        return 0
    fi

    info "Creating ServiceMonitor resources..."

    cat <<EOF | kubectl apply -f -
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: node-exporter
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: node-exporter
spec:
  selector:
    matchLabels:
      app: node-exporter
  endpoints:
    - port: metrics
      interval: 30s
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kube-state-metrics
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: kube-state-metrics
spec:
  selector:
    matchLabels:
      app: kube-state-metrics
  endpoints:
    - port: http-metrics
      interval: 30s
    - port: telemetry
      interval: 30s
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: prometheus
  namespace: ${MONITORING_NAMESPACE}
  labels:
    app: prometheus
spec:
  selector:
    matchLabels:
      app: prometheus
  endpoints:
    - port: http
      interval: 30s
EOF

    info "ServiceMonitors created"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    echo ""
    echo "========================================"
    echo "  Packalares — Monitoring Stack Setup"
    echo "========================================"
    echo ""

    [[ $EUID -eq 0 ]] || die "Must run as root"

    if [[ ! -f "$KUBECONFIG_PATH" ]]; then
        die "Kubeconfig not found at $KUBECONFIG_PATH — run setup-k3s.sh first"
    fi

    if ! kubectl cluster-info >/dev/null 2>&1; then
        die "Kubernetes API not reachable — is K3s running?"
    fi

    setup_namespace
    install_prometheus
    install_node_exporter
    install_kube_state_metrics
    install_service_monitors

    echo ""
    echo "========================================"
    echo "  Monitoring Stack Summary"
    echo "========================================"
    echo ""
    echo "  Namespace:           $MONITORING_NAMESPACE"
    echo "  Prometheus:          $PROMETHEUS_VERSION"
    echo "  Node Exporter:       $NODE_EXPORTER_VERSION"
    echo "  kube-state-metrics:  $KUBE_STATE_METRICS_VERSION"
    echo "  Retention:           $PROMETHEUS_RETENTION"
    echo "  Data:                $DATA_PATH/prometheus"
    echo ""
    echo "  Access Prometheus:"
    echo "    kubectl port-forward -n $MONITORING_NAMESPACE svc/prometheus 9090:9090"
    echo "    then open http://localhost:9090"
    echo ""
    echo "========================================"

    info "Monitoring stack setup complete"
}

main "$@"
