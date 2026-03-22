# Session Summary — March 22, 2026

## What We Changed

### Repo Cleanup
- Removed 715MB of dead code: compiled binaries, empty directories, duplicate code
- 862MB → 147MB repo size
- Removed `core/`, `ui/`, 5 empty `internal/` dirs, 13 empty `pkg/` dirs, `cmd/daemon/`

### Centralized Config (`pkg/config/config.go`)
- All namespaces, service DNS, domains read from environment variables
- Defaults: `os-system` (platform), `os-framework` (framework), `monitoring`
- Zero hardcoded namespace strings in Go code (was 80+)
- Zero hardcoded namespaces in deploy manifests — all use `{{PLACEHOLDER}}` style
- Installer replaces placeholders from config before `kubectl apply`

### Deploy Manifests Fixed
- All image refs: `ghcr.io/packalares/*` (ours) or upstream (zero beclab)
- All `${REGISTRY}` variables removed
- All `${VARIABLE}` converted to `{{VARIABLE}}` convention
- Consistent naming: image names match GitHub Actions build matrix

### Phase 1: Infrastructure — was already complete
- containerd, etcd, K3s, Calico, OpenEBS, Helm — all written

### Phase 2: Platform
- KVRocks installer rewrote: host Redis → K8s KVRocks (`apache/kvrocks:2.15.0`)
- Created `deploy/platform/infisical.yaml` (Infisical v0.158.20 + dedicated Redis)
- Fixed all manifest namespaces and image tags
- Removed host Redis completely (zero references in codebase)

### Phase 3: Kubernetes Management
- Created `deploy/crds/crds-and-namespaces.yaml`: User, GlobalRole, GlobalRoleBinding CRDs + namespaces + RBAC
- Added installer step "Setup Kubernetes management" before service deploy
- Fixed installer service names to match actual images
- Fixed framework namespace: was using `PlatformNamespace()`, corrected to `FrameworkNamespace()`

### Phase 4: Framework
- Created `deploy/framework/kubesphere.yaml`
- Created `deploy/framework/tailscale.yaml` (always installed, configured via Settings)
- Fixed remaining wrong namespaces in files, mounts, auth, monitor manifests

### Phase 5: Monitoring + Logs
- Created `deploy/framework/loki.yaml` (Grafana Loki 2.9.4)
- Created `deploy/framework/promtail.yaml` (log collector DaemonSet)
- Created `deploy/framework/dcgm-exporter.yaml` (NVIDIA GPU metrics)
- Fixed `monitoring.yaml` namespace to `{{MONITORING_NAMESPACE}}`

### Phase 6: Security + DNS
- Created `deploy/framework/coredns-lan.yaml` (DNS forwarder on port 53 for LAN)
- Created `pkg/installer/phases/infisical.go` (seeds secrets after Infisical deploys)
- Added installer step "Seed secrets in Infisical"

### Phase 7: GPU
- Created `pkg/installer/phases/gpu.go` (auto-detect, install driver, container toolkit)
- Created `deploy/framework/hami.yaml` (HAMi v2.4.1 GPU sharing)
- Added installer step "Setup GPU"

### Phase 8: Post-install (partial)
- Created `internal/mdns/server.go` + `cmd/mdns/main.go` (mDNS agent)
- Created `deploy/framework/mdns.yaml` + `build/Dockerfile.mdns`
- Added to GitHub Actions + installer

### API Gateway
- Created `deploy/proxy/nginx.conf.template` — unified API under `/api/`
- Updated `frontend-app/src/boot/axios.ts` — `getApiBase()`, `getMainDomain()`, `getWsUrl()`
- Proxy translates clean external paths to internal service paths

---

## What Still Needs Wiring Up

### Frontend
- Vue pages still call old paths (`/market/v1/apps`, `/server/init`, `/bfl/backend/v1/user-info`)
- Need to update all API calls to use new `/api/` paths via `getApiBase()`
- Desktop, Market, Settings, Dashboard, Files, Login pages all need updating

### App Install Flow
- `internal/appservice/chartrewrite.go` was created by agent but needs review
- `service.go` `doInstall()` rewritten (no `--wait`, values injected into values.yaml)
- `helm.go` `InstallFromDir()` added (no `--set`)
- `k8s.go` Application CRD creation via dynamic client
- All created but reverted when user stopped — changes are lost, need to redo

### Installer Integration
- `pkg/installer/phases/deploy.go` reads manifest files + replaces placeholders — done
- But `generatePlatformManifest()` and `generateFrameworkManifest()` old functions may still be referenced
- Monitoring deploy step needs to include Loki + Promtail manifests
- DCGM exporter should only deploy if GPU detected (separate from monitoring step)

### Config File
- `/etc/packalares/config.yaml` doesn't exist yet — need to create the reader
- Currently everything reads from env vars via `pkg/config`
- Need a YAML parser that reads config.yaml and sets env vars at startup

### Setup Wizard
- No Vue page — needs creating
- First-time password set, NAS config, Tailscale auth key

### Phone Backup
- WebDAV exists in files service
- No Syncthing deployment
- No auto-discovery mechanism

### L4 Proxy
- Still in codebase (`cmd/l4proxy/`, `internal/l4proxy/`)
- Not needed for single-user — discuss whether to remove
