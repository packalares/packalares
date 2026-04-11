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

A market app requires ALL of these files:

```
market/
  catalog.json                          # App entry with metadata
  icons/{appname}.png                   # App icon (PNG)
  screenshots/{appname}/                # Screenshot directory
    featured.webp                       # Featured image
    1.webp                              # Screenshot 1
    2.webp                              # Screenshot 2 (optional)
    3.webp                              # Screenshot 3 (optional)
  charts/{appname}-{version}.tgz        # Packaged Helm chart (helm package output)
  charts/{appname}/                     # Raw chart dir (temporary, for editing before packaging)
    Chart.yaml
    OlaresManifest.yaml
    values.yaml
    templates/
      deployment.yaml
      configmap.yaml                    # (if needed)
      ...
```

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

Key fields:
- `olaresManifest.version: "0.10.0"`
- `metadata.name` / `metadata.appid` — must match chart name
- `metadata.version` — must match Chart.yaml version
- `entrances` — each needs `name`, `port`, `host`, `title`, `icon`, `openMethod`
- `permission.appData: true` / `permission.appCache: true` — as needed
- `options.apiTimeout: 0` — for AI/long-running apps
- NO `subCharts` section (we flattened)
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

| Value | Path | Notes |
|-------|------|-------|
| `{{ .Values.sharedlib }}` | `/packalares/Apps/sharedlib` | Shared AI models (ollama, comfyui, etc.) |
| `{{ .Values.appData }}` | `/packalares/Apps/appdata` | Persistent app config/databases (if permission.appData=true) |
| `{{ .Values.appCache }}` | `/packalares/Apps/appcache` | App cache/scratch (if permission.appCache=true) |
| `{{ .Values.olaresEnv.ADMIN_USERNAME }}` | | Generated admin username |
| `{{ .Values.olaresEnv.ADMIN_PASSWORD }}` | | Generated admin password |
| `{{ .Values.olaresEnv.UNIQUE_PASS }}` | | Per-app random password (for DB sidecars) |
| `{{ .Release.Namespace }}` | | Kubernetes namespace for this app |
| `{{ .Release.Name }}` | | Helm release name |

## Volume Path Conventions

```
{{ .Values.sharedlib }}/ai/ollama           # Ollama models (shared)
{{ .Values.sharedlib }}/ai/comfyui          # ComfyUI models (shared)
{{ .Values.appData }}/{appname}             # App-specific persistent data
{{ .Values.appCache }}/{appname}            # App-specific cache
{{ .Values.appCache }}/redis/{appname}      # Redis data (per-app)
{{ .Values.appCache }}/{appname}/postgres   # Postgres sidecar data
{{ .Values.appCache }}/{appname}/mysql      # MySQL sidecar data
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
    path: "{{ .Values.appCache }}/{appname}/postgres"
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
    path: "{{ .Values.appCache }}/{appname}/mysql"
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
    path: "{{ .Values.appCache }}/redis/{appname}"
    type: DirectoryOrCreate
```

## Common Chart Fix Patterns

### Shared App Flattening
Original Olares shared apps have: parent chart + sub-chart (e.g. `ollamaserver/`).
The sub-chart deploys in a different namespace. In Packalares:
1. Move all sub-chart templates into parent `templates/`
2. Remove `subCharts` from OlaresManifest
3. Fix any cross-namespace service references (use local service names)
4. Watch for duplicate deployment names across sub-charts — rename if needed

### Nginx Proxy Sidecars
If the original app has an nginx proxy sub-chart for auth:
- Usually NOT needed in Packalares (system-server handles routing)
- If kept, fix upstream to point to `localhost:{port}` or local service name
- Set proper timeouts for AI apps: `proxy_read_timeout 600s;`

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

- Download the app icon as PNG and save to `market/icons/{appname}.png`
- Download screenshots from the app's website/GitHub and save to `market/screenshots/{appname}/`
- Use `/api/market/` prefix for all asset paths in catalog.json (NOT external CDN URLs)
- If no screenshots are available, the detail page will just skip the screenshot section
