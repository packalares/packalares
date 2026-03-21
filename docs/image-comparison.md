# Olares vs Packalares Image Comparison

## Gateway & Proxy

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/bfl:v0.4.42` | API gateway, user info, activation | `packalares/bfl` | Replaced |
| `beclab/bfl-ingress:v0.3.30` | nginx ingress, TLS, app routing | Merged into `packalares/bfl` | Replaced |
| `beclab/l4-bfl-proxy:v0.3.12` | L4 TLS SNI router (nginx+Lua) | `packalares/l4proxy` | Replaced (pure Go) |

## Auth

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/auth:0.2.49` | Authelia fork, auth engine | `packalares/auth` | Replaced |
| `beclab/login:v1.9.22` | Login page frontend | Merged into `packalares/auth` | Replaced |

## App Management

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/app-service:0.5.8` | App install/uninstall, lifecycle | `packalares/appservice` | Replaced |
| `beclab/image-service:0.4.65` | App icon processing | Merged into `packalares/appservice` | Replaced |
| `beclab/market-backend:v0.6.22` | Marketplace API | `packalares/market` | Replaced |
| `beclab/dynamic-chart-repository:v0.3.10` | Helm chart repo | Not needed — apps install direct | Removed |

## System

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/system-server:0.1.33` | System API, nginx config gen | `packalares/systemserver` | Replaced |
| `beclab/user-service:v0.0.87` | User data management | Merged into `packalares/bfl` | Replaced |
| `beclab/provider-proxy:0.1.1` | nginx proxy for system services | Not needed — BFL handles routing | Removed |
| `beclab/integration-server:0.0.3` | External integrations | Not needed yet | Removed |
| `beclab/monitoring-server-v1:v0.3.9` | Monitoring dashboard API | `packalares/monitor` | Replaced |
| `beclab/osnode-init:v0.0.11` | Node init DaemonSet | Handled by CLI installer | Removed |
| `beclab/upgrade-job:0.1.7` | System upgrades | Handled by CLI `packalares upgrade` | Removed |

## Frontend

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/system-frontend:v1.9.24` | Desktop/settings/market/files UI | `packalares/desktop` | Replaced |
| `beclab/wizard:v1.6.40` | Setup wizard | Included in `packalares/desktop` | Replaced |
| `beclab/docker-nginx-headers-more` | nginx for frontend | Not needed — desktop uses standard nginx | Removed |

## Files

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/files-server:v0.2.152` | File manager API (closed source) | `packalares/files` | Replaced |
| `beclab/rclone:v1.73.0` | Remote storage mounts | `packalares/mounts` | Replaced |
| `beclab/samba-server:0.0.8` | SMB file sharing | `packalares/samba` | Replaced |
| `beclab/download-server:v0.1.22` | Download manager (closed source) | Not needed — files handles downloads | Removed |
| `beclab/aria2:v0.0.4` | Download engine | Not needed | Removed |
| `beclab/images-uploader:0.2.3` | Image upload handler | Merged into `packalares/files` | Replaced |
| `beclab/seahub-init:v0.0.7` | Seafile init | Not needed — using simple file sync | Removed |
| `beclab/pg_seafile_server:v0.0.21` | Seafile server | Not needed | Removed |

## Middleware

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/middleware-operator:0.2.32` | Provisions DB/Redis/NATS for apps | `packalares/middleware` | Replaced |
| `beclab/s3rver:latest` | Fake S3 storage | Not needed — using direct storage | Removed |
| `beclab/sys-event:0.2.16` | System event handler | Merged into `packalares/systemserver` | Replaced |

## Secrets

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/infisical:0.1.2` | Secrets management | Not needed — K8s Secrets + auth | Removed |
| `beclab/secret-vault:0.1.15` | Secret vault sidecar | Not needed | Removed |

## Backup

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/velero:v1.11.3` | K8s backup | Upstream `velero/velero` when needed | Future |
| `beclab/velero-plugin-for-terminus:v1.0.2` | Custom backup plugin | Not needed | Removed |
| `beclab/backup-server:v0.3.62` | Backup UI/API | Future addition | Not yet |

## File Sync

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/fsnotify-daemon:0.1.4` | File change watcher | Merged into `packalares/files` | Replaced |
| `beclab/fsnotify-proxy:0.1.11` | File change proxy | Not needed | Removed |

## KubeSphere

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/ks-apiserver:0.0.24` | User CRD API server | `packalares/kubesphere` | Replaced |

## Monitoring

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/kube-state-metrics:v2.3.0-ext.1` | K8s metrics | Upstream `kube-state-metrics` | Upstream |
| `beclab/node-exporter:0.0.5` | Host metrics | Upstream `prom/node-exporter` | Upstream |
| `beclab/kube-rbac-proxy:0.19.0` | Metrics auth proxy | Not needed | Removed |

## GPU

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/hami:v2.6.13` | GPU scheduler | Upstream HAMi when needed | Future |
| `beclab/hami-webui-fe-oss:v1.0.8` | GPU monitoring UI | Merged into desktop settings | Future |
| `beclab/hami-webui-be-oss:v1.0.8` | GPU monitoring API | Merged into `packalares/monitor` | Future |
| `beclab/dcgm-exporter:4.2.3` | NVIDIA metrics | Upstream when needed | Future |
| `beclab/gpu-scheduler:v0.1.1` | GPU scheduling | Upstream HAMi when needed | Future |

## Removed (cloud/LarePass)

| Olares Image | What it does | Status |
|---|---|---|
| Vault server + admin (2) | LarePass password manager | Removed |
| Headscale + init + wrapper (3) | Built-in VPN | Removed (use Tailscale) |
| Search3 + monitor + validation (3) | Search service | Removed |
| Notifications API (1) | Push to LarePass | Removed |
| OpenTelemetry (3) | Tracing stack | Removed |

## Misc

| Olares Image | What it does | Packalares | Status |
|---|---|---|---|
| `beclab/reverse-proxy:v0.1.10` | FRP tunnel agent | — | Removed (use Tailscale) |
| `beclab/openssl:v3` | Cert generation init | Handled by CLI/BFL | Removed |
| `beclab/alpine:3.14` | Init container | Not needed | Removed |
| `bytetrade/envoy:v1.25.11` | Sidecar proxy | Not needed — direct routing | Removed |
| `openservicemesh/init:v1.2.3` | iptables init | Not needed | Removed |
| `owncloudci/wait-for:latest` | Wait for service | Not needed | Removed |
| `busybox:1.28` | Init container | Not needed | Removed |

## Upstream (unchanged)

| Image | What it does |
|---|---|
| `postgres:16-alpine` | Database |
| `nats:2.10-alpine` | Messaging |
| `lldap/lldap:v0.6.1` | User directory |
| `calico/*` (3) | Networking |
| `registry.k8s.io/coredns` | DNS (K3s built-in) |
| `openebs/*` (2) | Storage |
| `prom/prometheus` | Metrics DB |
| `prom/node-exporter` | Host metrics |
| `nginx:stable-alpine` | Frontend server |

## Summary

- **Olares: 97 images** (40 beclab, 30 upstream, 27 misc)
- **Packalares: 28 images** (13 ours, 15 upstream)
