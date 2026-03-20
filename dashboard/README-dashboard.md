# Packalares Dashboard

Single-page web app that serves as both the setup wizard and the main dashboard.
Plain HTML/CSS/JS, no build step. Served by Caddy at the root path.

## How it works

### Wizard / Dashboard switch

On load, the page calls `GET /api/status`. The response contains an `activated` boolean:

- **`activated: false`** — Shows the setup wizard. Polls `/api/status` every 3 seconds to track installation progress.
- **`activated: true`** — Shows the dashboard. Polls `/api/metrics` and `/api/apps` every 10 seconds.

Once all services reach "running" state during the wizard phase, a "System Ready" screen appears with the hosts file entry and desktop link. After activation, subsequent page loads go straight to the dashboard.

### Wizard flow

1. Page loads, shows a spinner while first `/api/status` call completes.
2. Services are grouped into three sections: **Platform**, **Services**, **User**.
3. Each service gets a colored dot: green (running), yellow (pending/creating), red (error/crashloop), gray (waiting).
4. A progress bar shows `X / Y services running` with percentage.
5. When all services are running, the wizard transitions to the "System Ready" screen showing:
   - Hosts file entry to copy (uses `node_ip` and `user_zone` from the API)
   - Username display
   - "Open Desktop" link pointing to `https://desktop.{user_zone}`

### Dashboard view

- **Metrics cards**: CPU, memory, disk usage with color-coded bars (green < 70%, yellow 70-90%, red > 90%)
- **GPU card**: Shown only if `gpu.available` is true in metrics response
- **Quick links**: Desktop, Settings, Market, Files — each links to `https://{service}.{user_zone}`
- **Installed apps**: List with status badges (running/stopped/error)

## API contracts

### `GET /api/status`

Returns system state and service list.

```json
{
  "activated": false,
  "username": "laurs",
  "user_zone": "laurs.olares.local",
  "node_ip": "192.168.1.100",
  "services": [
    { "name": "citus", "section": "platform", "status": "running" },
    { "name": "kvrocks", "section": "platform", "status": "pending" },
    { "name": "nats", "section": "platform", "status": "waiting" },
    { "name": "app-service", "section": "services", "status": "running" },
    { "name": "desktop", "section": "user", "status": "creating" }
  ]
}
```

Service `status` values: `running`, `pending`, `creating`, `waiting`, `error`, `crashloop`.

Service `section` values: `platform`, `services`, `user`.

### `GET /api/metrics`

Returns system resource usage.

```json
{
  "cpu_percent": 23.5,
  "cpu_cores": 8,
  "mem_percent": 45.2,
  "mem_used": 7700000000,
  "mem_total": 17179869184,
  "disk_percent": 31.0,
  "disk_used": 50000000000,
  "disk_total": 256000000000,
  "gpu": {
    "available": true,
    "model": "NVIDIA RTX 4090",
    "utilization": 12,
    "mem_used": 2147483648,
    "mem_total": 25769803776,
    "temperature": 42
  }
}
```

If no GPU is present, `gpu.available` is `false` or `gpu` is omitted entirely.

### `GET /api/apps`

Returns installed applications.

```json
{
  "apps": [
    {
      "name": "Stable Diffusion",
      "description": "AI image generation",
      "status": "running",
      "icon": "\ud83c\udfa8"
    }
  ]
}
```

App `status` values: `running`, `stopped`, `error`.

## Deployment

The Kubernetes manifest (`dashboard-deployment.yaml`) creates:

- **Namespace**: `packalares-dashboard`
- **Deployment**: Caddy 2.9 Alpine serving the static HTML
- **ConfigMaps**: `dashboard-html` (the page), `dashboard-caddyfile` (Caddy config)
- **Service**: ClusterIP on port 80

The main Caddy reverse proxy (separate component) routes `/` to this service and proxies `/api/*` calls to the backend API server.

### Environment variables

Available inside the pod via the downward API:

| Variable    | Source                       | Description                          |
|-------------|------------------------------|--------------------------------------|
| `USER_ZONE` | Pod annotation               | Full domain, e.g. `laurs.olares.local` |
| `USERNAME`  | Pod annotation               | Username, e.g. `laurs`               |
| `NODE_IP`   | `status.hostIP`              | Node IP address                      |

These are available for future server-side templating if needed. The current implementation gets all values from the `/api/status` response instead.

## Customizing the theme

All colors are defined as CSS custom properties in the `:root` selector at the top of `index.html`. Edit these to change the look:

```css
:root {
  --bg:          #0a0a0a;     /* page background */
  --bg-card:     #141414;     /* card background */
  --bg-hover:    #1a1a1a;     /* hover state */
  --border:      #222;        /* borders and dividers */
  --text:        #e0e0e0;     /* body text */
  --text-dim:    #888;        /* secondary text */
  --text-bright: #fff;        /* headings, emphasis */
  --accent:      #3b82f6;     /* links, progress bar, focus color */
  --accent-dim:  #1d4ed8;     /* accent hover state */
  --green:       #22c55e;     /* success / running */
  --yellow:      #eab308;     /* warning / pending */
  --red:         #ef4444;     /* error / critical */
  --radius:      10px;        /* border radius for cards */
  --font:        -apple-system, ...; /* font stack */
}
```

For a light theme, swap `--bg` to `#f5f5f5`, `--bg-card` to `#fff`, `--text` to `#333`, etc.

## File structure

```
dashboard/
  index.html                  # The entire SPA — HTML, CSS, JS in one file
  dashboard-deployment.yaml   # Kubernetes manifests
  README-dashboard.md         # This file
```
