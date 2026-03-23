# Session Summary — March 23, 2026

## Install Status

**Full install completes successfully.** 32/32 pods running on clean server (188.241.210.104).

### Working Pods
- **Platform:** Citus (PostgreSQL), KVRocks, NATS, LLDAP, Infisical, Infisical-Redis
- **Framework:** auth, bfl, app-service, system-server, middleware-operator, files-server, market-backend, monitoring-server, mounts-server, kubesphere, samba-server, mdns-agent, proxy (nginx)
- **Monitoring:** Prometheus, node-exporter, kube-state-metrics
- **User apps:** desktop, wizard
- **Infrastructure:** Calico CNI, CoreDNS, OpenEBS, metrics-server

### Install Flow (20 phases)
1. Precheck → 2. Download binaries → 3. Kernel config → 4. containerd → 5. etcd → 6. K3s → 7. Calico CNI → 8. OpenEBS → 9. CRDs + namespaces → 10. KVRocks → 11. Helm → 12. Generate secrets (+ detect SERVER_IP, COREDNS_CLUSTER_IP) → 13. Platform services → 14. Framework services → 15. Seed Infisical → 16. User apps → 17. Monitoring → 18. GPU (if detected) → 19. Wait for pods → 20. Write release file

---

## What Changed (March 22-23)

### Repo & Build Pipeline
- Removed 715MB dead code, repo 862MB → 147MB
- GitHub Actions workflow builds 15 container images + CLI binary
- CLI binary published to GitHub Releases (not artifacts)
- Workflow triggers on: `internal/`, `cmd/`, `pkg/`, `frontend/`, `frontend-app/`, `build/`, `deploy/`, `install.sh`, `go.mod`, `go.sum`
- All deploy manifests embedded in CLI binary via `go:embed`

### Config System
- `pkg/config/config.go` — single source of truth, reads from `/etc/packalares/config.yaml` → env vars → defaults
- `config.yaml.template` with `{{PLACEHOLDER}}` values, rendered by installer from `--username`, `--domain`, `--tailscale-auth-key` flags
- `config.APIGroup()` — CRD API group configurable (default: `packalares.io`)
- `config.TLSSecretName()` — TLS secret name configurable
- `replaceConfigPlaceholders()` replaces all `{{VAR}}` in manifests from config at deploy time
- Auto-detect `SERVER_IP` (via `ip route`) and `COREDNS_CLUSTER_IP` (via `kubectl get svc kube-dns`)

### Olares Cleanup
- All `bytetrade.io` CRD groups → `{{API_GROUP}}` in YAML, `config.APIGroup()` in Go
- All `beclab/*` image references → `ghcr.io/packalares/*`
- All hardcoded namespaces → `{{PLACEHOLDER}}` or `config.*Namespace()`
- All hardcoded "laurs" → removed
- L4 proxy removed from deploy (port conflict with unified proxy)
- `beclab/apps` GitHub repo kept as app catalog source (intentional — public charts)

### Install Fixes
- **OpenEBS:** Matched Olares setup — v3.3.0, `kube-system` namespace, `openebs-maya-operator` SA, `OPENEBS_IO_HELPER_IMAGE` env
- **Calico:** Wait for CRDs before applying Installation CR
- **KVRocks:** `--log-dir stdout`, `emptyDir` volume
- **LLDAP:** `nitnelave/lldap:v0.5.0`, init container chowns `/data` to UID 1000, `?mode=rwc` on SQLite URL
- **Infisical:** Init containers wait for Postgres + Redis, seed via `kubectl exec` (not host HTTP), removed broken migration init container
- **Citus:** `{{PG_USER}}` placeholder, init script creates `infisical` database
- **Middleware-operator:** Use `{{PG_PASSWORD}}`/`{{REDIS_PASSWORD}}` placeholders instead of missing Secrets, fixed service name to `citus-coordinator-svc`
- **Tailscale:** `dnsPolicy: ClusterFirstWithHostNet`, skipped if no auth key
- **Proxy:** Inline nginx.conf.template into ConfigMap, TLS secret name from config, correct namespace (`os-framework`)
- **Desktop/wizard/monitoring:** Use embedded YAML files (`deploy/apps/`, `deploy/framework/monitoring.yaml`) instead of inline Go generators with `beclab/*` images
- **Namespace ordering:** CRDs + namespaces created before KVRocks (was after)
- **Cleanup:** `/var/openebs` and `/var/lib/packalares` removed on reinstall

### Frontend (Quasar Vue 3)
- All Material Symbols icons fixed to use `sym_r_` prefix with `<q-icon>`
- Launchpad: click-to-close via pointer-events passthrough, no fade transitions
- Desktop: macOS-style dock, wallpaper, daily info widget
- Login: background image, default avatar
- Settings: account avatar icon fixed
- `quasar build` output → `frontend/` → Docker image

---

## Still Needed

### Frontend Wiring
- Vue pages still call old API paths — need to use `/api/` routes via `getApiBase()`
- Desktop, Market, Settings, Dashboard, Files pages need API path updates

### App Install Flow
- `chartrewrite.go`, `doInstall()` rewrite — was done but reverted, needs redo
- Helm chart values injection (no `--set`, inject into values.yaml)

### Setup Wizard
- No Vue page for first-time setup (password, NAS, Tailscale key)

### Phone Backup
- WebDAV exists in files service
- No Syncthing deployment or auto-discovery

### Loki + Promtail
- Manifests exist (`deploy/framework/loki.yaml`, `deploy/framework/promtail.yaml`)
- Not in the deploy list yet — monitoring phase only deploys Prometheus stack

### CoreDNS LAN
- Manifest exists (`deploy/framework/coredns-lan.yaml`)
- Not in the deploy list yet

### DCGM Exporter
- Manifest exists (`deploy/framework/dcgm-exporter.yaml`)
- Should only deploy if GPU detected (separate from monitoring)

### L4 Proxy Code
- `cmd/l4proxy/` and `internal/l4proxy/` still in codebase
- Removed from deploy but code remains — can delete entirely
