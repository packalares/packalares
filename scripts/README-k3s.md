# setup-k3s.sh

K3s (lightweight Kubernetes) installer for Packalares. Sets up a single-node cluster with Calico networking, OpenEBS local storage, Redis host service, KubeBlocks database operator, and a Prometheus monitoring stack.

## What it does

1. **Preflight checks** -- validates root access, dependencies, NODE_IP, and loads IPVS kernel modules if needed.
2. **Kernel modules** -- loads and persists `ip_vs`, `ip_vs_rr`, `ip_vs_wrr`, `ip_vs_sh`, `nf_conntrack`, `br_netfilter`, and `overlay` via `/etc/modules-load.d/packalares.conf`.
3. **Installs K3s** from the official `get.k3s.io` script with Traefik, ServiceLB, and built-in local-storage disabled (Packalares uses Caddy, not Traefik; OpenEBS, not K3s local-storage).
4. **Sysctl tuning** -- applies kernel parameter tuning (`somaxconn`, `tcp_tw_reuse`, `tcp_keepalive`, `vm.overcommit_memory`, `fs.file-max`, `vm.max_map_count`, etc.) via `/etc/sysctl.d/99-packalares.conf`.
5. **Waits for cluster readiness** -- API server reachable, node status `Ready`.
6. **Installs Calico CNI** for pod-to-pod networking using the upstream manifest. Configures IP autodetection to match `NODE_IP`.
7. **Installs OpenEBS LocalPV** provisioner with a default `openebs-hostpath` StorageClass backed by `$DATA_PATH/openebs/local`.
8. **Installs Redis** as a host systemd service (not in K8s). Generates a random password saved to `/etc/packalares/redis-password`. Binds to `NODE_IP` and localhost.
9. **Installs KubeBlocks** database operator via Helm. Enables apps to create their own Postgres, MySQL, and MongoDB instances.
10. **Deploys monitoring stack** by calling `setup-monitoring.sh` -- Prometheus, Node Exporter (DaemonSet), kube-state-metrics, and optional ServiceMonitor CRDs.
11. **Verifies** everything: K3s service, node readiness, Calico pods, OpenEBS StorageClass, CoreDNS, sysctl, kernel modules, Redis, KubeBlocks.

The script is **idempotent** -- each step checks whether its component is already installed and skips if so. Safe to re-run.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `NODE_IP` | auto-detected (`hostname -I`) | IPv4 address to bind K3s to |
| `NODE_NAME` | auto-detected (`hostname -s`) | Kubernetes node name |
| `KUBE_PROXY_MODE` | `ipvs` | kube-proxy mode (`ipvs` or `iptables`) |
| `DATA_PATH` | `/packalares/data` | Root data directory for K3s and OpenEBS |
| `K3S_VERSION` | `v1.33.3+k3s1` | K3s release to install |
| `CALICO_VERSION` | `v3.29.2` | Calico release (should match `images.yaml`) |
| `OPENEBS_VERSION` | `3.3.0` | OpenEBS LocalPV provisioner version |
| `REDIS_VERSION` | `7.2` | Redis version (informational; installed via apt) |
| `REDIS_MAXMEMORY` | `256mb` | Redis maxmemory setting |
| `KUBEBLOCKS_VERSION` | `1.0.2` | KubeBlocks Helm chart version |
| `MONITORING_ENABLED` | `true` | Set to `false` to skip monitoring stack |
| `MONITORING_NAMESPACE` | `packalares-monitoring` | Kubernetes namespace for monitoring |
| `PROMETHEUS_VERSION` | `v2.34.0` | Prometheus container image version |
| `NODE_EXPORTER_VERSION` | `v1.8.2` | Node Exporter container image version |
| `KUBE_STATE_METRICS_VERSION` | `v2.10.1` | kube-state-metrics container image version |
| `PROMETHEUS_RETENTION` | `15d` | Prometheus TSDB retention period |
| `PROMETHEUS_STORAGE` | `10Gi` | Prometheus storage allocation |
| `READY_TIMEOUT` | `300` | Seconds to wait for node readiness |
| `POD_TIMEOUT` | `300` | Seconds to wait for pod readiness per component |

## What it creates

### Systemd services

| Service | Description |
|---|---|
| `k3s.service` | K3s server process |
| `redis-server.service` (or `redis.service`) | Redis host cache service |

### Directories

| Path | Purpose |
|---|---|
| `/etc/rancher/k3s/` | K3s config and kubeconfig |
| `/etc/packalares/` | Packalares config (redis-password, etc.) |
| `/etc/redis/` | Redis configuration |
| `/etc/modules-load.d/` | Kernel module persistence |
| `/etc/sysctl.d/` | Sysctl tuning persistence |
| `$DATA_PATH/k3s/` | K3s data (etcd, containerd images, kubelet) |
| `$DATA_PATH/openebs/local/` | OpenEBS hostpath PV backing directory |
| `$DATA_PATH/prometheus/` | Prometheus TSDB data |
| `/root/.kube/config` | Symlink to K3s kubeconfig |

### Kubernetes resources

| Resource | Namespace | Notes |
|---|---|---|
| Calico DaemonSet (`calico-node`) | `kube-system` | Pod networking |
| Calico Deployment (`calico-kube-controllers`) | `kube-system` | Policy controller |
| OpenEBS Deployment (`openebs-localpv-provisioner`) | `openebs` | Local PV provisioner |
| StorageClass `openebs-hostpath` | cluster-scoped | Default StorageClass |
| KubeBlocks operator | `kb-system` | Database operator (Postgres/MySQL/MongoDB) |
| Prometheus Deployment | `packalares-monitoring` | Metrics server |
| Node Exporter DaemonSet | `packalares-monitoring` | Host metrics exporter |
| kube-state-metrics Deployment | `packalares-monitoring` | Kubernetes object metrics |
| ServiceMonitors (optional) | `packalares-monitoring` | Created if prometheus-operator CRD exists |

### Config files

| File | Description |
|---|---|
| `/etc/rancher/k3s/config.yaml` | K3s server configuration |
| `/etc/rancher/k3s/k3s.yaml` | Kubeconfig (generated by K3s) |
| `/etc/redis/redis.conf` | Redis server configuration |
| `/etc/packalares/redis-password` | Redis password (mode 600) |
| `/etc/sysctl.d/99-packalares.conf` | Kernel parameter tuning |
| `/etc/modules-load.d/packalares.conf` | Kernel modules loaded at boot |

## Sysctl tuning

The following kernel parameters are applied after K3s install:

| Parameter | Value | Reason |
|---|---|---|
| `net.core.somaxconn` | `65535` | Increase listen backlog |
| `net.ipv4.ip_local_port_range` | `1024 65535` | Wider ephemeral port range |
| `net.ipv4.tcp_tw_reuse` | `1` | Faster connection recycling |
| `net.ipv4.tcp_fin_timeout` | `10` | Faster FIN-WAIT timeout |
| `net.ipv4.tcp_keepalive_time` | `30` | Aggressive keepalive for containers |
| `net.ipv4.tcp_keepalive_probes` | `3` | Keepalive probe count |
| `net.ipv4.tcp_keepalive_intvl` | `15` | Keepalive probe interval |
| `vm.overcommit_memory` | `1` | Required by Redis and similar |
| `fs.file-max` | `1048576` | System-wide file descriptor limit |
| `vm.max_map_count` | `262144` | Required by Elasticsearch, etc. |

## Kernel modules

Loaded at boot via `/etc/modules-load.d/packalares.conf`:

| Module | Purpose |
|---|---|
| `ip_vs` | IPVS load balancing (kube-proxy) |
| `ip_vs_rr` | IPVS round-robin scheduler |
| `ip_vs_wrr` | IPVS weighted round-robin scheduler |
| `ip_vs_sh` | IPVS source hashing scheduler |
| `nf_conntrack` | Connection tracking |
| `br_netfilter` | Bridge netfilter (Calico/iptables) |
| `overlay` | OverlayFS (containerd) |

## Network configuration

### CIDRs

- Pod CIDR: `10.42.0.0/16`
- Service CIDR: `10.43.0.0/16`
- Cluster DNS: `10.43.0.10`

### Ports opened

| Port | Protocol | Service |
|---|---|---|
| 6443 | TCP | Kubernetes API server |
| 6379 | TCP | Redis (host service, bound to NODE_IP) |
| 9090 | TCP | Prometheus (ClusterIP only) |
| 9100 | TCP | Node Exporter (hostPort) |
| 10250 | TCP | Kubelet API |
| 10251 | TCP | kube-scheduler |
| 10252 | TCP | kube-controller-manager |
| 179 | TCP | Calico BGP (node-to-node mesh) |

## Integration with other Packalares modules

This script produces `KUBECONFIG=/etc/rancher/k3s/k3s.yaml`. Downstream modules should set:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
```

Redis password can be read by other modules:

```bash
REDIS_PASSWORD=$(cat /etc/packalares/redis-password)
```

Modules that depend on this script:

- **Caddy proxy** -- reads KUBECONFIG to discover service endpoints
- **Auth service** -- deploys into the cluster this script creates
- **App service** -- uses `openebs-hostpath` StorageClass for app data; reads Redis password for caching
- **Dashboard frontend** -- connects to the API server on port 6443
- **Database workloads** -- use KubeBlocks to create Postgres/MySQL/MongoDB instances

## Usage

```bash
# Minimal (auto-detect everything)
sudo ./scripts/setup-k3s.sh

# Explicit configuration
sudo NODE_IP=192.168.1.100 NODE_NAME=olares DATA_PATH=/mnt/ssd/packalares ./scripts/setup-k3s.sh

# Override K3s version
sudo K3S_VERSION=v1.33.3+k3s1 ./scripts/setup-k3s.sh

# Skip monitoring stack
sudo MONITORING_ENABLED=false ./scripts/setup-k3s.sh

# Run monitoring stack separately
sudo ./scripts/setup-monitoring.sh
```

## Detecting existing installation

The script checks for an existing install at each step. To test whether components are already set up from another script:

```bash
# K3s installed and node ready?
k3s kubectl get node "$(hostname -s)" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True

# Calico running?
kubectl get daemonset calico-node -n kube-system >/dev/null 2>&1

# OpenEBS StorageClass present?
kubectl get storageclass openebs-hostpath >/dev/null 2>&1

# Redis running?
systemctl is-active --quiet redis-server 2>/dev/null || systemctl is-active --quiet redis 2>/dev/null

# KubeBlocks installed?
kubectl get deployment -n kb-system kubeblocks >/dev/null 2>&1

# Prometheus running?
kubectl get deployment prometheus-server -n packalares-monitoring >/dev/null 2>&1

# Sysctl tuning applied?
test -f /etc/sysctl.d/99-packalares.conf

# Kernel modules persisted?
test -f /etc/modules-load.d/packalares.conf
```

## setup-monitoring.sh

The monitoring stack is complex enough to have its own script at `scripts/setup-monitoring.sh`. It can be run standalone or is called automatically by `setup-k3s.sh` when `MONITORING_ENABLED=true` (the default).

It deploys:

- **Prometheus** (`prom/prometheus`) -- metrics collection and storage, with Kubernetes service discovery
- **Node Exporter** (`prom/node-exporter`) -- DaemonSet that exposes host-level metrics (CPU, memory, disk, network)
- **kube-state-metrics** (`registry.k8s.io/kube-state-metrics/kube-state-metrics`) -- exposes Kubernetes object state as Prometheus metrics
- **ServiceMonitor CRDs** -- created automatically if `prometheus-operator` is detected (the CRD `servicemonitors.monitoring.coreos.com` exists)

See environment variables above for version and configuration overrides.
