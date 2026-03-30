# Packalares

[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Kubernetes](https://img.shields.io/badge/K3s-v1.29-326CE5?logo=kubernetes&logoColor=white)](https://k3s.io)
[![License](https://img.shields.io/badge/License-Elastic%202.0-blue)](LICENSE)
[![Version](https://img.shields.io/badge/Version-1.0.0-green)](https://github.com/packalares/packalares/releases)

A self-hosted personal cloud operating system. Packalares is a fork of [Olares](https://github.com/beclab/olares) with all cloud dependencies stripped out -- no account registration, no phone verification, no external auth services. Everything runs on your hardware, under your control.

**Key features:**

- **170+ self-contained apps** installable from a built-in marketplace
- **AI model management** with Ollama and vLLM support, including GPU sharing via HAMi
- **Tailscale VPN** integration for secure remote access through your own control plane
- **Self-signed CA** with automatic TLS certificate generation for all services
- **Single-node Kubernetes** (K3s) with full observability stack (Prometheus, Loki, Grafana)

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/packalares/packalares/main/install.sh | sudo bash
```

Or with options:

```bash
curl -fsSL https://raw.githubusercontent.com/packalares/packalares/main/install.sh | \
  sudo bash -s -- --username admin --domain mydomain.com
```

Environment variables for unattended install:

| Variable | Default | Description |
|----------|---------|-------------|
| `PACKALARES_USERNAME` | `admin` | Admin username |
| `PACKALARES_PASSWORD` | (generated) | Admin password |
| `PACKALARES_DOMAIN` | `olares.local` | Base domain |
| `PACKALARES_BASE_DIR` | `/opt/packalares` | Installation root |
| `OLARES_TAILSCALE_AUTH_KEY` | (none) | Tailscale auth key |
| `OLARES_TAILSCALE_CONTROL_URL` | (none) | Headscale/Tailscale control URL |
| `OLARES_CERT_MODE` | `local` | `local` (self-signed) or `acme` |

---

## Architecture

### System Overview

```
+------------------------------------------------------------------+
|  Host (Ubuntu 22.04/24.04)                                       |
|                                                                   |
|  +-- containerd --------+  +-- etcd --+  +-- KVRocks (Redis) --+ |
|  |  container runtime   |  |  K3s DB  |  |  sessions, cache    | |
|  +----------------------+  +----------+  +---------------------+ |
|                                                                   |
|  +-- K3s (Kubernetes) -------------------------------------------+|
|  |                                                                ||
|  |  Namespace: os-system (Platform)                               ||
|  |  +----------+ +-------+ +-------+ +-----------+ +----------+  ||
|  |  | Citus    | | NATS  | | LLDAP | | Infisical | | KVRocks  |  ||
|  |  | Postgres | | msg   | | LDAP  | | + TAPR    | | (proxy)  |  ||
|  |  +----------+ +-------+ +-------+ +-----------+ +----------+  ||
|  |                                                                ||
|  |  Namespace: os-framework (Services)                            ||
|  |  +------+ +-----+ +--------+ +--------+ +----------+          ||
|  |  | auth | | bfl | | app-   | | system | | middle-  |          ||
|  |  |      | |     | | service| | server | | ware     |          ||
|  |  +------+ +-----+ +--------+ +--------+ +----------+          ||
|  |  +-------+ +--------+ +---------+ +-------+ +--------+        ||
|  |  | files | | market | | monitor | | mounts| | hostctl|        ||
|  |  +-------+ +--------+ +---------+ +-------+ +--------+        ||
|  |  +-------+ +------+ +-----------+ +-------+                   ||
|  |  | samba | | mdns | | kubesphere| | proxy |                   ||
|  |  +-------+ +------+ +-----------+ +-------+                   ||
|  |                                                                ||
|  |  Namespace: user-space-<username>                              ||
|  |  +---------+ +--------+ +------+ +------+ +------+            ||
|  |  | desktop | | wizard | | app1 | | app2 | | ...  |            ||
|  |  +---------+ +--------+ +------+ +------+ +------+            ||
|  |                                                                ||
|  |  Namespace: monitoring                                         ||
|  |  +------------+ +---------------+ +------+ +---------+        ||
|  |  | prometheus | | kube-state-   | | loki | | promtail|        ||
|  |  |            | | metrics       | |      | |         |        ||
|  |  +------------+ +---------------+ +------+ +---------+        ||
|  |                                                                ||
|  |  Namespace: hami-system (if GPU detected)                      ||
|  |  +------------------+ +----------------+                       ||
|  |  | hami-device-     | | hami-scheduler |                      ||
|  |  | plugin           | |                |                       ||
|  |  +------------------+ +----------------+                       ||
|  +----------------------------------------------------------------+|
+-------------------------------------------------------------------+
```

### Request Flow

```
Browser (HTTPS)
    |
    v
+-- nginx reverse proxy (hostPort 443) --------------------------+
|                                                                  |
|   1. TLS termination (self-signed CA or ACME cert)               |
|   2. auth_request to /_auth  -->  auth-backend:9091/api/verify   |
|   3. Route by subdomain:                                         |
|      desktop.<zone>  -->  desktop-svc (user namespace)           |
|      market.<zone>   -->  market-backend:6756                    |
|      api.<zone>      -->  bfl:3002                               |
|      files.<zone>    -->  files-server:8080                      |
|      <app>.<zone>    -->  system-server generates nginx config   |
|                           per app, routes to app pod             |
+------------------------------------------------------------------+
```

### Authentication Flow

```
Browser                  nginx                auth-backend          LLDAP        KVRocks
   |                       |                       |                  |             |
   |-- POST /api/login --> |                       |                  |             |
   |                       |-- /api/firstfactor -->|                  |             |
   |                       |                       |-- rate check --> |             |
   |                       |                       |                  |             |
   |                       |                       |-- LDAP bind ---->|             |
   |                       |                       |                  |             |
   |                       |                       |-- store session ------------>  |
   |                       |                       |                               |
   |<---- session cookie --|<-- JWT + cookie ------|                               |
   |                       |                       |                               |
   |-- GET /app (cookie)-->|                       |                               |
   |                       |-- /_auth subrequest ->|                               |
   |                       |                       |-- verify session -----------> |
   |                       |<---- 200 OK ----------|                               |
   |<---- app content -----|                       |                               |
```

If TOTP is enabled, the first-factor response returns `requires_totp=true`. The browser then submits the TOTP code to `/api/secondfactor/totp`, which upgrades the session to `auth_level=2`.

### App Install Flow

```
Desktop UI                  app-service:6755          market-backend        Helm / K8s
     |                            |                        |                    |
     |-- POST /api/apps/install ->|                        |                    |
     |                            |-- GET charts/index --> |                    |
     |                            |<-- chart .tgz ---------|                    |
     |                            |                                             |
     |                            |-- parse OlaresManifest.yaml                 |
     |                            |-- rewrite chart values (ns, owner, zone)    |
     |                            |-- create Application CRD -----------------> |
     |                            |-- helm install from chart dir ------------> |
     |                            |                                             |
     |                            |   system-server watches Application CRDs    |
     |                            |   --> generates nginx config for subdomain  |
     |                            |   --> app reachable at <app>.<zone>         |
     |<--- install complete ------|                                             |
```

### Model Install Flow

```
Desktop UI                  app-service:6755                 K8s
     |                            |                           |
     |-- POST /api/models/install |                           |
     |                            |                           |
     |  Ollama models:            |                           |
     |    HTTP API to ollama pod  |                           |
     |    (installed as app)      |                           |
     |                            |                           |
     |  vLLM models:              |                           |
     |    helm install from       |                           |
     |    deploy/charts/vllm-model|                           |
     |      +-- hf-downloader     |-- init container ------->|
     |      |   (HuggingFace)     |                           |
     |      +-- vLLM container    |-- OpenAI-compat API ---->|
     |      +-- HAMi scheduler    |-- GPU slice allocation ->|
     |                            |                           |
```

---

## Components

### Platform Services (os-system)

| Service | Image | Purpose |
|---------|-------|---------|
| Citus | `ghcr.io/packalares/citus:12.1` | Distributed PostgreSQL database |
| NATS | `ghcr.io/packalares/nats:2.10-alpine` | Message bus (JetStream) |
| LLDAP | `ghcr.io/packalares/lldap:v0.5.0` | Lightweight LDAP directory |
| Infisical | `ghcr.io/packalares/infisical:v0.158.20` | Secrets management platform |
| TAPR | `ghcr.io/packalares/tapr` | Infisical API proxy / secrets vault |
| KVRocks | Host systemd service | Redis-compatible key-value store (sessions, cache) |

### Framework Services (os-framework)

| Service | Image | Purpose |
|---------|-------|---------|
| auth | `ghcr.io/packalares/auth` | Authentication (LLDAP bind, sessions, TOTP) |
| bfl | `ghcr.io/packalares/bfl` | Backend For Launcher (user info, settings, zone) |
| app-service | `ghcr.io/packalares/appservice` | App lifecycle (install, upgrade, uninstall) |
| system-server | `ghcr.io/packalares/systemserver` | App routing, nginx config gen, envoy sidecars |
| middleware | `ghcr.io/packalares/middleware` | Middleware operator (provisions DB/cache for apps) |
| files | `ghcr.io/packalares/files` | File manager (WebDAV, upload/download) |
| market | `ghcr.io/packalares/market` | App marketplace backend (catalog, chart sync) |
| monitor | `ghcr.io/packalares/monitor` | Metrics aggregation and system status |
| mounts | `ghcr.io/packalares/mounts` | Network mount manager (SMB, NFS, rclone) |
| kubesphere | `ghcr.io/packalares/kubesphere` | KubeSphere CRD compatibility layer |
| samba | `ghcr.io/packalares/samba` | SMB file sharing server |
| mdns | `ghcr.io/packalares/mdns` | mDNS LAN discovery agent |
| hostctl | `ghcr.io/packalares/hostctl` | Host control agent (SSH, sysctl, services) |
| tailscale | `ghcr.io/packalares/tailscale:stable` | VPN client (if configured) |
| proxy (nginx) | `ghcr.io/packalares/nginx:stable-alpine` | Reverse proxy for all HTTP/HTTPS traffic |

### Monitoring (monitoring)

| Service | Image | Purpose |
|---------|-------|---------|
| Prometheus | `ghcr.io/packalares/prometheus:v2.51.0` | Metrics collection |
| Node Exporter | `ghcr.io/packalares/node-exporter:v1.7.0` | Host metrics (DaemonSet) |
| Kube State Metrics | `ghcr.io/packalares/kube-state-metrics:v2.11.0` | Kubernetes resource metrics |
| Loki | `ghcr.io/packalares/loki:2.9.4` | Log aggregation |
| Promtail | `ghcr.io/packalares/promtail:2.9.4` | Log shipping |
| DCGM Exporter | `ghcr.io/packalares/dcgm-exporter:4.5.2` | GPU metrics (if NVIDIA detected) |

### User Apps (user-space-\<username\>)

| Service | Image | Purpose |
|---------|-------|---------|
| Desktop | `ghcr.io/packalares/desktop` | Quasar Vue 3 SPA (desktop, login, settings) |
| Wizard | `ghcr.io/packalares/desktop` | First-run setup wizard |
| (installed apps) | varies | User-installed applications from marketplace |

### GPU Management (hami-system, conditional)

| Service | Image | Purpose |
|---------|-------|---------|
| HAMi Device Plugin | `ghcr.io/packalares/hami:v2.4.1` | GPU device plugin (DaemonSet) |
| HAMi Scheduler | `ghcr.io/packalares/hami:v2.4.1` | GPU slice allocation scheduler |

---

## Installation

### Prerequisites

- **OS:** Ubuntu 22.04 or 24.04 (amd64 or arm64)
- **CPU:** 4+ cores recommended
- **RAM:** 8 GB minimum, 16 GB recommended
- **Disk:** 60 GB minimum free space
- **Network:** Ports 80 and 443 available
- **Root access** required

### Install

```bash
curl -fsSL https://raw.githubusercontent.com/packalares/packalares/main/install.sh | sudo bash
```

The installer runs 15 phases automatically:

1. System precheck
2. Download binaries (K3s, containerd, etcd, helm)
3. Install containerd
4. Install etcd with TLS certificates
5. Install K3s (external etcd backend)
6. Deploy Calico CNI
7. Deploy OpenEBS storage
8. Install KVRocks (host systemd service)
9. Configure kernel modules and sysctl
10. Deploy CRDs and namespaces
11. Deploy platform charts (Citus, NATS, LLDAP, Infisical)
12. Deploy framework charts (auth, bfl, app-service, system-server, etc.)
13. Deploy app charts (desktop, wizard)
14. Deploy monitoring (Prometheus, Loki, Promtail)
15. Wait for all pods to be ready

All manifests are embedded in the CLI binary via `deploy/embed.go`. Config placeholders (`{{VARIABLE}}`) are replaced at deploy time.

### Post-Install Access

After installation completes, access the system at:

```
https://desktop.<username>.<domain>
```

Default: `https://desktop.admin.olares.local`

The wizard will guide you through initial setup (password, optional TOTP, Tailscale).

### GPU Setup

GPU support is automatic. If an NVIDIA GPU is detected during installation:

- HAMi device plugin and scheduler are deployed to the `hami-system` namespace
- DCGM Exporter is deployed for GPU metrics
- GPU sharing between apps is enabled by default (configurable via `gpu.sharing` in config)

To force-enable or disable:

```yaml
# /etc/packalares/config.yaml
gpu:
  enabled: true   # auto (default), true, or false
  sharing: true   # HAMi GPU sharing between apps
```

---

## App Marketplace

The built-in marketplace ships with **170 apps** and **4 model cards** (174 total entries).

### Installing Apps

From the Desktop UI, open the Market app. Browse or search for an app, then click Install. Behind the scenes:

1. `app-service` downloads the Helm chart from `market-backend`
2. The chart's `OlaresManifest.yaml` is parsed for metadata
3. Chart values are rewritten with namespace, owner, and zone info
4. An `Application` CRD is created
5. Helm installs the chart into the user namespace
6. `system-server` generates nginx config for the app's subdomain
7. The app becomes accessible at `https://<appname>.<username>.<domain>`

### Uninstalling Apps

From the Desktop UI, right-click an app and select Uninstall. The `app-service` runs `helm uninstall` and cleans up the `Application` CRD.

### AI Models

Two model runtimes are supported:

| Runtime | How it works | GPU required |
|---------|-------------|--------------|
| **Ollama** | Installed as a regular app. Models pulled via Ollama's HTTP API. | Recommended |
| **vLLM** | Deployed via `deploy/charts/vllm-model/`. An `hf-downloader` init container fetches the model from HuggingFace. Serves an OpenAI-compatible API. HAMi allocates GPU slices. | Yes |

Apps that need LLM access (chat UIs, agents) connect to the Ollama service within the cluster via its internal service DNS name.

---

## Security

### TLS Certificates

- A self-signed CA is generated during installation
- Wildcard certificates are auto-generated for `*.<username>.<domain>`
- Certificates are stored in the `zone-tls` Kubernetes Secret
- All external traffic is HTTPS with automatic HTTP-to-HTTPS redirect
- ACME (Let's Encrypt) mode available via `OLARES_CERT_MODE=acme`
- Certificates are auto-regenerated on domain or network changes

### Authentication

- **LLDAP** provides the user directory (lightweight LDAP)
- **Session cookies**: HttpOnly, Secure, HMAC-SHA256 signed
- **JWT tokens**: HS512 signed, issued after successful login
- **Rate limiting**: Exponential backoff, 1-hour block after 10 failed attempts

### Two-Factor Authentication (TOTP)

- Optional TOTP setup via the Settings page
- Standard 30-second TOTP codes (SHA1-HMAC)
- After first-factor login, session is upgraded to `auth_level=2` upon TOTP verification
- All protected routes check auth level when TOTP is configured

### Request Authentication

Every request to a protected route goes through nginx `auth_request`:

1. nginx sends a subrequest to `auth-backend/api/verify`
2. Auth service checks the session cookie against KVRocks
3. If TOTP is required, verifies `auth_level >= 2`
4. Returns 200 (allow) or 401 (deny)
5. Sets `Remote-User` and `Remote-Groups` headers for downstream services

---

## Network

### Tailscale VPN

Packalares integrates with Tailscale (or Headscale) for secure remote access:

```bash
# During install:
OLARES_TAILSCALE_AUTH_KEY=tskey-auth-xxx \
OLARES_TAILSCALE_CONTROL_URL=https://headscale.example.com \
  curl -fsSL .../install.sh | sudo bash

# Or configure after install in /etc/packalares/config.yaml:
network:
  tailscale:
    enabled: true
    auth_key: "tskey-auth-xxx"
    hostname: packalares
```

The Tailscale pod runs in `os-framework` with `NET_ADMIN` and `NET_RAW` capabilities.

### Custom Domains

Set your domain during install or in the config:

```yaml
system:
  domain: mydomain.com
user:
  name: admin
# Services accessible at: desktop.admin.mydomain.com, market.admin.mydomain.com, etc.
```

### Subdomains

The system creates the following subdomains under `<username>.<domain>`:

| Subdomain | Service |
|-----------|---------|
| `desktop` | Main desktop UI |
| `market` | App marketplace |
| `settings` | System settings |
| `files` | File manager |
| `dashboard` | Monitoring dashboard |
| `auth` | Authentication |
| `api` | BFL API endpoint |
| `<appname>` | Each installed app gets its own subdomain |

### mDNS

The `mdns` service broadcasts on the local network, making the system discoverable at `packalares.local` without DNS configuration.

---

## Configuration

### config.yaml

The main configuration file lives at `/etc/packalares/config.yaml`. All values are optional -- the system uses sensible defaults.

```yaml
system:
  hostname: packalares
  domain: olares.local
  timezone: UTC
  api_group: packalares.io
  namespaces:
    platform: os-system
    framework: os-framework
    monitoring: monitoring

user:
  name: admin

network:
  tls_secret_name: zone-tls
  tailscale:
    enabled: false
    auth_key: ""
    hostname: packalares
  dns:
    expose_port_53: false

storage:
  data_path: /packalares/data

database:
  host: ""        # auto-discovered
  port: "5432"
  user: packalares
  password: ""    # generated by installer

redis:
  host: ""        # auto-discovered
  port: "6379"

gpu:
  enabled: auto   # auto, true, false
  sharing: true

catalog:
  sync_interval: 24h
```

### Environment Variables

Every config value can be overridden by an environment variable. Priority: config.yaml > environment variable > default.

| Variable | Config Key | Default |
|----------|-----------|---------|
| `SYSTEM_DOMAIN` | `system.domain` | `olares.local` |
| `USERNAME` | `user.name` | (none) |
| `PLATFORM_NAMESPACE` | `system.namespaces.platform` | `os-system` |
| `FRAMEWORK_NAMESPACE` | `system.namespaces.framework` | `os-framework` |
| `PG_HOST` | `database.host` | auto-discovered |
| `PG_PORT` | `database.port` | `5432` |
| `KVROCKS_HOST` | `redis.host` | auto-discovered |
| `NATS_HOST` | `nats.host` | auto-discovered |
| `TS_AUTH_KEY` | `network.tailscale.auth_key` | (none) |
| `GPU_ENABLED` | `gpu.enabled` | `auto` |
| `DATA_PATH` | `storage.data_path` | `/packalares/data` |
| `PACKALARES_CONFIG` | -- | Path to alternate config file |

### Important Paths

| Path | Purpose |
|------|---------|
| `/etc/packalares/config.yaml` | Main configuration |
| `/opt/packalares/` | Installation root |
| `/packalares/data/` | Persistent data (apps, files, models) |
| `/var/lib/etcd/` | etcd data directory |
| `/var/lib/containerd/` | Container images and layers |
| `/var/lib/openebs/` | OpenEBS local PV storage |

---

## Development

### Building from Source

```bash
# Prerequisites: Go 1.25+, Node.js 20+ (for frontend), Docker

# Build the CLI
go build -o packalares ./cmd/cli/

# Build a specific service
go build -o auth-server ./cmd/auth/

# Build the frontend
cd frontend-app && npm install && npm run build

# Build all container images (uses GitHub Actions locally with act, or):
docker build -f build/Dockerfile.auth -t ghcr.io/packalares/auth:dev .
docker build -f build/Dockerfile.bfl -t ghcr.io/packalares/bfl:dev .
# ... etc for each service
```

### Project Structure

```
packalares/
  cmd/                    # Entry points for all binaries
    cli/                  #   CLI (install, uninstall, upgrade, status)
      commands/           #     install.go, uninstall.go, upgrade.go, status.go
    auth/                 #   Auth server
    bfl/                  #   Backend For Launcher
    appservice/           #   App lifecycle manager
    systemserver/         #   Routing and nginx config gen
    market/               #   Marketplace backend
    files/                #   File manager
    monitor/              #   Monitoring server
    middleware/           #   Middleware operator
    mounts/               #   Mount manager
    hostctl/              #   Host control agent
    kubesphere/           #   KubeSphere compat layer
    mdns/                 #   mDNS agent
    tapr/                 #   Infisical proxy
    hf-downloader/        #   HuggingFace model downloader
    l4proxy/              #   L4 TCP proxy
  internal/               # Service implementations (not importable externally)
    auth/                 #   Login, sessions, TOTP, rate limiting
    appservice/           #   Chart download, helm client, manifest parsing
    bfl/                  #   User info, settings, zone config
    market/               #   Catalog sync, chart repo
    files/                #   WebDAV, upload/download
    monitor/              #   Metrics aggregation
    middleware/           #   DB/cache provisioning for apps
    systemserver/         #   Nginx config generation, envoy sidecars
    mounts/               #   SMB, NFS, rclone backends
    tapr/                 #   Secrets vault interface, crypto (AES-256-GCM, NaCl)
    ...
  pkg/                    # Shared packages
    config/               #   Centralized config (config.yaml + env vars)
    installer/            #   Install phases (K3s, etcd, containerd, charts)
    secrets/              #   Secret generation utilities
    validation/           #   Input validation
  deploy/                 # Kubernetes manifests (embedded in CLI binary)
    platform/             #   citus.yaml, nats.yaml, lldap.yaml, infisical.yaml
    framework/            #   auth, bfl, appservice, systemserver, middleware, ...
    apps/                 #   desktop.yaml, wizard.yaml
    proxy/                #   nginx reverse proxy config
    crds/                 #   CRDs and namespace definitions
    infrastructure/       #   calico.yaml (CNI)
    charts/               #   Helm charts (vllm-model)
    envoy/                #   Envoy sidecar configs
    embed.go              #   Go embed directives for all manifests
  frontend-app/           # Vue 3 / Quasar SPA (desktop, wizard, login, settings)
  market/                 # App catalog
    catalog.json          #   170 apps + 4 model cards
    charts/               #   Helm chart archives
    icons/                #   App icons
    screenshots/          #   App screenshots
  build/                  # Dockerfiles for all services
  install.sh              # One-liner installer wrapper
  VERSION                 # Current version (1.0.0)
```

### Adding a New App to the Catalog

1. Create a Helm chart for your app with an `OlaresManifest.yaml` in the chart root
2. Package the chart as a `.tgz` and place it in `market/charts/`
3. Add an entry to `market/catalog.json` with metadata (name, icon, description, resource requirements, entrances)
4. Add an icon to `market/icons/` and screenshots to `market/screenshots/<appname>/`
5. Rebuild the market image or sync the catalog at runtime

The `OlaresManifest.yaml` defines app entrances (subdomains), permissions, dependencies, and middleware requests (databases, caches).

### CI/CD Pipeline

Two GitHub Actions workflows:

| Workflow | File | Trigger | Purpose |
|----------|------|---------|---------|
| Build All Images | `.github/workflows/build-images.yaml` | Push to `main` | Builds and pushes all container images to GHCR |
| Mirror Images | `.github/workflows/mirror-images.yaml` | Manual / schedule | Mirrors upstream images (Citus, LLDAP, NATS, etc.) to GHCR |

Images are tagged with both `:latest` and `:<git-sha>`.

---

## Uninstall

```bash
sudo packalares uninstall
```

This removes all Kubernetes resources, K3s, etcd, containerd, KVRocks, and cleans up data directories.

---

## License

See [LICENSE](LICENSE) for details.
