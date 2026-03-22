# Packalares Architecture

## System Flow

```
Browser → Nginx Proxy (443, hostNetwork)
            ├── auth_request → Auth Service (sessions, TOTP)
            ├── System apps: desktop/market/settings/files/dashboard
            └── Wildcard *.zone → System-Server (Go reverse proxy) → Installed Apps

Platform:  Citus (PostgreSQL) | KVRocks (Redis) | NATS | LLDAP | Infisical (+own Redis)
           └── Middleware Operator watches MiddlewareRequest CRDs, provisions per-app

App Install:
  1. Chart synced from Olares/CasaOS → rewritten → stored locally
  2. helm install (no --wait) → creates Deployment + Service + MiddlewareRequest
  3. Middleware operator provisions DB/Redis → stores creds in K8s Secret
  4. App pod starts → reads Secret → works
  5. Application CRD created → system-server routes subdomain
```

## Services

| Service | Port | Purpose |
|---------|------|---------|
| auth | 9091 | Login, sessions, TOTP 2FA |
| bfl | 8080 | User info gateway |
| appservice | 6755 | App install/uninstall, desktop APIs, WebSocket |
| systemserver | 8080 | Watches Application CRDs, reverse proxy to apps |
| middleware | - | Watches MiddlewareRequest CRDs, provisions DB/Redis/NATS |
| market | 6756 | App catalog (local cache, multi-source) |
| files | 80 | File CRUD, thumbnails, upload |
| monitor | 8000 | Prometheus queries, system metrics, GPU |
| mounts | 8080 | SMB/NFS/rclone mount management |
| kubesphere | 9090 | User CRD API |
| l4proxy | 443 | L4 TLS SNI proxy |
| desktop | 80 | Quasar Vue 3 SPA |
| samba | 445 | SMB file sharing |

## Images

All on ghcr.io/packalares/ — zero beclab images.
Upstream: Citus, Redis, KVRocks, LLDAP, NATS, Prometheus, Infisical, Nginx.
