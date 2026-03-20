# App Service — Packalares Marketplace Backend

The app-service is a lightweight REST API that manages the Packalares app marketplace. It provides endpoints for browsing a catalog of self-hosted apps, installing/uninstalling them via Helm, and reporting system status and resource usage.

Caddy routes `/api/*` to this service on port 5000. The dashboard frontend consumes these endpoints.

## API Endpoints

All endpoints are served under `/api/`.

### GET /api/status

Returns cluster health: whether K3s is reachable, pod states, services, and whether the system has been activated (has at least one running pod in the apps namespace).

Response:
```json
{
  "cluster_reachable": true,
  "activated": true,
  "pods": [
    {"name": "pack-jellyfin-0", "namespace": "packalares-apps", "phase": "Running", "ready": true, "restarts": 0}
  ],
  "services": [
    {"name": "pack-jellyfin", "type": "ClusterIP", "cluster_ip": "10.43.x.x", "ports": [{"port": 8096, "target": 8096}]}
  ]
}
```

### GET /api/metrics

Returns CPU, memory, disk, and GPU usage for the node. CPU and memory come from `kubectl top node` (requires metrics-server). GPU data comes from `nvidia-smi` when available.

Response:
```json
{
  "cpu": {"usage": "450m", "percent": "11%"},
  "memory": {"usage": "3.2Gi", "percent": "40%"},
  "disk": {"total": "500G", "used": "120G", "available": "380G", "percent": "24%"},
  "gpu": [{"name": "NVIDIA GeForce RTX 4090", "memory_used_mib": "2048", "memory_total_mib": "24576", "utilization_percent": "15"}]
}
```

Fields will be `null` when the data source is unavailable (e.g., no GPU present, metrics-server not installed).

### GET /api/apps/available

Returns the marketplace catalog. Each app entry includes an `installed` boolean indicating whether a corresponding Helm release exists.

Response:
```json
{
  "apps": [
    {
      "name": "jellyfin",
      "title": "Jellyfin",
      "icon_url": "https://...",
      "description": "Free software media system...",
      "version": "1.9.0",
      "chart_url": "https://...",
      "ports": [8096],
      "storage_needed": "10Gi",
      "gpu_optional": true,
      "installed": false
    }
  ]
}
```

### GET /api/apps/installed

Returns all installed apps with their Helm release status and pod states.

Response:
```json
{
  "apps": [
    {
      "name": "jellyfin",
      "title": "Jellyfin",
      "icon_url": "https://...",
      "version": "1.9.0",
      "status": "deployed",
      "updated": "2026-03-21 12:00:00",
      "pods": [
        {"name": "pack-jellyfin-0", "phase": "Running", "ready": true}
      ]
    }
  ]
}
```

### POST /api/apps/install

Installs an app from the catalog via Helm.

Request body:
```json
{"name": "jellyfin", "version": "1.9.0"}
```

`version` is optional; defaults to the version specified in the catalog.

The service will:
1. Look up the app in `catalog.json`
2. Add the Helm repo if a `chart_repo` is specified
3. Create the `packalares-apps` namespace if it does not exist
4. Run `helm upgrade --install` with the catalog's `values_override`
5. Wait for the release to become ready (up to 5 minutes)

Response (success):
```json
{"ok": true, "app": "jellyfin", "version": "1.9.0", "release": "pack-jellyfin"}
```

### POST /api/apps/uninstall

Uninstalls an app by removing its Helm release.

Request body:
```json
{"name": "jellyfin"}
```

Response (success):
```json
{"ok": true, "app": "jellyfin", "release": "pack-jellyfin"}
```

### GET /healthz

Health check endpoint. Returns `{"ok": true}`.

## How to Add Apps to the Catalog

Edit `catalog.json` and add an entry to the `apps` array:

```json
{
  "name": "myapp",
  "title": "My App",
  "icon_url": "https://example.com/icon.png",
  "description": "What this app does.",
  "version": "1.0.0",
  "chart_url": "https://charts.example.com/myapp",
  "chart_repo": "https://charts.example.com",
  "chart_name": "myrepo/myapp",
  "ports": [8080],
  "storage_needed": "5Gi",
  "gpu_optional": false,
  "values_override": {
    "service.type": "ClusterIP"
  }
}
```

Field reference:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Machine-readable identifier (lowercase, no spaces) |
| `title` | yes | Human-readable display name |
| `icon_url` | yes | URL to the app icon (SVG or PNG) |
| `description` | yes | Short description shown in the marketplace |
| `version` | yes | Default chart version to install |
| `chart_url` | yes | Direct URL to the chart (informational) |
| `chart_repo` | yes | Helm repository URL to `helm repo add` |
| `chart_name` | yes | Chart reference as `repo/chart` for `helm install` |
| `ports` | yes | List of ports the app exposes |
| `storage_needed` | yes | Recommended PVC size |
| `gpu_optional` | yes | Whether the app can use a GPU for acceleration |
| `values_override` | no | Helm values to set via `--set` during install |

After editing, update the ConfigMap:

```bash
kubectl create configmap app-catalog \
  --from-file=catalog.json=catalog.json \
  -n packalares-system \
  --dry-run=client -o yaml | kubectl apply -f -
```

Then restart the app-service pod to pick up changes:

```bash
kubectl rollout restart deployment/app-service -n packalares-system
```

## How App Installation Works

1. The user selects an app in the dashboard and clicks Install.
2. The dashboard POSTs `{"name": "appname"}` to `/api/apps/install`.
3. The app-service looks up the app in `catalog.json`.
4. It runs `helm repo add` and `helm repo update` for the chart repository.
5. It creates the `packalares-apps` namespace if needed.
6. It runs `helm upgrade --install pack-<name> <chart> -n packalares-apps` with any `values_override` from the catalog.
7. Helm deploys the chart and waits up to 5 minutes for pods to become ready.
8. The API returns success or failure with the Helm output.

All apps are installed as Helm releases with the prefix `pack-` in the `packalares-apps` namespace. This keeps marketplace apps isolated from system components in `packalares-system`.

## How Status Reporting Works

**System status** (`/api/status`) queries the Kubernetes API via `kubectl` to list pods and services in the apps namespace. The `activated` field is true when the cluster is reachable and at least one pod exists.

**Metrics** (`/api/metrics`) combines three data sources:
- `kubectl top node` for CPU and memory (requires metrics-server, installed by default with K3s)
- `df -h` for disk usage on the data partition
- `nvidia-smi` for GPU utilization (only present when an NVIDIA GPU is detected)

**Installed apps** (`/api/apps/installed`) uses `helm list -n packalares-apps -o json` to enumerate releases, then queries pod status for each release using label selectors.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NAMESPACE` | `packalares-apps` | Kubernetes namespace for installed apps |
| `KUBECONFIG` | (empty, uses in-cluster) | Path to kubeconfig file |
| `CATALOG_PATH` | `./catalog.json` | Path to the marketplace catalog JSON |
| `MARKETPLACE_URL` | (empty) | Remote catalog URL (reserved for future use) |
| `APP_SERVICE_PORT` | `5000` | Port the API server listens on |

## Running Locally (Development)

```bash
pip install -r requirements.txt
export NAMESPACE=packalares-apps
python server.py
```

The server starts on `http://localhost:5000`. It needs `kubectl` and `helm` in PATH to interact with a cluster.

## Deployment

Apply the Kubernetes manifest:

```bash
kubectl apply -f app-service-deployment.yaml
```

This creates:
- `packalares-system` namespace (for system components)
- `packalares-apps` namespace (for user-installed apps)
- ServiceAccount and RBAC for the app-service
- Deployment running the API server
- ClusterIP Service on port 5000

Caddy (managed by a separate agent) reverse-proxies `/api/*` to `app-service.packalares-system.svc.cluster.local:5000`.
