# Packalares

A self-hosted personal cloud OS that runs on your hardware. No cloud accounts, no phone apps, no external dependencies. Install it, set a username and password, and you have a full personal cloud with app marketplace, file manager, GPU/AI support, and secure access from anywhere.

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/packalares/packalares/main/install.sh | bash
```

Or clone and run:

```bash
git clone https://github.com/packalares/packalares.git
cd packalares
sudo bash install.sh
```

Non-interactive install:

```bash
PACKALARES_USER=alice PACKALARES_PASSWORD=secret sudo -E bash install.sh
```

## What You Get

- **Desktop** — web-based desktop environment at `https://desktop.alice.olares.local`
- **App Marketplace** — one-click install of self-hosted apps (Nextcloud, Jellyfin, Home Assistant, etc.)
- **File Manager** — browse local storage, mount NAS/SMB shares, Google Drive, S3
- **GPU/AI Workloads** — NVIDIA GPU passthrough with CUDA support (auto-detected)
- **Secure Access** — Authelia login with optional 2FA, self-signed TLS everywhere
- **Remote Access** — works with your own Tailscale/VPN setup

## Architecture

```
                    Internet / LAN
                         |
                    +---------+
                    |  Caddy   |  reverse proxy, TLS termination
                    +---------+
                    /    |    \
          +--------+ +------+ +--------+
          |Authelia| | Apps | |  mDNS  |
          +--------+ +------+ +--------+
               |         |
          +--------+ +----------+
          | LLDAP  | |app-service|
          +--------+ +----------+
               |         |
          +--------+ +--------+
          | Redis  | |  K3s   |  Kubernetes runtime
          +--------+ +--------+
                         |
              +----------+----------+
              |          |          |
          +--------+ +------+ +-------+
          |Postgres| | NATS | | MinIO |  platform services
          +--------+ +------+ +-------+
                         |
                    +---------+
                    | Storage  |  local disk, NAS mounts
                    +---------+
```

**Request flow:** Browser hits Caddy on port 443. Caddy terminates TLS with a self-signed certificate and forwards the request to Authelia for authentication. Once authenticated, the request is proxied to the target application running as a Kubernetes pod. Applications are accessed by subdomain (`desktop.alice.olares.local`, `files.alice.olares.local`) or by path when using the IP directly (`https://192.168.1.100/desktop/`).

## Modules

The installer is modular. Each module is an independent script in `scripts/` that receives configuration through environment variables and checks its own state before acting (idempotent).

| Module | Script | What it does |
|--------|--------|-------------|
| **K3s** | `scripts/setup-k3s.sh` | Installs K3s (lightweight Kubernetes). Skips if already running. |
| **Auth** | `scripts/setup-auth.sh` | Deploys Authelia (SSO/2FA), LLDAP (user directory), and Redis (session store) into the `packalares-auth` namespace. |
| **Platform** | `scripts/setup-platform.sh` | Deploys PostgreSQL, NATS, and MinIO into the `packalares-platform` namespace. Shared services for marketplace apps. |
| **Caddy** | `scripts/setup-caddy.sh` | Deploys Caddy as a DaemonSet with hostNetwork. Processes `proxy/Caddyfile.tmpl` with environment variables. |
| **Activate** | `scripts/activate.sh` | Creates the user in LLDAP, generates TLS certs, configures Caddy, sets up mDNS. |

The master installer (`install.sh`) calls them in order and deploys the core application manifests (`app-service/`, `dashboard/`).

### Module contract

Every module script:
1. Reads configuration **only** from environment variables (never hardcodes values)
2. Checks if its components are already installed and **skips** if so
3. Can be run standalone for debugging: `PACKALARES_USER=alice PACKALARES_DOMAIN=olares.local bash scripts/setup-auth.sh`

### Environment variables passed between modules

| Variable | Description | Default |
|----------|-------------|---------|
| `PACKALARES_USER` | Username | *(prompted)* |
| `PACKALARES_PASSWORD` | Password | *(prompted)* |
| `PACKALARES_DOMAIN` | Base domain | `olares.local` |
| `USER_ZONE` | Full zone (`user.domain`) | *derived* |
| `NODE_IP` | Server IP address | *auto-detected* |
| `PACKALARES_GPU` | GPU available | `true` / `false` |
| `PACKALARES_OS` | OS identifier | *auto-detected* |
| `PACKALARES_VERSION` | Version string | from `VERSION` file |
| `KUBECONFIG` | Path to kubeconfig | `/etc/rancher/k3s/k3s.yaml` |
| `POSTGRES_VERSION` | PostgreSQL image tag | `16-alpine` |
| `NATS_VERSION` | NATS image tag | `2.10-alpine` |
| `MINIO_VERSION` | MinIO image tag | `RELEASE.2024-06-13T22-53-53Z` |
| `POSTGRES_PASSWORD` | PostgreSQL admin password | *auto-generated* |
| `MINIO_ROOT_USER` | MinIO root user | `packalares` |
| `MINIO_ROOT_PASSWORD` | MinIO root password | *auto-generated* |

## Configuration

All configuration lives in `config.yaml`. The installer reads from environment variables; the config file is used by the running system.

```yaml
server:
  name: alice
  domain: olares.local
  timezone: auto
  language: en-US

network:
  mode: local          # local | tailscale
  ip: auto

auth:
  policy: one_factor   # one_factor | two_factor
  session_timeout: 24h

gpu:
  enabled: auto        # auto | true | false
```

## How to Add Apps

Apps are Kubernetes deployments. To add a new app:

1. **Create a Helm chart or deployment YAML** for your app. It should deploy into the `user-space-USERNAME` namespace.

2. **Expose it as a service** named `APPNAME-svc` on port 80. Caddy's wildcard rule (`*.USER_ZONE`) automatically routes `APPNAME.USER_ZONE` to `APPNAME-svc.user-space-USERNAME:80`.

3. **Deploy it:**
   ```bash
   kubectl apply -f my-app.yaml -n user-space-alice
   ```

4. **Access it** at `https://myapp.alice.olares.local`. Authentication is handled automatically by Authelia via Caddy's forward-auth.

Example minimal app deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: user-space-alice
spec:
  replicas: 1
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:latest
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: myapp-svc
  namespace: user-space-alice
spec:
  selector:
    app: myapp
  ports:
    - port: 80
      targetPort: 8080
```

Apps installed through the marketplace follow this same convention and are managed by `app-service`.

## Docker Images

All container images are listed in `images.yaml`. This is the single source of truth for image versions across the entire system.

## File Layout

```
packalares/
  install.sh              — master installer
  config.yaml             — system configuration
  images.yaml             — all container image references
  VERSION                 — current version string
  scripts/
    setup-k3s.sh          — K3s installer module
    setup-auth.sh         — auth stack module
    setup-platform.sh     — platform services module
    setup-caddy.sh        — reverse proxy module
    activate.sh           — user activation module
  proxy/
    Caddyfile.tmpl        — Caddy config template
    caddy-deployment.yaml — Caddy K8s manifest
  database/
    postgres-deployment.yaml — PostgreSQL K8s manifest
  messaging/
    nats-deployment.yaml  — NATS K8s manifest
  objectstore/
    minio-deployment.yaml — MinIO K8s manifest
  app-service/
    app-service-deployment.yaml
  dashboard/
    dashboard-deployment.yaml
```

## Requirements

- Linux (Ubuntu 22.04+ / Debian 12+ recommended)
- 4 GB RAM minimum, 8 GB recommended
- Root access
- Ports 80 and 443 available
- NVIDIA GPU (optional, auto-detected)
