# Caddy Reverse Proxy

Single Caddy instance that replaces five Olares networking components: L4 proxy, BFL nginx, OpenResty, Lua scripts, and token_auth.

## What This Does

Caddy runs as a Kubernetes DaemonSet in `packalares-system` with `hostNetwork: true`, so it binds directly to the node's ports 80 and 443. It terminates TLS with self-signed certificates (`tls internal`), routes requests to backend services, and delegates authentication to Authelia via `forward_auth`.

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 80   | HTTP     | Redirects and plain HTTP access |
| 443  | HTTPS    | TLS-terminated traffic to all services |

Both ports bind on the host network (not behind a NodePort or LoadBalancer).

## How Routing Works

There are two routing modes: hostname-based and path-based.

### Hostname-Based Routing

When DNS or `/etc/hosts` is configured, each service gets its own subdomain under `USER_ZONE`:

| Hostname | Backend Service |
|----------|----------------|
| `desktop.USER_ZONE` | `desktop-svc` in `user-space-USERNAME` |
| `settings.USER_ZONE` | `settings-svc` in `user-space-USERNAME` |
| `market.USER_ZONE` | `market-svc` in `user-space-USERNAME` |
| `files.USER_ZONE` | `files-svc` in `user-space-USERNAME` |
| `vault.USER_ZONE` | `vault-svc` in `user-space-USERNAME` |
| `auth.USER_ZONE` | `authelia-svc` in `packalares-auth` |
| `api.USER_ZONE` | `system-server` in `user-space-USERNAME` |
| `bfl.USER_ZONE` | `bfl` in `user-space-USERNAME` |
| `*.USER_ZONE` | Wildcard catch-all for user-installed apps |

The wildcard block extracts the subdomain label and proxies to `{app}-svc` in the user namespace, so any app installed as `foo` is reachable at `foo.USER_ZONE`.

### Path-Based Routing

When accessing by IP (no DNS configured), the node IP address block routes by path prefix:

| Path | Backend Service |
|------|----------------|
| `/desktop/*` | `desktop-svc` |
| `/market/*` | `market-svc` |
| `/settings/*` | `settings-svc` |
| `/files/*` | `files-svc` |
| `/api/*` | `system-server` |
| `/auth/*` | `authelia-svc` |
| `/` | Redirects to `/desktop/` |

Path prefixes are stripped before forwarding to the backend.

## How Auth Integration Works

Authentication uses Caddy's `forward_auth` directive, which calls Authelia before allowing a request through:

1. User hits `desktop.USER_ZONE`.
2. Caddy sends a subrequest to Authelia at `/api/verify`.
3. If Authelia returns 200, the request proceeds. Caddy copies `Remote-User`, `Remote-Groups`, `Remote-Name`, and `Remote-Email` headers from Authelia's response to the proxied request.
4. If Authelia returns 401, the user is redirected to `auth.USER_ZONE` to log in.

The auth snippet is defined once and imported into every route that needs protection. Routes that handle their own authentication (like `api.USER_ZONE` for system-server) do not import it.

## How to Add a New App Route

### Automatic (user-installed apps)

Apps installed through the marketplace are automatically routed by the wildcard block `*.USER_ZONE`. The only requirement is that the app's Kubernetes Service follows the naming convention:

- Service name: `{app}-svc`
- Namespace: `user-space-{USERNAME}`
- Port: `80`

The app is then accessible at `https://{app}.USER_ZONE`.

### Manual (new core service)

To add a new core service with its own hostname:

1. Edit `proxy/Caddyfile.tmpl` and add a new block:

```
newservice.${USER_ZONE}, https://newservice.${USER_ZONE} {
	tls internal

	import authelia

	reverse_proxy newservice-svc.user-space-${USERNAME}.svc.cluster.local:80
}
```

2. If it should also work via IP path-based routing, add a `handle` block inside the `${NODE_IP}` section:

```
handle /newservice/* {
	uri strip_prefix /newservice
	import authelia
	reverse_proxy newservice-svc.user-space-${USERNAME}.svc.cluster.local:80
}
```

3. Re-run `scripts/setup-caddy.sh` to regenerate the ConfigMap and restart Caddy.

## Environment Variables

| Variable | Required | Default | Example |
|----------|----------|---------|---------|
| `USER_ZONE` | Yes | - | `laurs.olares.local` |
| `USERNAME` | Yes | - | `laurs` |
| `NODE_IP` | Yes | - | `192.168.1.100` |
| `AUTH_SERVICE` | No | `authelia-svc.packalares-auth:9091` | - |
| `KUBECONFIG` | No | `/root/.kube/config` | - |

## Files

| File | Purpose |
|------|---------|
| `proxy/Caddyfile.tmpl` | Caddyfile template with `${VAR}` placeholders |
| `proxy/caddy-deployment.yaml` | Kubernetes DaemonSet + Namespace manifest |
| `scripts/setup-caddy.sh` | Deployment script: template, ConfigMap, apply, wait |

## Debugging

```bash
# Check pod status
kubectl get pods -n packalares-system -l app=caddy

# View Caddy logs
kubectl logs -n packalares-system -l app=caddy

# View the rendered Caddyfile
kubectl get configmap caddy-config -n packalares-system -o jsonpath='{.data.Caddyfile}'

# Reload Caddy without restart (after updating ConfigMap)
kubectl exec -n packalares-system -l app=caddy -- caddy reload --config /etc/caddy/Caddyfile
```
