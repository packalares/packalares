# App Service -- Packalares Marketplace Backend

The app-service is a Go REST API that manages the Packalares app marketplace. It provides endpoints for browsing a catalog of self-hosted apps, installing and uninstalling them via Helm, and reporting system status and resource metrics.

The server listens on port 8080. Caddy reverse-proxies `/api/*` to this service.

## API Endpoints

### GET /api/status

Returns cluster health: pod states, services in the apps namespace, and activation state.

```json
{
  "activated": true,
  "namespace": "packalares-apps",
  "pods": [
    {"name": "packalares-jellyfin-xxx", "status": "Running", "ready": "true", "age": "2025-01-01T00:00:00Z"}
  ],
  "services": ["packalares-jellyfin"]
}
```

### GET /api/metrics

Returns CPU, memory, disk, and GPU usage read from `/proc` and system commands.

```json
{
  "cpu": {"usage_percent": 12.5, "cores": 4},
  "memory": {"total_mb": 16384, "used_mb": 6400, "used_percent": 39.1},
  "disk": {"total_gb": 500, "used_gb": 120, "used_percent": 24},
  "gpu": {"name": "NVIDIA GeForce RTX 4090", "memory_mb": 24576, "used_mb": 2048, "temp_c": 45, "available": true}
}
```

The `gpu` field is `null` when no NVIDIA GPU is detected.

### GET /api/apps/available

Returns the marketplace catalog loaded from `catalog.json`.

```json
[
  {
    "name": "jellyfin",
    "title": "Jellyfin",
    "icon": "https://...",
    "description": "Free software media system...",
    "version": "1.9.0",
    "chart_url": "https://...",
    "ports": [8096],
    "storage": "10Gi",
    "gpu_optional": true
  }
]
```

### GET /api/apps/installed

Returns all Helm releases in the apps namespace.

```json
[
  {
    "name": "packalares-jellyfin",
    "namespace": "packalares-apps",
    "revision": "1",
    "status": "deployed",
    "chart": "jellyfin-1.9.0",
    "app_version": "1.9.0"
  }
]
```

### POST /api/apps/install

Installs an app from the catalog.

Request:
```json
{"name": "jellyfin", "version": "1.9.0"}
```

The `version` field is optional; defaults to the version in the catalog.

Response:
```json
{"status": "installed", "name": "jellyfin", "release": "packalares-jellyfin", "version": "1.9.0", "output": "..."}
```

### POST /api/apps/uninstall

Uninstalls an app by removing its Helm release.

Request:
```json
{"name": "jellyfin"}
```

Response:
```json
{"status": "uninstalled", "name": "jellyfin", "release": "packalares-jellyfin", "output": "..."}
```

## How to Add Apps to the Catalog

Edit `catalog.json` (a JSON array at the top level). Each entry has these fields:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Machine-readable identifier (lowercase, hyphens ok) |
| `title` | string | Human-readable display name |
| `icon` | string | URL to the app icon |
| `description` | string | Short description for the marketplace |
| `version` | string | Default Helm chart version to install |
| `chart_url` | string | Helm chart URL passed to `helm upgrade --install` |
| `ports` | []int | Ports the app exposes |
| `storage` | string | Recommended PVC size |
| `gpu_optional` | bool | Whether the app can use GPU acceleration |

Example entry:

```json
{
  "name": "myapp",
  "title": "My App",
  "icon": "https://example.com/icon.png",
  "description": "What this app does.",
  "version": "1.0.0",
  "chart_url": "https://charts.example.com/myapp",
  "ports": [8080],
  "storage": "5Gi",
  "gpu_optional": false
}
```

After editing, update the ConfigMap and restart:

```bash
kubectl create configmap app-catalog \
  --from-file=catalog.json=catalog.json \
  -n packalares-system \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/app-service -n packalares-system
```

## How App Installation Works

1. The user sends `POST /api/apps/install` with `{"name": "appname"}`.
2. The service looks up the app in the in-memory catalog (loaded from `catalog.json`).
3. It runs `helm upgrade --install packalares-<name> <chart_url> -n packalares-apps --create-namespace --version <version> --wait --timeout 5m`.
4. Helm deploys the chart and waits for pods to become ready.
5. The API returns the Helm output on success, or an error message on failure.

All apps are installed as Helm releases with the prefix `packalares-` in the `packalares-apps` namespace.

Uninstalling runs `helm uninstall packalares-<name> -n packalares-apps`.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NAMESPACE` | `packalares-apps` | Kubernetes namespace for installed apps |
| `KUBECONFIG` | (in-cluster) | Path to kubeconfig file |
| `CATALOG_PATH` | `/etc/packalares/catalog.json` | Path to the catalog JSON file |
| `MARKETPLACE_URL` | (empty) | Remote catalog URL (reserved for future use) |

## Build and Deploy

### Build the container image

```bash
cd app-service
docker build -t packalares/app-service:latest .
```

### Run locally (development)

```bash
export CATALOG_PATH=./catalog.json
export NAMESPACE=packalares-apps
go run .
```

The server starts on `http://localhost:8080`. Requires `kubectl` and `helm` in PATH.

### Deploy to Kubernetes

```bash
kubectl apply -f app-service-deployment.yaml
```

This creates:
- `packalares-system` namespace (system components)
- `packalares-apps` namespace (user-installed apps)
- ServiceAccount and RBAC for the app-service
- Deployment running the Go binary
- ClusterIP Service on port 8080
