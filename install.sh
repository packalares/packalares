#!/bin/bash
set -e

echo "================================"
echo "  Packalares Installer"
echo "================================"
echo ""

# Read config or use defaults
CONFIG_FILE="${PACKALARES_CONFIG:-./config.yaml}"
USERNAME="${PACKALARES_USER:-}"
PASSWORD="${PACKALARES_PASSWORD:-}"
DOMAIN="${PACKALARES_DOMAIN:-olares.local}"
NODE_IP=$(hostname -I | awk '{print $1}')

if [ -z "$USERNAME" ]; then
    read -p "Username: " USERNAME
fi
if [ -z "$PASSWORD" ]; then
    read -s -p "Password: " PASSWORD
    echo ""
fi

USER_ZONE="${USERNAME}.${DOMAIN}"

echo ""
echo "Server:   $NODE_IP"
echo "User:     $USERNAME"
echo "Domain:   $USER_ZONE"
echo ""

# ============================================================
# Phase 1: System binaries
# ============================================================
echo "[1/6] Installing system binaries..."

# Download K3s, containerd, etcd, helm, etc.
# (reuse olares-cli download or direct URLs from images.yaml)
# TODO: implement

# ============================================================
# Phase 2: Container runtime + K3s
# ============================================================
echo "[2/6] Starting Kubernetes..."

# Install containerd, start K3s, wait for cluster ready
# TODO: implement

# ============================================================
# Phase 3: Networking + Storage
# ============================================================
echo "[3/6] Setting up networking and storage..."

# Deploy Calico CNI, OpenEBS, CoreDNS
# TODO: implement

# ============================================================
# Phase 4: Caddy reverse proxy
# ============================================================
echo "[4/6] Setting up reverse proxy..."

# Generate Caddyfile from config
# Deploy Caddy as DaemonSet on port 80/443
# Auto HTTPS with self-signed cert for $USER_ZONE
# IP access works out of the box
# TODO: implement

# ============================================================
# Phase 5: Platform services
# ============================================================
echo "[5/6] Deploying platform services..."

# Deploy: Citus, KVRocks, NATS, LLDAP, OPA
# Deploy: Auth (Authelia), middleware-operator, KubeBlocks
# Deploy: App-service, system-server, monitoring
# Deploy: Frontend (desktop, settings, market, files)
# Deploy: Wizard
# All images from images.yaml
# TODO: implement

# ============================================================
# Phase 6: Activate
# ============================================================
echo "[6/6] Activating system..."

# Set LLDAP password
# Generate TLS cert → zone-ssl-config configmap
# Set user annotations
# Patch authelia config with real domain
# Trigger L4 proxy generation (or Caddy route)
# TODO: implement

echo ""
echo "================================"
echo "  Installation complete!"
echo "================================"
echo ""
echo "Open: https://$NODE_IP or https://desktop.$USER_ZONE"
echo ""
echo "Add to hosts file:"
echo "  $NODE_IP  desktop.$USER_ZONE  auth.$USER_ZONE  settings.$USER_ZONE  market.$USER_ZONE  files.$USER_ZONE  $USER_ZONE"
echo ""
echo "Login: $USERNAME / (your password)"
echo ""
