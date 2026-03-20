#!/bin/bash
set -euo pipefail

# ============================================================
# setup-caddy.sh — Deploy Caddy reverse proxy to Kubernetes
#
# Replaces 5 Olares components:
#   L4 proxy, BFL nginx, OpenResty, Lua scripts, token_auth
#
# Env vars (required):
#   USER_ZONE  — user's zone (e.g. laurs.olares.local)
#   USERNAME   — system username (e.g. laurs)
#   NODE_IP    — host IP address
#
# Env vars (optional):
#   AUTH_SERVICE — authelia address (default: authelia-svc.packalares-auth:9091)
#   KUBECONFIG  — path to kubeconfig (default: /root/.kube/config)
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROXY_DIR="$PROJECT_DIR/proxy"

# Validate required env vars
for var in USER_ZONE USERNAME NODE_IP; do
    if [ -z "${!var:-}" ]; then
        echo "ERROR: $var is not set" >&2
        exit 1
    fi
done

# Defaults
export AUTH_SERVICE="${AUTH_SERVICE:-authelia-svc.packalares-auth:9091}"
export KUBECONFIG="${KUBECONFIG:-/root/.kube/config}"

echo "Setting up Caddy reverse proxy..."
echo "  User zone:  $USER_ZONE"
echo "  Username:   $USERNAME"
echo "  Node IP:    $NODE_IP"
echo "  Auth:       $AUTH_SERVICE"

# ============================================================
# Step 1: Create namespace
# ============================================================
echo ""
echo "Creating namespace packalares-system..."

kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: packalares-system
  labels:
    app.kubernetes.io/part-of: packalares
EOF

# ============================================================
# Step 2: Generate Caddyfile from template
# ============================================================
echo "Generating Caddyfile..."

CADDYFILE_TMPL="$PROXY_DIR/Caddyfile.tmpl"

if [ ! -f "$CADDYFILE_TMPL" ]; then
    echo "ERROR: Template not found: $CADDYFILE_TMPL" >&2
    exit 1
fi

CADDYFILE_RENDERED=$(envsubst '${USER_ZONE} ${USERNAME} ${NODE_IP} ${AUTH_SERVICE}' < "$CADDYFILE_TMPL")

# ============================================================
# Step 3: Create/update ConfigMap with the rendered Caddyfile
# ============================================================
echo "Creating ConfigMap..."

# Delete existing configmap if present (idempotent)
kubectl delete configmap caddy-config -n packalares-system --ignore-not-found

kubectl create configmap caddy-config \
    -n packalares-system \
    --from-literal=Caddyfile="$CADDYFILE_RENDERED"

# ============================================================
# Step 4: Deploy Caddy DaemonSet
# ============================================================
echo "Deploying Caddy DaemonSet..."

kubectl apply -f "$PROXY_DIR/caddy-deployment.yaml"

# ============================================================
# Step 5: Wait for Caddy to be ready
# ============================================================
echo "Waiting for Caddy to be ready..."

TIMEOUT=120
INTERVAL=5
ELAPSED=0

while [ $ELAPSED -lt $TIMEOUT ]; do
    DESIRED=$(kubectl get daemonset caddy -n packalares-system -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo "0")
    READY=$(kubectl get daemonset caddy -n packalares-system -o jsonpath='{.status.numberReady}' 2>/dev/null || echo "0")

    if [ "$DESIRED" != "0" ] && [ "$DESIRED" = "$READY" ]; then
        echo "Caddy is ready ($READY/$DESIRED pods running)"
        break
    fi

    echo "  Waiting... ($READY/$DESIRED ready, ${ELAPSED}s/${TIMEOUT}s)"
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
    echo "WARNING: Caddy did not become ready within ${TIMEOUT}s" >&2
    echo "  Check: kubectl get pods -n packalares-system -l app=caddy" >&2
    exit 1
fi

echo ""
echo "Caddy reverse proxy deployed successfully."
echo ""
echo "  HTTP:   http://$NODE_IP"
echo "  HTTPS:  https://$NODE_IP"
echo "  Zone:   https://desktop.$USER_ZONE"
echo ""
