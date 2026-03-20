#!/bin/bash
# setup-k3s.sh — Install K3s, Calico CNI, and OpenEBS for Packalares
# Part of the Packalares self-hosted personal cloud OS
#
# This script is idempotent: re-running it skips already-completed steps.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration — all from environment, with sane defaults
# ---------------------------------------------------------------------------
NODE_IP="${NODE_IP:-$(hostname -I | awk '{print $1}')}"
NODE_NAME="${NODE_NAME:-$(hostname -s)}"
KUBE_PROXY_MODE="${KUBE_PROXY_MODE:-ipvs}"
DATA_PATH="${DATA_PATH:-/packalares/data}"
K3S_VERSION="${K3S_VERSION:-v1.33.3+k3s1}"

KUBECONFIG_PATH="/etc/rancher/k3s/k3s.yaml"
export KUBECONFIG="$KUBECONFIG_PATH"

# Calico and OpenEBS versions — pulled from images.yaml defaults
CALICO_VERSION="${CALICO_VERSION:-v3.29.2}"
OPENEBS_VERSION="${OPENEBS_VERSION:-3.3.0}"

# Timeouts
READY_TIMEOUT="${READY_TIMEOUT:-300}"
POD_TIMEOUT="${POD_TIMEOUT:-300}"

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log()  { echo "[$(date '+%H:%M:%S')] $*"; }
info() { log "INFO  $*"; }
warn() { log "WARN  $*"; }
die()  { log "FATAL $*"; exit 1; }

# ---------------------------------------------------------------------------
# Preflight checks
# ---------------------------------------------------------------------------
preflight() {
    info "Running preflight checks..."

    [[ $EUID -eq 0 ]] || die "Must run as root"

    if ! command -v curl >/dev/null 2>&1; then
        die "curl is required but not installed"
    fi

    if [[ -z "$NODE_IP" ]]; then
        die "Could not determine NODE_IP — set it explicitly"
    fi

    # Validate NODE_IP looks like an IPv4 address
    if ! [[ "$NODE_IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        die "NODE_IP=$NODE_IP does not look like a valid IPv4 address"
    fi

    # Check required kernel modules for IPVS mode
    if [[ "$KUBE_PROXY_MODE" == "ipvs" ]]; then
        local modules=(ip_vs ip_vs_rr ip_vs_wrr ip_vs_sh nf_conntrack)
        for mod in "${modules[@]}"; do
            if ! lsmod | grep -qw "$mod" 2>/dev/null; then
                modprobe "$mod" 2>/dev/null || warn "Could not load kernel module: $mod"
            fi
        done
    fi

    # Create data directory
    mkdir -p "$DATA_PATH"

    info "Preflight OK — NODE_IP=$NODE_IP NODE_NAME=$NODE_NAME"
}

# ---------------------------------------------------------------------------
# Wait helpers
# ---------------------------------------------------------------------------

# Wait for a specific node to be Ready
wait_for_node_ready() {
    local timeout=$READY_TIMEOUT
    local interval=5
    local elapsed=0

    info "Waiting for node '$NODE_NAME' to be Ready (timeout: ${timeout}s)..."

    while (( elapsed < timeout )); do
        if kubectl get node "$NODE_NAME" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q "True"; then
            info "Node '$NODE_NAME' is Ready"
            return 0
        fi
        sleep "$interval"
        elapsed=$(( elapsed + interval ))
    done

    die "Node '$NODE_NAME' not Ready after ${timeout}s"
}

# Wait for all pods in a namespace to be Running/Succeeded
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
# Step 1: Install K3s
# ---------------------------------------------------------------------------
install_k3s() {
    if command -v k3s >/dev/null 2>&1 && k3s kubectl get node "$NODE_NAME" >/dev/null 2>&1; then
        info "K3s already installed and running — skipping install"
        return 0
    fi

    info "Installing K3s $K3S_VERSION..."

    mkdir -p /etc/rancher/k3s

    # Write K3s config file
    cat > /etc/rancher/k3s/config.yaml <<EOF
node-ip: "${NODE_IP}"
node-name: "${NODE_NAME}"
bind-address: "${NODE_IP}"
advertise-address: "${NODE_IP}"

flannel-backend: "none"
disable-network-policy: true
disable:
  - traefik
  - servicelb
  - local-storage

cluster-cidr: "10.42.0.0/16"
service-cidr: "10.43.0.0/16"
cluster-dns: "10.43.0.10"

kube-proxy-arg:
  - "proxy-mode=${KUBE_PROXY_MODE}"
  - "conntrack-max-per-core=0"

kubelet-arg:
  - "max-pods=110"
  - "serialize-image-pulls=false"
  - "registry-qps=0"
  - "registry-burst=20"

data-dir: "${DATA_PATH}/k3s"

write-kubeconfig: "${KUBECONFIG_PATH}"
write-kubeconfig-mode: "0644"
EOF

    # Install K3s using the official script
    curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="$K3S_VERSION" sh -s - server \
        --config /etc/rancher/k3s/config.yaml

    info "K3s install command completed"

    # Wait for kubeconfig to appear
    local waited=0
    while [[ ! -f "$KUBECONFIG_PATH" ]] && (( waited < 60 )); do
        sleep 2
        waited=$(( waited + 2 ))
    done

    if [[ ! -f "$KUBECONFIG_PATH" ]]; then
        die "Kubeconfig not found at $KUBECONFIG_PATH after 60s"
    fi

    # Set up standard kubeconfig location
    mkdir -p /root/.kube
    ln -sf "$KUBECONFIG_PATH" /root/.kube/config

    info "K3s installed — kubeconfig at $KUBECONFIG_PATH"
}

# ---------------------------------------------------------------------------
# Step 2: Wait for cluster readiness
# ---------------------------------------------------------------------------
wait_for_cluster() {
    info "Waiting for K3s API server..."

    local waited=0
    while ! kubectl cluster-info >/dev/null 2>&1 && (( waited < 120 )); do
        sleep 3
        waited=$(( waited + 3 ))
    done

    if ! kubectl cluster-info >/dev/null 2>&1; then
        die "K3s API server not reachable after 120s"
    fi

    info "API server is up"
    wait_for_node_ready
}

# ---------------------------------------------------------------------------
# Step 3: Install Calico CNI
# ---------------------------------------------------------------------------
install_calico() {
    if kubectl get daemonset calico-node -n kube-system >/dev/null 2>&1; then
        info "Calico already installed — skipping"
        return 0
    fi

    info "Installing Calico CNI $CALICO_VERSION..."

    # Apply the Calico operator and CRDs
    kubectl apply -f "https://raw.githubusercontent.com/projectcalico/calico/${CALICO_VERSION}/manifests/calico.yaml"

    # Patch Calico to use the correct IP autodetection for single-node
    # Wait for the calico-node daemonset to exist before patching
    local waited=0
    while ! kubectl get daemonset calico-node -n kube-system >/dev/null 2>&1 && (( waited < 60 )); do
        sleep 3
        waited=$(( waited + 3 ))
    done

    # Set IP autodetection to use the node IP interface
    kubectl set env daemonset/calico-node -n kube-system \
        IP_AUTODETECTION_METHOD="can-reach=${NODE_IP}" \
        CALICO_IPV4POOL_CIDR="10.42.0.0/16" \
        2>/dev/null || true

    wait_for_pods "kube-system" "k8s-app=calico-node"

    info "Calico CNI installed"
}

# ---------------------------------------------------------------------------
# Step 4: Install OpenEBS LocalPV
# ---------------------------------------------------------------------------
install_openebs() {
    if kubectl get storageclass openebs-hostpath >/dev/null 2>&1; then
        info "OpenEBS already installed — skipping"
        return 0
    fi

    info "Installing OpenEBS LocalPV $OPENEBS_VERSION..."

    local openebs_dir="${DATA_PATH}/openebs/local"
    mkdir -p "$openebs_dir"

    # Create namespace
    kubectl create namespace openebs --dry-run=client -o yaml | kubectl apply -f -

    # Apply OpenEBS LocalPV provisioner
    cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openebs-maya-operator
  namespace: openebs
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
    resources: ["namespaces", "services", "pods", "pods/exec", "deployments",
                "replicationcontrollers", "replicasets", "events", "endpoints",
                "configmaps", "secrets", "jobs", "cronjobs"]
    verbs: ["*"]
  - apiGroups: ["*"]
    resources: ["storageclasses", "persistentvolumeclaims", "persistentvolumes"]
    verbs: ["*"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["csinodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: openebs-maya-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openebs-maya-operator
subjects:
  - kind: ServiceAccount
    name: openebs-maya-operator
    namespace: openebs
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openebs-localpv-provisioner
  namespace: openebs
  labels:
    name: openebs-localpv-provisioner
    openebs.io/component-name: openebs-localpv-provisioner
spec:
  replicas: 1
  selector:
    matchLabels:
      name: openebs-localpv-provisioner
      openebs.io/component-name: openebs-localpv-provisioner
  template:
    metadata:
      labels:
        name: openebs-localpv-provisioner
        openebs.io/component-name: openebs-localpv-provisioner
    spec:
      serviceAccountName: openebs-maya-operator
      containers:
        - name: openebs-provisioner-hostpath
          image: openebs/provisioner-localpv:${OPENEBS_VERSION}
          imagePullPolicy: IfNotPresent
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
            - name: OPENEBS_IO_INSTALLER_TYPE
              value: "openebs-operator"
            - name: OPENEBS_IO_HELPER_IMAGE
              value: "openebs/linux-utils:${OPENEBS_VERSION}"
            - name: OPENEBS_IO_BASE_PATH
              value: "${openebs_dir}"
            - name: OPENEBS_IO_ENABLE_ANALYTICS
              value: "false"
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 256Mi
          livenessProbe:
            exec:
              command:
                - sh
                - -c
                - test -f /tmp/openebs/healthy
            initialDelaySeconds: 30
            periodSeconds: 60
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: openebs-hostpath
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
    openebs.io/cas-type: local
provisioner: openebs.io/local
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
parameters:
  basePath: "${openebs_dir}"
EOF

    wait_for_pods "openebs" "name=openebs-localpv-provisioner"

    info "OpenEBS LocalPV installed — default StorageClass: openebs-hostpath"
}

# ---------------------------------------------------------------------------
# Step 5: Post-install verification
# ---------------------------------------------------------------------------
verify() {
    info "Running post-install verification..."

    local errors=0

    # Check K3s service
    if ! systemctl is-active --quiet k3s; then
        warn "k3s systemd service is not active"
        errors=$(( errors + 1 ))
    fi

    # Check node
    if ! kubectl get node "$NODE_NAME" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q "True"; then
        warn "Node $NODE_NAME is not Ready"
        errors=$(( errors + 1 ))
    fi

    # Check Calico
    local calico_ready
    calico_ready=$(kubectl get pods -n kube-system -l k8s-app=calico-node --no-headers 2>/dev/null \
        | grep -c "Running" || true)
    if [[ "$calico_ready" -eq 0 ]]; then
        warn "No Calico pods running"
        errors=$(( errors + 1 ))
    fi

    # Check OpenEBS
    if ! kubectl get storageclass openebs-hostpath >/dev/null 2>&1; then
        warn "openebs-hostpath StorageClass not found"
        errors=$(( errors + 1 ))
    fi

    # Check CoreDNS
    local dns_ready
    dns_ready=$(kubectl get pods -n kube-system -l k8s-app=kube-dns --no-headers 2>/dev/null \
        | grep -c "Running" || true)
    if [[ "$dns_ready" -eq 0 ]]; then
        warn "CoreDNS not running"
        errors=$(( errors + 1 ))
    fi

    if [[ "$errors" -gt 0 ]]; then
        warn "$errors verification issue(s) found — cluster may still be settling"
    else
        info "All checks passed"
    fi

    # Print summary
    echo ""
    echo "========================================"
    echo "  K3s Cluster Summary"
    echo "========================================"
    echo ""
    echo "  K3s Version:   $K3S_VERSION"
    echo "  Node:          $NODE_NAME ($NODE_IP)"
    echo "  Proxy Mode:    $KUBE_PROXY_MODE"
    echo "  Data Path:     $DATA_PATH"
    echo "  Kubeconfig:    $KUBECONFIG_PATH"
    echo "  StorageClass:  openebs-hostpath (default)"
    echo "  Pod CIDR:      10.42.0.0/16"
    echo "  Service CIDR:  10.43.0.0/16"
    echo "  DNS:           10.43.0.10"
    echo ""
    echo "  Ports:"
    echo "    6443/tcp   — Kubernetes API server"
    echo "    10250/tcp  — Kubelet"
    echo "    10251/tcp  — kube-scheduler"
    echo "    10252/tcp  — kube-controller-manager"
    echo "    179/tcp    — Calico BGP (node-to-node mesh)"
    echo ""
    echo "  Next steps:"
    echo "    export KUBECONFIG=$KUBECONFIG_PATH"
    echo "    kubectl get nodes"
    echo "    kubectl get pods -A"
    echo ""
    echo "========================================"

    return "$errors"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    echo ""
    echo "========================================"
    echo "  Packalares — K3s Cluster Setup"
    echo "========================================"
    echo ""

    preflight
    install_k3s
    wait_for_cluster
    install_calico
    install_openebs

    local verify_errors=0
    verify || verify_errors=$?

    if [[ "$verify_errors" -gt 0 ]]; then
        warn "K3s setup finished with $verify_errors verification warning(s)"
    else
        info "K3s setup complete — all checks passed"
    fi
}

main "$@"
