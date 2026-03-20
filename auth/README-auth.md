# Packalares Authentication Module

## Overview

Packalares uses [Authelia](https://www.authelia.com/) (stock upstream, not a fork) as the central authentication gateway. Every HTTP request to any Packalares service is verified by Authelia before it reaches the backend. This happens transparently through Caddy's `forward_auth` directive.

Three services work together:

| Service  | Purpose                        | Namespace          | Internal endpoint                          |
|----------|--------------------------------|--------------------|---------------------------------------------|
| Authelia | Authentication & authorization | packalares-auth    | authelia-svc.packalares-auth:9091           |
| LLDAP    | User directory (LDAP)          | packalares-auth    | lldap-svc.packalares-auth:3890 (LDAP)      |
| Redis    | Session storage                | packalares-auth    | redis-svc.packalares-auth:6379             |

## How Authentication Works

1. A user navigates to any Packalares subdomain (e.g. `https://desktop.laurs.olares.local/`).
2. Caddy intercepts the request and calls Authelia's verification endpoint at `authelia-svc.packalares-auth:9091/api/authz/forward-auth`.
3. Authelia checks the session cookie (`packalares_session`).
   - If the cookie is valid and the session is active, Authelia returns 200 and Caddy forwards the request to the backend.
   - If the cookie is missing or expired, Authelia returns 401 and Caddy redirects the user to the login page at `https://auth.USER_ZONE/`.
4. The user logs in with username and password (and optionally TOTP 2FA).
5. Authelia validates credentials against LLDAP, creates a session in Redis, sets a cookie, and redirects back to the original URL.

All redirects use HTTPS. The login URL is always `https://auth.USER_ZONE/`.

## How Session Cookies Work

Authelia sets a cookie named `packalares_session` scoped to the `USER_ZONE` domain (e.g. `.laurs.olares.local`). Because it is set on the parent domain, it is automatically sent by the browser to all subdomains:

- `desktop.laurs.olares.local`
- `files.laurs.olares.local`
- `settings.laurs.olares.local`
- `market.laurs.olares.local`
- Any app installed at `*.laurs.olares.local`

This means a single login covers every service. Session data is stored in Redis (not in the cookie itself). Default timeouts:

| Setting    | Duration |
|------------|----------|
| Expiration | 24 hours |
| Inactivity | 4 hours  |
| Remember me| 30 days  |

## Adding Users

Users are managed in LLDAP. There are two ways to add a user:

### Via LLDAP Web UI

LLDAP has a built-in web interface at `http://lldap-svc.packalares-auth:17170` (cluster-internal). To access it from outside the cluster, port-forward:

```bash
kubectl port-forward svc/lldap-svc -n packalares-auth 17170:17170
```

Then open `http://localhost:17170` and log in as `admin` with the LLDAP admin password.

### Via the LLDAP GraphQL API

```bash
# Get the LLDAP cluster IP
LLDAP_IP=$(kubectl get svc lldap-svc -n packalares-auth -o jsonpath='{.spec.clusterIP}')

# Get the admin password
LLDAP_PASS=$(kubectl get secret lldap-credentials -n packalares-auth \
    -o jsonpath='{.data.admin-password}' | base64 -d)

# Authenticate
TOKEN=$(curl -s -X POST "http://$LLDAP_IP:17170/auth/simple/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$LLDAP_PASS\"}" | \
    python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')

# Create a user
curl -s -X POST "http://$LLDAP_IP:17170/api/graphql" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"query":"mutation { createUser(user: {id: \"newuser\", email: \"newuser@olares.local\", displayName: \"New User\"}) { id } }"}'

# Set the password
curl -s -X POST "http://$LLDAP_IP:17170/api/graphql" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"query":"mutation { modifyUser(user: {id: \"newuser\", password: \"securepassword\"}) { ok } }"}'
```

## TOTP Two-Factor Authentication

Authelia supports TOTP (Time-based One-Time Passwords) compatible with Google Authenticator, Authy, and any TOTP app.

### How it works

1. TOTP is enabled in the Authelia configuration by default (`totp.disable: false`).
2. The default access control policy is `one_factor` (password only). To require 2FA for specific services, change the policy to `two_factor` in the Authelia config for those domains.
3. When a user first encounters a `two_factor` policy, Authelia prompts them to register a TOTP device.
4. The user scans a QR code with their authenticator app.
5. On subsequent logins, the user enters their password plus the 6-digit TOTP code.

### Enabling 2FA for all services

Edit the Authelia config template at `auth/authelia-config.yaml.tmpl` and change the default policy:

```yaml
access_control:
  default_policy: two_factor
```

Then re-run `setup-auth.sh` to apply the change.

### TOTP registration notifications

Since Packalares uses a filesystem notifier (no email server), TOTP registration links are written to a file inside the Authelia container:

```bash
kubectl exec -n packalares-auth deploy/authelia -- cat /data/notifications.txt
```

This file contains the registration URL that the user must open to set up their TOTP device.

## Configuration

All configuration is driven by environment variables, nothing is hardcoded:

| Variable             | Required | Description                          | Example                |
|----------------------|----------|--------------------------------------|------------------------|
| USERNAME             | Yes      | Primary user login                   | laurs                  |
| PASSWORD             | Yes      | Primary user password                | (your password)        |
| USER_ZONE            | Yes      | Full user domain                     | laurs.olares.local     |
| DOMAIN               | Yes      | Base domain                          | olares.local           |
| AUTH_SECRET           | No       | HMAC secret (auto-generated)         | (64 hex chars)         |
| LLDAP_ADMIN_PASSWORD  | No       | LLDAP admin password (auto-generated)| (64 hex chars)         |
| REDIS_PASSWORD        | No       | Redis password (auto-generated)      | (64 hex chars)         |

## Integration with Caddy

The proxy module (at `/home/laurs/packalares/proxy/`) configures Caddy to use Authelia's forward-auth endpoint. For every protected route, Caddy includes a directive like:

```caddyfile
forward_auth authelia-svc.packalares-auth:9091 {
    uri /api/authz/forward-auth
    copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
}
```

Caddy sends the original request headers to Authelia. If Authelia returns 200, the request proceeds. If Authelia returns 401/302, Caddy redirects the user to the login page.

## File Layout

```
auth/
  authelia-config.yaml.tmpl   — Authelia config with placeholder variables
  authelia-deployment.yaml    — Authelia Kubernetes manifest
  lldap-deployment.yaml       — LLDAP Kubernetes manifest
  redis-deployment.yaml       — Redis Kubernetes manifest
  README-auth.md              — This file

scripts/
  setup-auth.sh               — Deploys everything, creates user, renders config
```

## Troubleshooting

Check pod status:
```bash
kubectl get pods -n packalares-auth
```

Check Authelia logs:
```bash
kubectl logs -n packalares-auth deploy/authelia
```

Check LLDAP logs:
```bash
kubectl logs -n packalares-auth deploy/lldap
```

Verify the rendered Authelia config:
```bash
kubectl get configmap authelia-config -n packalares-auth -o jsonpath='{.data.configuration\.yml}'
```

Test the auth endpoint directly:
```bash
kubectl exec -n packalares-auth deploy/authelia -- wget -qO- http://localhost:9091/api/health
```
