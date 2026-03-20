#!/bin/bash
# ==========================================================================
# Packalares — Master Installer
# ==========================================================================
# Orchestrates all modules to install a self-hosted personal cloud OS.
#
# Usage:
#   Interactive:   bash install.sh
#   Non-interactive:
#     PACKALARES_USER=alice PACKALARES_PASSWORD=secret bash install.sh
#   Via curl:
#     curl -fsSL https://raw.githubusercontent.com/packalares/packalares/main/install.sh | bash
#
# Idempotent — safe to re-run. Each module checks its own state and skips
# if already installed.
# ==========================================================================

set -euo pipefail

# --------------------------------------------------------------------------
# Constants
# --------------------------------------------------------------------------
REPO_OWNER="packalares"
REPO_NAME="packalares"
REPO="${REPO_OWNER}/${REPO_NAME}"
REPO_RAW="https://raw.githubusercontent.com/${REPO}/main"
REPO_RELEASES="https://github.com/${REPO}/releases/latest/download"

# Where we are — detect git clone vs curl-pipe
SCRIPT_DIR=""
if [ -f "${BASH_SOURCE[0]:-}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

# --------------------------------------------------------------------------
# Colors / helpers
# --------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; }
die()   { err "$@"; exit 1; }
step()  { echo ""; echo -e "${BOLD}── Step $1: $2${NC}"; }

# --------------------------------------------------------------------------
# Root check
# --------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
    die "This installer must be run as root (sudo bash install.sh)"
fi

# --------------------------------------------------------------------------
# Banner
# --------------------------------------------------------------------------
echo ""
echo "=========================================="
echo "  Packalares Installer"
echo "=========================================="
echo ""

# --------------------------------------------------------------------------
# Step 1: Read configuration
# --------------------------------------------------------------------------
step "1/8" "Configuration"

# Accept both PACKALARES_ and legacy OLARES_ prefixes
export PACKALARES_USER="${PACKALARES_USER:-${OLARES_USER:-}}"
export PACKALARES_PASSWORD="${PACKALARES_PASSWORD:-${OLARES_PASSWORD:-}}"
export PACKALARES_DOMAIN="${PACKALARES_DOMAIN:-olares.local}"

# Prompt interactively if not set
if [ -z "$PACKALARES_USER" ]; then
    read -r -p "  Username: " PACKALARES_USER
    [ -z "$PACKALARES_USER" ] && die "Username is required"
    export PACKALARES_USER
fi

if [ -z "$PACKALARES_PASSWORD" ]; then
    read -r -s -p "  Password: " PACKALARES_PASSWORD
    echo ""
    [ -z "$PACKALARES_PASSWORD" ] && die "Password is required"
    export PACKALARES_PASSWORD
fi

# Derived values
export NODE_IP
NODE_IP="$(hostname -I | awk '{print $1}')"

export USER_ZONE="${PACKALARES_USER}.${PACKALARES_DOMAIN}"
export KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

# GPU detection
export PACKALARES_GPU="false"
if command -v nvidia-smi &>/dev/null && nvidia-smi &>/dev/null; then
    PACKALARES_GPU="true"
    info "NVIDIA GPU detected"
fi

# OS version
export PACKALARES_OS
PACKALARES_OS="$(. /etc/os-release 2>/dev/null && echo "${ID}-${VERSION_ID}" || uname -s)"

# Version from repo
export PACKALARES_VERSION
if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/VERSION" ]; then
    PACKALARES_VERSION="$(cat "$SCRIPT_DIR/VERSION" | tr -d '[:space:]')"
else
    PACKALARES_VERSION="1.12.6-20260317"
fi

info "User:     $PACKALARES_USER"
info "Domain:   $USER_ZONE"
info "Node IP:  $NODE_IP"
info "GPU:      $PACKALARES_GPU"
info "OS:       $PACKALARES_OS"
info "Version:  $PACKALARES_VERSION"

# --------------------------------------------------------------------------
# Helper: resolve a module script path
# --------------------------------------------------------------------------
resolve_script() {
    local name="$1"
    if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/scripts/$name" ]; then
        echo "$SCRIPT_DIR/scripts/$name"
    else
        # Running via curl — download to temp
        local tmp="/tmp/packalares-$name"
        if [ ! -f "$tmp" ]; then
            info "Downloading scripts/$name ..."
            curl -fsSL "$REPO_RAW/scripts/$name" -o "$tmp"
            chmod +x "$tmp"
        fi
        echo "$tmp"
    fi
}

# --------------------------------------------------------------------------
# Helper: resolve a manifest path
# --------------------------------------------------------------------------
resolve_manifest() {
    local relpath="$1"
    if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/$relpath" ]; then
        echo "$SCRIPT_DIR/$relpath"
    else
        local tmp="/tmp/packalares-$(basename "$relpath")"
        if [ ! -f "$tmp" ]; then
            info "Downloading $relpath ..."
            curl -fsSL "$REPO_RAW/$relpath" -o "$tmp"
        fi
        echo "$tmp"
    fi
}

# --------------------------------------------------------------------------
# Step 2: System detection and prerequisites
# --------------------------------------------------------------------------
step "2/8" "System prerequisites"

# Minimal packages
for pkg in curl openssl jq; do
    if ! command -v "$pkg" &>/dev/null; then
        info "Installing $pkg ..."
        apt-get update -qq && apt-get install -y -qq "$pkg" >/dev/null 2>&1 || true
    fi
done
ok "Prerequisites satisfied"

# --------------------------------------------------------------------------
# Step 3: Install K3s (Kubernetes)
# --------------------------------------------------------------------------
step "3/8" "Kubernetes (K3s)"

SETUP_K3S="$(resolve_script setup-k3s.sh)"
info "Running $SETUP_K3S"
bash "$SETUP_K3S"

# Ensure kubeconfig is available
export KUBECONFIG="/etc/rancher/k3s/k3s.yaml"
if [ -f "$KUBECONFIG" ]; then
    ok "K3s is running, kubeconfig at $KUBECONFIG"
else
    die "K3s install failed — kubeconfig not found"
fi

# Wait for K3s node to be ready
info "Waiting for K3s node to be ready ..."
for i in $(seq 1 60); do
    if kubectl get nodes 2>/dev/null | grep -q " Ready"; then
        break
    fi
    sleep 5
done
kubectl get nodes 2>/dev/null | grep -q " Ready" || die "K3s node never became Ready"
ok "K3s node is Ready"

# --------------------------------------------------------------------------
# Step 4: Install Auth stack (Authelia + LLDAP + Redis)
# --------------------------------------------------------------------------
step "4/8" "Authentication (Authelia + LLDAP + Redis)"

SETUP_AUTH="$(resolve_script setup-auth.sh)"
info "Running $SETUP_AUTH"
bash "$SETUP_AUTH"

ok "Auth stack deployed"

# --------------------------------------------------------------------------
# Step 5: Platform services (PostgreSQL, NATS, MinIO)
# --------------------------------------------------------------------------
step "5/8" "Platform services (PostgreSQL, NATS, MinIO)"

SETUP_PLATFORM="$(resolve_script setup-platform.sh)"
info "Running $SETUP_PLATFORM"
bash "$SETUP_PLATFORM"

ok "Platform services deployed"

# --------------------------------------------------------------------------
# Step 6: Install reverse proxy (Caddy)
# --------------------------------------------------------------------------
step "6/8" "Reverse proxy (Caddy)"

SETUP_CADDY="$(resolve_script setup-caddy.sh)"
info "Running $SETUP_CADDY"
bash "$SETUP_CADDY"

ok "Caddy deployed"

# --------------------------------------------------------------------------
# Step 7: Deploy core applications
# --------------------------------------------------------------------------
step "7/8" "Core applications"

# 7a. App-service
APP_SERVICE_MANIFEST="$(resolve_manifest app-service/app-service-deployment.yaml)"
if [ -f "$APP_SERVICE_MANIFEST" ]; then
    if kubectl get deployment app-service -n packalares-system &>/dev/null; then
        ok "app-service already deployed — skipping"
    else
        info "Deploying app-service ..."
        envsubst < "$APP_SERVICE_MANIFEST" | kubectl apply -f -
        ok "app-service deployed"
    fi
else
    warn "app-service manifest not found — skipping"
fi

# 7b. Dashboard
DASHBOARD_MANIFEST="$(resolve_manifest dashboard/dashboard-deployment.yaml)"
if [ -f "$DASHBOARD_MANIFEST" ]; then
    if kubectl get deployment dashboard -n packalares-system &>/dev/null; then
        ok "dashboard already deployed — skipping"
    else
        info "Deploying dashboard ..."
        envsubst < "$DASHBOARD_MANIFEST" | kubectl apply -f -
        ok "dashboard deployed"
    fi
else
    warn "dashboard manifest not found — skipping"
fi

# --------------------------------------------------------------------------
# Step 8: Activate user account
# --------------------------------------------------------------------------
step "8/8" "User activation"

ACTIVATE_SCRIPT="$(resolve_script activate.sh)"
info "Running $ACTIVATE_SCRIPT"
bash "$ACTIVATE_SCRIPT"

ok "User '$PACKALARES_USER' activated"

# --------------------------------------------------------------------------
# Done — print access info
# --------------------------------------------------------------------------
echo ""
echo "=========================================="
echo -e "  ${GREEN}Packalares is ready!${NC}"
echo "=========================================="
echo ""
echo "  Add this line to your hosts file (/etc/hosts on Linux/Mac,"
echo "  C:\\Windows\\System32\\drivers\\hosts on Windows):"
echo ""
echo "    $NODE_IP  $USER_ZONE desktop.$USER_ZONE auth.$USER_ZONE settings.$USER_ZONE market.$USER_ZONE files.$USER_ZONE"
echo ""
echo "  Then open:"
echo "    https://desktop.$USER_ZONE"
echo ""
echo "  Login:"
echo "    Username: $PACKALARES_USER"
echo "    Password: (the password you provided)"
echo ""
echo "  Or access directly by IP:"
echo "    https://$NODE_IP  (path-based routing)"
echo ""
echo "  If mDNS was set up, .local domains resolve automatically"
echo "  on the same LAN — no hosts file needed."
echo ""
