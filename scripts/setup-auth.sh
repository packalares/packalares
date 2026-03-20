#!/bin/bash
###############################################################
# setup-auth.sh — Deploy Authelia + LLDAP + Redis
#
# Deploys stock upstream Authelia as the authentication layer.
# Caddy calls authelia-svc.packalares-auth:9091 via forward_auth.
#
# Required env vars:
#   USERNAME       — primary user login name
#   PASSWORD       — primary user password
#   USER_ZONE      — e.g. laurs.olares.local
#   DOMAIN         — e.g. olares.local
#
# Optional env vars (auto-generated if unset):
#   AUTH_SECRET          — HMAC secret for JWT/sessions/encryption
#   LLDAP_ADMIN_PASSWORD — LLDAP admin password
#   REDIS_PASSWORD       — Redis password
###############################################################
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AUTH_DIR="$(cd "$SCRIPT_DIR/../auth" && pwd)"

echo "Setting up authentication..."

# -----------------------------------------------------------
# Validate required variables
# -----------------------------------------------------------
for var in USERNAME PASSWORD USER_ZONE DOMAIN; do
    if [ -z "${!var:-}" ]; then
        echo "ERROR: $var is not set" >&2
        exit 1
    fi
done

# -----------------------------------------------------------
# Generate secrets if not provided
# -----------------------------------------------------------
generate_secret() {
    openssl rand -hex 32
}

AUTH_SECRET="${AUTH_SECRET:-$(generate_secret)}"
LLDAP_ADMIN_PASSWORD="${LLDAP_ADMIN_PASSWORD:-$(generate_secret)}"
REDIS_PASSWORD="${REDIS_PASSWORD:-$(generate_secret)}"
LLDAP_JWT_SECRET="$(generate_secret)"

echo "  Secrets ready"

# -----------------------------------------------------------
# Create namespace
# -----------------------------------------------------------
kubectl create namespace packalares-auth --dry-run=client -o yaml | kubectl apply -f -

# -----------------------------------------------------------
# Deploy Redis
# -----------------------------------------------------------
echo "  Deploying Redis..."

# Create Redis secret
kubectl create secret generic redis-credentials \
    --namespace packalares-auth \
    --from-literal=password="$REDIS_PASSWORD" \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f "$AUTH_DIR/redis-deployment.yaml"

# Wait for Redis to be ready
kubectl rollout status deployment/redis -n packalares-auth --timeout=120s

echo "  Redis running"

# -----------------------------------------------------------
# Deploy LLDAP
# -----------------------------------------------------------
echo "  Deploying LLDAP..."

# Create LLDAP secret
kubectl create secret generic lldap-credentials \
    --namespace packalares-auth \
    --from-literal=admin-password="$LLDAP_ADMIN_PASSWORD" \
    --from-literal=jwt-secret="$LLDAP_JWT_SECRET" \
    --dry-run=client -o yaml | kubectl apply -f -

# Substitute USER_ZONE in LLDAP deployment and apply
sed "s/__USER_ZONE__/$USER_ZONE/g" "$AUTH_DIR/lldap-deployment.yaml" | kubectl apply -f -

# Wait for LLDAP to be ready
kubectl rollout status deployment/lldap -n packalares-auth --timeout=180s

echo "  LLDAP running"

# -----------------------------------------------------------
# Create primary user in LLDAP
# -----------------------------------------------------------
echo "  Creating user $USERNAME..."

LLDAP_IP=$(kubectl get svc lldap-svc -n packalares-auth -o jsonpath='{.spec.clusterIP}')

# Wait for LLDAP HTTP API to respond
for i in $(seq 1 30); do
    if curl -sf "http://$LLDAP_IP:17170/health" >/dev/null 2>&1; then
        break
    fi
    sleep 2
done

# Get admin token
ADMIN_TOKEN=$(curl -sf -X POST "http://$LLDAP_IP:17170/auth/simple/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$LLDAP_ADMIN_PASSWORD\"}" | \
    python3 -c 'import sys,json; print(json.load(sys.stdin).get("token",""))' 2>/dev/null)

if [ -z "$ADMIN_TOKEN" ]; then
    echo "  WARNING: Could not get LLDAP admin token, user creation deferred" >&2
else
    # Create user
    CREATE_RESULT=$(curl -sf -X POST "http://$LLDAP_IP:17170/api/graphql" \
        -H 'Content-Type: application/json' \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -d "{\"query\":\"mutation { createUser(user: {id: \\\"$USERNAME\\\", email: \\\"$USERNAME@$DOMAIN\\\", displayName: \\\"$USERNAME\\\"}) { id } }\"}" 2>/dev/null || echo "")

    # Set password (works even if user already exists)
    curl -sf -X POST "http://$LLDAP_IP:17170/api/graphql" \
        -H 'Content-Type: application/json' \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -d "{\"query\":\"mutation { modifyUser(user: {id: \\\"$USERNAME\\\", password: \\\"$PASSWORD\\\"}) { ok } }\"}" >/dev/null 2>&1 || true

    echo "  User $USERNAME created"
fi

# -----------------------------------------------------------
# Deploy Authelia
# -----------------------------------------------------------
echo "  Deploying Authelia..."

# Render config template
AUTHELIA_CONFIG=$(sed \
    -e "s/__USER_ZONE__/$USER_ZONE/g" \
    -e "s/__DOMAIN__/$DOMAIN/g" \
    -e "s/__AUTH_SECRET__/$AUTH_SECRET/g" \
    -e "s/__LLDAP_ADMIN_PASSWORD__/$LLDAP_ADMIN_PASSWORD/g" \
    -e "s/__REDIS_PASSWORD__/$REDIS_PASSWORD/g" \
    -e "s/__USERNAME__/$USERNAME/g" \
    "$AUTH_DIR/authelia-config.yaml.tmpl")

# Store rendered config in ConfigMap
kubectl create configmap authelia-config \
    --namespace packalares-auth \
    --from-literal=configuration.yml="$AUTHELIA_CONFIG" \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f "$AUTH_DIR/authelia-deployment.yaml"

# Wait for Authelia to be ready
kubectl rollout status deployment/authelia -n packalares-auth --timeout=180s

echo "  Authelia running"

# -----------------------------------------------------------
# Store deployment info for other modules
# -----------------------------------------------------------
kubectl create configmap auth-info \
    --namespace packalares-auth \
    --from-literal=auth-endpoint="authelia-svc.packalares-auth:9091" \
    --from-literal=login-url="https://auth.$USER_ZONE/" \
    --from-literal=lldap-endpoint="lldap-svc.packalares-auth:3890" \
    --from-literal=lldap-http="lldap-svc.packalares-auth:17170" \
    --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "  Authentication ready"
echo "    Auth endpoint:  authelia-svc.packalares-auth:9091"
echo "    Login URL:      https://auth.$USER_ZONE/"
echo "    LLDAP admin:    http://lldap-svc.packalares-auth:17170"
echo ""
