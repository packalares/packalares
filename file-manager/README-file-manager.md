# File Manager

Web-based file manager for Packalares. Lets users browse, upload, download, and manage files on the server. Supports mounting external storage (SMB/NFS, S3, Google Drive via rclone).

## Access

- Via subdomain: `https://files.USER_ZONE`
- Via IP path: `https://NODE_IP/files/`
- Caddy routes to the `files-svc` service on port 80

## Architecture

Single Go binary serving both the API and embedded UI. The HTML/CSS/JS frontend is compiled into the binary via `go:embed`.

```
file-manager/
  main.go          — HTTP server, config, embedded static files
  handlers.go      — all API handlers and file operations
  index.html       — single-page file browser UI
  Dockerfile       — multi-stage build (Go builder + Alpine runtime)
  file-manager-deployment.yaml — Kubernetes Deployment + Service
  go.mod
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/files/list?path=...` | List directory contents |
| GET | `/api/files/download?path=...` | Download a file |
| POST | `/api/files/upload` | Upload file (multipart form, fields: `path`, `file`) |
| POST | `/api/files/mkdir` | Create directory (`{"path": "..."}`) |
| POST | `/api/files/delete` | Delete file/directory (`{"path": "..."}`) |
| POST | `/api/files/move` | Move/rename (`{"source": "...", "destination": "..."}`) |
| POST | `/api/files/copy` | Copy (`{"source": "...", "destination": "..."}`) |
| GET | `/api/files/info?path=...` | File/directory metadata |
| GET | `/api/storage/mounts` | List mounted external storages |
| POST | `/api/storage/mount` | Mount external storage |
| POST | `/api/storage/unmount` | Unmount (`{"id": "..."}`) |
| GET | `/healthz` | Health check |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATA_PATH` | `/packalares/data` | Root directory for file operations |
| `UPLOAD_MAX_SIZE` | `10G` | Maximum upload size (supports human-readable: `10G`, `500M`) |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |

## Build

```bash
docker build -t packalares/file-manager:latest .
```

## Deploy

```bash
kubectl apply -f file-manager-deployment.yaml
```

The deployment:
- Mounts the host path `/packalares/data` into the container
- Runs with `privileged: true` and `SYS_ADMIN` capability for mount operations (SMB/NFS)
- Includes rclone, cifs-utils, and nfs-utils in the container image for external storage support
- Exposes on port 80 via `files-svc` ClusterIP service in the `user-space-${USERNAME}` namespace (matching Caddy routing)
- The `${USERNAME}` placeholder is substituted by the install script at deploy time

## UI Features

- Dark theme matching the Packalares dashboard
- List and grid view toggle (persisted in localStorage)
- Breadcrumb navigation with click-to-navigate
- Drag-and-drop file upload with progress bar
- Right-click context menu: open, download, rename, copy, move, delete
- Keyboard shortcuts: Delete key, Backspace to go up
- Storage mounts sidebar panel with mount/unmount controls
- Responsive layout for mobile

## Mount Types

| Type | Source Format | Example |
|------|--------------|---------|
| `smb` / `cifs` | `//host/share` | `//192.168.1.100/media` |
| `nfs` | `host:/export` | `nas.local:/volume1/data` |
| `rclone` | rclone remote path | `gdrive:Documents` or `s3:my-bucket` |

## Security

- All file paths are validated against `DATA_PATH` to prevent path traversal
- The data root directory cannot be deleted
- Filenames are sanitized on upload
- Auth is handled by Caddy's forward_auth to Authelia (not in this service)
