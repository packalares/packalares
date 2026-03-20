#!/bin/bash
# setup-k3s.sh — Install K3s, Calico CNI, OpenEBS, Redis, KubeBlocks for Packalares
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

# Redis
REDIS_VERSION="${REDIS_VERSION:-7.2}"
REDIS_MAXMEMORY="${REDIS_MAXMEMORY:-256mb}"

# KubeBlocks
KUBEBLOCKS_VERSION="${KUBEBLOCKS_VERSION:-1.0.2}"

# Monitoring (deployed via setup-monitoring.sh, called from here)
MONITORING_ENABLED="${MONITORING_ENABLED:-true}"

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
# Step 5: Kernel modules — load and persist for boot
# ---------------------------------------------------------------------------
setup_kernel_modules() {
    local conf="/etc/modules-load.d/packalares.conf"
    local modules=(ip_vs ip_vs_rr ip_vs_wrr ip_vs_sh nf_conntrack br_netfilter overlay)

    if [[ -f "$conf" ]]; then
        # Check whether all modules are already listed
        local missing=0
        for mod in "${modules[@]}"; do
            if ! grep -qw "$mod" "$conf" 2>/dev/null; then
                missing=1
                break
            fi
        done
        if [[ "$missing" -eq 0 ]]; then
            info "Kernel modules already configured in $conf — skipping"
            return 0
        fi
    fi

    info "Configuring kernel modules for persistence..."

    mkdir -p /etc/modules-load.d

    cat > "$conf" <<EOF
# Packalares — kernel modules loaded at boot
# IPVS modules for kube-proxy
ip_vs
ip_vs_rr
ip_vs_wrr
ip_vs_sh
# Connection tracking
nf_conntrack
# Bridge netfilter (required for Calico/iptables)
br_netfilter
# OverlayFS (required for containerd)
overlay
EOF

    # Load them now
    for mod in "${modules[@]}"; do
        if ! lsmod | grep -qw "$mod" 2>/dev/null; then
            modprobe "$mod" 2>/dev/null || warn "Could not load kernel module: $mod"
        fi
    done

    info "Kernel modules configured and loaded"
}

# ---------------------------------------------------------------------------
# Step 6: Sysctl tuning
# ---------------------------------------------------------------------------
setup_sysctl() {
    local conf="/etc/sysctl.d/99-packalares.conf"

    if [[ -f "$conf" ]]; then
        info "Sysctl tuning already applied ($conf exists) — skipping"
        return 0
    fi

    info "Applying sysctl tuning..."

    mkdir -p /etc/sysctl.d

    cat > "$conf" <<EOF
# Packalares — kernel parameter tuning for K3s workloads

# Network: increase listen backlog and port range
net.core.somaxconn = 65535
net.ipv4.ip_local_port_range = 1024 65535

# Network: faster connection recycling
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 10

# Network: aggressive keepalive for container connections
net.ipv4.tcp_keepalive_time = 30
net.ipv4.tcp_keepalive_probes = 3
net.ipv4.tcp_keepalive_intvl = 15

# Memory: allow overcommit (required by Redis and similar)
vm.overcommit_memory = 1

# File descriptors: raise system-wide limit
fs.file-max = 1048576

# Virtual memory: raise mmap count (required by Elasticsearch, etc.)
vm.max_map_count = 262144
EOF

    sysctl --system >/dev/null 2>&1 || warn "sysctl --system returned non-zero"

    info "Sysctl tuning applied"
}

# ---------------------------------------------------------------------------
# Step 7: Redis host service
# ---------------------------------------------------------------------------
install_redis() {
    if systemctl is-active --quiet redis-server 2>/dev/null || systemctl is-active --quiet redis 2>/dev/null; then
        info "Redis already running — skipping"
        return 0
    fi

    info "Installing Redis as a host service..."

    # Install via apt if available, otherwise bail with guidance
    if command -v apt-get >/dev/null 2>&1; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update -qq
        apt-get install -y -qq redis-server >/dev/null 2>&1
    else
        die "apt-get not found — Redis install requires Debian/Ubuntu"
    fi

    # Generate a random password
    local password
    password=$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32)

    # Save password for other Packalares modules
    mkdir -p /etc/packalares
    echo "$password" > /etc/packalares/redis-password
    chmod 600 /etc/packalares/redis-password

    # Write config
    mkdir -p /etc/redis

    cat > /etc/redis/redis.conf <<EOF
# Packalares Redis configuration
bind ${NODE_IP} 127.0.0.1
port 6379
protected-mode yes
requirepass ${password}

# Memory
maxmemory ${REDIS_MAXMEMORY}
maxmemory-policy allkeys-lru

# Persistence (AOF for durability)
appendonly yes
appendfilename "appendonly.aof"
dir /var/lib/redis

# Logging
loglevel notice
logfile /var/log/redis/redis-server.log

# Security
rename-command FLUSHDB ""
rename-command FLUSHALL ""
rename-command DEBUG ""

# Performance
tcp-backlog 511
timeout 0
tcp-keepalive 300
EOF

    # Ensure directories exist
    mkdir -p /var/lib/redis /var/log/redis
    chown redis:redis /var/lib/redis /var/log/redis 2>/dev/null || true

    # Create systemd override to use our config if the package unit exists
    if [[ -f /lib/systemd/system/redis-server.service ]]; then
        mkdir -p /etc/systemd/system/redis-server.service.d
        cat > /etc/systemd/system/redis-server.service.d/packalares.conf <<EOF
[Service]
ExecStart=
ExecStart=/usr/bin/redis-server /etc/redis/redis.conf
EOF
        systemctl daemon-reload
        systemctl enable --now redis-server
    else
        # Create the service from scratch
        cat > /etc/systemd/system/redis.service <<EOF
[Unit]
Description=Redis — Packalares host cache
After=network.target

[Service]
Type=notify
ExecStart=/usr/bin/redis-server /etc/redis/redis.conf
ExecStop=/usr/bin/redis-cli -a ${password} shutdown
Restart=always
RestartSec=5
User=redis
Group=redis
RuntimeDirectory=redis
RuntimeDirectoryMode=0755
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
        # Ensure redis user exists
        id -u redis >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin redis
        chown redis:redis /var/lib/redis /var/log/redis
        systemctl daemon-reload
        systemctl enable --now redis
    fi

    info "Redis installed — password stored at /etc/packalares/redis-password"
}

# ---------------------------------------------------------------------------
# Step 8: KubeBlocks database operator
# ---------------------------------------------------------------------------
install_kubeblocks() {
    if kubectl get deployment -n kb-system kubeblocks >/dev/null 2>&1; then
        info "KubeBlocks already installed — skipping"
        return 0
    fi

    info "Installing KubeBlocks $KUBEBLOCKS_VERSION..."

    # Install Helm if not present (KubeBlocks is deployed via Helm)
    if ! command -v helm >/dev/null 2>&1; then
        info "Helm not found — installing..."
        curl -sfL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
    fi

    # Add the KubeBlocks Helm repo
    helm repo add kubeblocks https://apecloud.github.io/helm-charts 2>/dev/null || true
    helm repo update kubeblocks

    # Install KubeBlocks operator
    helm upgrade --install kubeblocks kubeblocks/kubeblocks \
        --namespace kb-system \
        --create-namespace \
        --version "$KUBEBLOCKS_VERSION" \
        --set image.registry=docker.io \
        --wait \
        --timeout 600s

    wait_for_pods "kb-system"

    info "KubeBlocks $KUBEBLOCKS_VERSION installed — apps can create Postgres/MySQL/MongoDB instances"
}

# ---------------------------------------------------------------------------
# Step 9: Monitoring stack (delegated to setup-monitoring.sh)
# ---------------------------------------------------------------------------
install_monitoring() {
    if [[ "$MONITORING_ENABLED" != "true" ]]; then
        info "Monitoring disabled (MONITORING_ENABLED=$MONITORING_ENABLED) — skipping"
        return 0
    fi

    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local monitoring_script="${script_dir}/setup-monitoring.sh"

    if [[ ! -f "$monitoring_script" ]]; then
        warn "Monitoring script not found at $monitoring_script — skipping"
        return 0
    fi

    info "Deploying monitoring stack..."
    bash "$monitoring_script"
    info "Monitoring stack deployment complete"
}

# ---------------------------------------------------------------------------
# Step 10: Post-install verification
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

    # Check sysctl tuning
    if [[ ! -f /etc/sysctl.d/99-packalares.conf ]]; then
        warn "Sysctl tuning file not found"
        errors=$(( errors + 1 ))
    fi

    # Check kernel modules persistence
    if [[ ! -f /etc/modules-load.d/packalares.conf ]]; then
        warn "Kernel modules persistence file not found"
        errors=$(( errors + 1 ))
    fi

    # Check Redis
    if systemctl is-active --quiet redis-server 2>/dev/null || systemctl is-active --quiet redis 2>/dev/null; then
        : # Redis OK
    else
        warn "Redis host service is not running"
        errors=$(( errors + 1 ))
    fi

    # Check KubeBlocks
    if ! kubectl get deployment -n kb-system kubeblocks >/dev/null 2>&1; then
        warn "KubeBlocks operator not found"
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
    echo "  K3s Version:     $K3S_VERSION"
    echo "  Node:            $NODE_NAME ($NODE_IP)"
    echo "  Proxy Mode:      $KUBE_PROXY_MODE"
    echo "  Data Path:       $DATA_PATH"
    echo "  Kubeconfig:      $KUBECONFIG_PATH"
    echo "  StorageClass:    openebs-hostpath (default)"
    echo "  Pod CIDR:        10.42.0.0/16"
    echo "  Service CIDR:    10.43.0.0/16"
    echo "  DNS:             10.43.0.10"
    echo "  KubeBlocks:      $KUBEBLOCKS_VERSION"
    echo "  Redis:           host service (port 6379)"
    echo "  Redis password:  /etc/packalares/redis-password"
    echo "  Monitoring:      $MONITORING_ENABLED"
    echo ""
    echo "  Ports:"
    echo "    6443/tcp   — Kubernetes API server"
    echo "    6379/tcp   — Redis (host service)"
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
    setup_kernel_modules
    install_k3s
    setup_sysctl
    wait_for_cluster
    install_calico
    install_openebs
    install_redis
    install_kubeblocks
    install_monitoring

    local verify_errors=0
    verify || verify_errors=$?

    if [[ "$verify_errors" -gt 0 ]]; then
        warn "K3s setup finished with $verify_errors verification warning(s)"
    else
        info "K3s setup complete — all checks passed"
    fi
}

main "$@"
