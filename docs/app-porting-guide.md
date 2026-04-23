# Packalares App Porting Guide

## Mandatory Pre-Packaging Checklist

Before building ANY chart, verify ALL of these:

1. **Flatten sub-charts** — No `subCharts` section in OlaresManifest. Move all sub-chart templates into single `templates/` dir
2. **No ProviderRegistry** — CRD doesn't exist, delete any ProviderRegistry templates
3. **No admin conditionals** — Remove `{{- if and .Values.admin ...}}` wrappers, keep the content
4. **No OIDC** — Remove all OIDC/OAuth config, we don't support it
5. **DirectoryOrCreate** — All hostPath volumes: `type: DirectoryOrCreate` (never `type: Directory`)
6. **ConfigMap naming** — Prefix with app name: `{appname}-nginx-config`, `{appname}-redis-config`, etc. Generic names like `nginx-config`, `script`, `config` WILL collide
7. **Redis image** — Use `ghcr.io/packalares/redis:7-alpine` (no auth), NOT `beclab/aboveos-redis:7` (has password)
8. **Instance labels** — Every pod template MUST have `app.kubernetes.io/instance: {chartname}`
9. **Namespace** — All resources: `namespace: "{{ .Release.Namespace }}"`
10. **No hardcoded passwords** — Use `{{ .Values.olaresEnv.UNIQUE_PASS }}` for generated passwords
11. **Pin image tags** — Never use `:latest`. Verify image exists: `docker manifest inspect image:tag`
12. **Latest stable version** — Always check Docker Hub for the latest stable upstream release. Do NOT copy old versions from the original chart. Use the Docker Hub API: `https://hub.docker.com/v2/repositories/{owner}/{repo}/tags/?page_size=20&ordering=last_updated` and pick the newest non-rc, non-rocm, non-alpha, non-beta tag.
13. **Image priority** — Use upstream images by default (e.g. `ollama/ollama`, `postgres`, `mysql`, `redis`). Use `beclab/` images ONLY when no upstream exists (e.g. `beclab/terminal` — custom web terminal, no upstream). Use `ghcr.io/packalares/` ONLY for our custom builds (e.g. redis without auth).

## Complete App Package

A market app requires ALL of these files. When building a new app, you MUST create/download ALL of them — not just the chart:

```
market/
  catalog.json                          # App entry with FULL metadata (description, screenshots, resources, etc.)
  icons/{appname}.png                   # App icon (PNG) — download from upstream or Olares CDN
  screenshots/{appname}/                # Screenshot directory
    featured.webp                       # Featured image
    1.webp                              # Screenshot 1
    2.webp                              # Screenshot 2 (optional)
    3.webp                              # Screenshot 3 (optional)
  charts/{appname}-{version}.tgz        # Packaged Helm chart (helm package output)
```

### Building a new app — FULL checklist:
1. **Research the app** — Follow this order:
   a. **Check Olares app store** — Fetch `https://market.olares.com/app-store/api/v2/market/data` (JSON endpoint with all apps). Search for the app by name. This gives you the correct image, ports, chart structure, env vars, and metadata. Also check the Olares GitHub (`https://github.com/beclab/apps`) for chart source code.
   b. **Check CUDA compatibility** — RTX 5090 (Blackwell) needs CUDA 12.8+. Images with cu124 or lower won't work. Search Docker Hub for a cu128+ variant of the same image.
   c. **Check upstream** — Visit the app's GitHub repo for official Docker images, default ports, required env vars, volume paths.
   d. **Verify image exists** — `docker manifest inspect image:tag` or check Docker Hub page
2. **Download icon** — from upstream project, Olares CDN, or app website
3. **Download/create screenshots** — from upstream, Olares CDN, or take them manually
4. **Create the chart** — Chart.yaml, OlaresManifest.yaml (with FULL spec including fullDescription, developer, website, license, resources), templates/deployment.yaml, values.yaml
5. **Use `{{ .Values.appName }}`** — for ALL names, labels, service names, volume paths in templates
6. **Add GPU support** — if the app needs GPU, add `runtimeClassName: nvidia` to pod spec
7. **Package the chart** — `helm package {appname}/` to create .tgz
8. **Add catalog.json entry** — with FULL metadata including fullDescription matching the manifest
9. **Verify** — icon file is not empty (>1KB), screenshots exist, chart packages without errors

### Chart.yaml

```yaml
apiVersion: v2
appVersion: '{upstream_version}'
description: {short description}
name: {appname}
type: application
version: '{chart_version}'
```

### OlaresManifest.yaml

`metadata.name` is the **single source of truth** for the app name. The installer injects it as `{{ .Values.appName }}` into Helm templates. Entrance `name`, `host`, and `appid` are auto-derived from `metadata.name` when omitted.

Minimal example:
```yaml
olaresManifest.version: '0.10.0'
olaresManifest.type: app
metadata:
  name: myapp              # Single source of truth — becomes {{ .Values.appName }}
  title: My App            # Display name (can differ from name)
  description: Short description
  icon: https://file.bttcdn.com/appstore/default/icon.png
  version: '1.0.0'
  categories:
  - Creativity
entrances:
- port: 3000               # name/host auto-filled from metadata.name
  title: My App
  openMethod: window
permission:
  appData: true
  appCache: true
spec:
  versionName: '1.0.0'
  supportArch:
  - amd64
options:
  apiTimeout: 0             # Set to 0 for AI/long-running apps
```

Key rules:
- `metadata.name` = URL subdomain = service name = data path prefix
- `metadata.version` must match Chart.yaml `version`
- Entrance `name`/`host` can be omitted — auto-filled from `metadata.name`
- NO `subCharts` section (we flatten sub-charts)
- `spec.supportArch: [amd64]` minimum

### catalog.json Entry

Each app in `catalog.json` → `apps[]` array needs:

```json
{
  "name": "{appname}",
  "cfgType": "app",
  "chartName": "{appname}",
  "icon": "/api/market/icons/{appname}.png",
  "description": "Short description",
  "fullDescription": "Full markdown description",
  "promoteImage": [
    "/api/market/screenshots/{appname}/1.webp",
    "/api/market/screenshots/{appname}/2.webp"
  ],
  "featuredImage": "/api/market/screenshots/{appname}/featured.webp",
  "developer": "developer name",
  "title": "Display Title",
  "entrances": [...],
  "version": "{chart_version}",
  "versionName": "{upstream_version}",
  "categories": ["Working"],
  "requiredMemory": "...",
  "limitedMemory": "...",
  "requiredCpu": "...",
  "limitedCPU": "...",
  "requiredDisk": "...",
  "requiredGpu": "...",
  "supportArch": ["amd64"],
  "status": "active",
  "type": "app",
  "locale": ["en-US"],
  "permission": { "appData": true, "appCache": true, "userData": null },
  "source": "olares",
  "hasChart": true,
  "hasCredentials": false,
  "loginType": ""
}
```

**Credentials fields:**
- `hasCredentials: true` — if the app has a login screen that uses the generated ADMIN_USERNAME/ADMIN_PASSWORD
- `loginType` — `"user"` (default), `"email"` (login with email), `"user-email"` (show both)
- When `loginType: "email"`, the chart should set admin email to `{{ .Values.olaresEnv.ADMIN_USERNAME }}@{{ .Values.user.zone }}`

**Labels:**
- Do NOT use `bytetrade.io` labels — use `packalares.io` instead
- Terminal containers use label `packalares.io/terminal: {appname}` on the target pod

**Chart packaging:**
- After building the chart directory, run `helm package {appname}` to create the `.tgz`
- The `.tgz` goes in `market/charts/`, the raw directory is NOT committed to git
- The market backend serves charts from `.tgz` files only

**Resources in catalog.json are informational only** — the real resource limits come from the chart's deployment templates and are auto-extracted by the market backend. The catalog values are fallback display values.

**Container images are auto-extracted** from chart templates — do NOT add an `images` field to catalog.json.
```

**IMPORTANT**: All image paths use `/api/market/` prefix (served from local filesystem), NOT external CDN URLs.

## Helm Values Available

App-service injects these at install time:

| Value | Path / Example | Notes |
|-------|----------------|-------|
| `{{ .Values.appName }}` | `sunoace` | **App name** — injected from OlaresManifest `metadata.name`. Use for service names, labels, volume paths |
| `{{ .Values.sharedlib }}` | `/packalares/Apps/sharedlib` | Shared AI models (ollama, comfyui, etc.) |
| `{{ .Values.userspace.appData }}` | `/packalares/Apps/appdata` | Persistent app config/databases (if permission.appData=true) |
| `{{ .Values.userspace.appCache }}` | `/packalares/Apps/appcache` | App cache/scratch (if permission.appCache=true) |
| `{{ .Values.olaresEnv.ADMIN_USERNAME }}` | | Generated admin username |
| `{{ .Values.olaresEnv.ADMIN_PASSWORD }}` | | Generated admin password |
| `{{ .Values.olaresEnv.UNIQUE_PASS }}` | | Per-app random password (for DB sidecars) |
| `{{ .Release.Namespace }}` | | Kubernetes namespace for this app |
| `{{ .Release.Name }}` | | Helm release name |

## Volume Path Conventions

```
{{ .Values.sharedlib }}/ai/ollama                          # Ollama models (shared)
{{ .Values.sharedlib }}/ai/comfyui                         # ComfyUI models (shared)
{{ .Values.userspace.appData }}/{{ .Values.appName }}                # App-specific persistent data
{{ .Values.userspace.appCache }}/{{ .Values.appName }}               # App-specific cache
{{ .Values.userspace.appCache }}/redis/{{ .Values.appName }}         # Redis data (per-app)
{{ .Values.userspace.appCache }}/postgres/{{ .Values.appName }}      # Postgres sidecar data
{{ .Values.userspace.appCache }}/mysql/{{ .Values.appName }}         # MySQL sidecar data
```

## Database Patterns

### Shared Citus (Postgres)
For apps that need Postgres but don't need special extensions:
- Host: `citus-0.citus-headless.os-platform.svc.cluster.local`
- Port: 5432
- User: postgres
- Password: use `{{ .Values.olaresEnv.UNIQUE_PASS }}`
- Database: create via init container

### Postgres Sidecar
For apps needing specific extensions (pgvector, etc.):
```yaml
- name: postgres
  image: pgvector/pgvector:0.8.2-pg17  # or postgres:17-alpine
  env:
    - name: POSTGRES_PASSWORD
      value: "{{ .Values.olaresEnv.UNIQUE_PASS }}"
    - name: POSTGRES_DB
      value: "{appname}"
    - name: PGDATA
      value: /var/lib/postgresql/data/pgdata
  volumeMounts:
    - name: pg-data
      mountPath: /var/lib/postgresql/data
# Volume:
- name: pg-data
  hostPath:
    path: "{{ .Values.userspace.appCache }}/postgres/{appname}"
    type: DirectoryOrCreate
```

### MySQL Sidecar
```yaml
- name: mysql
  image: mysql:8.0
  env:
    - name: MYSQL_ROOT_PASSWORD
      value: "{{ .Values.olaresEnv.UNIQUE_PASS }}"
    - name: MYSQL_DATABASE
      value: "{appname}"
  volumeMounts:
    - name: mysql-data
      mountPath: /var/lib/mysql
# Volume:
- name: mysql-data
  hostPath:
    path: "{{ .Values.userspace.appCache }}/mysql/{appname}"
    type: DirectoryOrCreate
```

### Redis Sidecar
```yaml
- name: redis
  image: ghcr.io/packalares/redis:7-alpine
  ports:
    - containerPort: 6379
  volumeMounts:
    - name: redis-data
      mountPath: /data
# Volume:
- name: redis-data
  hostPath:
    path: "{{ .Values.userspace.appCache }}/redis/{appname}"
    type: DirectoryOrCreate
```

## Porting from Olares — General Approach

When porting an app from Olares, follow this order:

1. **Get the original chart** — fetch from Olares market API or GitHub
2. **Keep the original structure** — do NOT rewrite from scratch. The original chart works. Preserve all template files, init containers, env vars, volume mounts, proxy configurations. Only change what's listed below.
3. **Apply these modifications:**
   - Replace all hardcoded app names with `{{ .Values.appName }}`
   - Replace `beclab/aboveos-busybox` with `docker.io/busybox:1.37.0`
   - Remove `appid` from OlaresManifest.yaml (auto-derived)
   - Remove `subCharts` section and flatten into single `templates/` dir
   - Remove ProviderRegistry templates, admin conditionals, OIDC config
   - Add `runtimeClassName: nvidia` if the app needs GPU
   - For GPU apps: read the image conditional logic (CUDA version checks) and pick the correct image for CUDA 12.8+
   - Add service name suffix `-svc` to match installer auto-fill convention
   - All hostPath volumes: `type: DirectoryOrCreate`
   - Use `{{ .Values.userspace.appData }}/{{ .Values.appName }}` for data paths
4. **Do NOT remove** init containers, nginx proxies, configmaps, scripts, or env vars unless you understand exactly what they do. If unsure, keep them.

## Common Chart Fix Patterns

### Shared App Flattening
Original Olares shared apps have: parent chart + sub-chart (e.g. `ollamaserver/`).
The sub-chart deploys in a different namespace. In Packalares:
1. Move all sub-chart templates into parent `templates/`
2. Remove `subCharts` from OlaresManifest
3. Fix any cross-namespace service references (use local service names)
4. Watch for duplicate deployment names across sub-charts — rename if needed

### Nginx Proxy Sidecars
There are TWO types of nginx proxies in Olares charts. Treat them differently:

**1. Auth/routing proxy** — proxies traffic through Olares auth system. These are NOT needed in Packalares (system-server handles auth routing). Remove them.

**2. Internal service merger proxy** — merges multiple internal ports (e.g., web UI on 8080, API on 3000, backend on 8188) into one endpoint. These ARE needed. Keep them. Without them the app won't function because the frontend expects all endpoints on the same port.

How to tell the difference:
- If the proxy config has `auth_request`, `X-BFL-USER`, or references to auth services → it's type 1, remove it
- If the proxy config just does `proxy_pass` to different internal ports → it's type 2, keep it

When keeping the proxy:
- Move the proxy deployment + configmap + service into the main `templates/` directory
- Rename resources to use `{{ .Values.appName }}-proxy` or `{{ .Values.appName }}-entrance`
- Fix upstream service references to use `{{ .Values.appName }}.{{ .Release.Namespace }}`
- Set proper timeouts for AI apps: `proxy_read_timeout 600s;`
- The entrance should point to the PROXY service, not the app service directly

### envFrom ConfigMap
Each container needs its OWN envFrom. Don't share a Python app's configmap with a Go daemon — they may not understand the same env vars.

## Image Verification

Before including any image, verify it exists:
```bash
docker manifest inspect docker.io/image:tag
```

Common issues:
- `beclab/` images may not exist on Docker Hub — only use when no upstream exists (e.g. `beclab/terminal`)
- `:latest` changes without notice — always pin to specific version
- Always check for the newest stable release, don't reuse old versions from original charts
- CUDA images: RTX 5090 (Blackwell) needs CUDA 12.8+ (cu129). Images shipping cu126 will NOT work.

## Validation

Run before packaging:
```bash
./scripts/validate-chart.sh market/charts/{appname}/
```

Package chart:
```bash
cd market/charts && helm package {appname}/
```
This creates `{appname}-{version}.tgz` which the market backend serves.

**IMPORTANT: After packaging, remove the raw chart directory. Only the `.tgz` is committed to git.**

## Icon and Screenshot Assets

All Olares apps have icons and screenshots on the Olares CDN. Download them:

**Icon:**
```bash
curl -sLo market/icons/{appname}.png "https://app.cdn.olares.com/appstore/{appname}/icon.png"
```

**Screenshots:**
```bash
mkdir -p market/screenshots/{appname}
curl -sLo market/screenshots/{appname}/1.webp "https://app.cdn.olares.com/appstore/{appname}/promote_image_1v2.webp"
curl -sLo market/screenshots/{appname}/2.webp "https://app.cdn.olares.com/appstore/{appname}/promote_image_2v2.webp"
curl -sLo market/screenshots/{appname}/3.webp "https://app.cdn.olares.com/appstore/{appname}/promote_image_3v2.webp"
curl -sLo market/screenshots/{appname}/featured.webp "https://app.cdn.olares.com/appstore/{appname}/promote_image_4v2.webp"
```

Some apps use `promote_image_1.webp` (without `v2`). If `v2` returns 404, try without the suffix.
If the CDN doesn't have assets, search the app's official website or GitHub for screenshots.

- Use `/api/market/` prefix for all asset paths in catalog.json (NOT external CDN URLs)
- Verify downloaded files are not empty (some apps may have fewer screenshots)
