#!/bin/bash
###############################################################
# setup-platform.sh — Deploy platform services (PostgreSQL, NATS, MinIO)
#
# These shared services are used by marketplace apps that need
# databases, messaging, or object storage.
#
# Deploys into namespace: packalares-platform
#
# Optional env vars (with defaults):
#   POSTGRES_VERSION   — PostgreSQL image tag  (default: 16-alpine)
#   NATS_VERSION       — NATS image tag        (default: 2.10-alpine)
#   MINIO_VERSION      — MinIO image tag       (default: RELEASE.2024-06-13T22-53-53Z)
#   POSTGRES_PASSWORD  — Admin password         (auto-generated if unset)
#   MINIO_ROOT_USER    — MinIO root user        (default: packalares)
#   MINIO_ROOT_PASSWORD — MinIO root password   (auto-generated if unset)
#   KUBECONFIG         — Path to kubeconfig     (default: /etc/rancher/k3s/k3s.yaml)
#   DATA_PATH          — Base data directory    (default: /packalares/data)
###############################################################
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DATABASE_DIR="$PROJECT_DIR/database"
MESSAGING_DIR="$PROJECT_DIR/messaging"
OBJECTSTORE_DIR="$PROJECT_DIR/objectstore"

# ---------------------------------------------------------------
# Configuration — from environment with sane defaults
# ---------------------------------------------------------------
POSTGRES_VERSION="${POSTGRES_VERSION:-16-alpine}"
NATS_VERSION="${NATS_VERSION:-2.10-alpine}"
MINIO_VERSION="${MINIO_VERSION:-RELEASE.2024-06-13T22-53-53Z}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-packalares}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
DATA_PATH="${DATA_PATH:-/packalares/data}"

export KUBECONFIG

NAMESPACE="packalares-platform"

# ---------------------------------------------------------------
# Logging
# ---------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[platform]${NC} $*"; }
ok()    { echo -e "${GREEN}[platform]${NC} $*"; }
warn()  { echo -e "${YELLOW}[platform]${NC} $*"; }
err()   { echo -e "${RED}[platform]${NC} $*"; }
die()   { err "$@"; exit 1; }

# ---------------------------------------------------------------
# Secret generation
# ---------------------------------------------------------------
generate_secret() {
    openssl rand -hex 32
}

# ---------------------------------------------------------------
# Namespace
# ---------------------------------------------------------------
info "Creating namespace $NAMESPACE..."
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# ===============================================================
# 1. PostgreSQL
# ===============================================================
deploy_postgres() {
    if kubectl get deployment postgres -n "$NAMESPACE" &>/dev/null; then
        ok "PostgreSQL already deployed -- skipping"
        return 0
    fi

    info "Deploying PostgreSQL (postgres:$POSTGRES_VERSION)..."

    # Generate password if not provided, or reuse existing secret
    local pg_password="${POSTGRES_PASSWORD:-}"
    if [ -z "$pg_password" ]; then
        # Check if secret already exists in the cluster
        pg_password="$(kubectl get secret postgres-credentials -n "$NAMESPACE" \
            -o jsonpath='{.data.admin-password}' 2>/dev/null | base64 -d 2>/dev/null || true)"
        if [ -z "$pg_password" ]; then
            pg_password="$(generate_secret)"
            info "  Generated PostgreSQL admin password"
        else
            info "  Reusing existing PostgreSQL password from secret"
        fi
    fi

    # Create/update secret
    kubectl create secret generic postgres-credentials \
        --namespace "$NAMESPACE" \
        --from-literal=admin-password="$pg_password" \
        --from-literal=connection-string="postgresql://packalares:${pg_password}@postgres-svc.${NAMESPACE}:5432/packalares" \
        --dry-run=client -o yaml | kubectl apply -f -

    # Apply deployment (patch image version if needed)
    sed "s|image: postgres:16-alpine|image: postgres:${POSTGRES_VERSION}|" \
        "$DATABASE_DIR/postgres-deployment.yaml" | kubectl apply -f -

    # Wait for rollout
    info "  Waiting for PostgreSQL to be ready..."
    kubectl rollout status deployment/postgres -n "$NAMESPACE" --timeout=180s

    ok "PostgreSQL deployed at postgres-svc.$NAMESPACE:5432"
}

# ===============================================================
# 2. NATS
# ===============================================================
deploy_nats() {
    if kubectl get deployment nats -n "$NAMESPACE" &>/dev/null; then
        ok "NATS already deployed -- skipping"
        return 0
    fi

    info "Deploying NATS (nats:$NATS_VERSION)..."

    # Apply deployment (patch image version if needed)
    sed "s|image: nats:2.10-alpine|image: nats:${NATS_VERSION}|" \
        "$MESSAGING_DIR/nats-deployment.yaml" | kubectl apply -f -

    # Wait for rollout
    info "  Waiting for NATS to be ready..."
    kubectl rollout status deployment/nats -n "$NAMESPACE" --timeout=120s

    ok "NATS deployed at nats-svc.$NAMESPACE:4222"
}

# ===============================================================
# 3. MinIO (S3-compatible object storage)
# ===============================================================
deploy_minio() {
    if kubectl get deployment minio -n "$NAMESPACE" &>/dev/null; then
        ok "MinIO already deployed -- skipping"
        return 0
    fi

    info "Deploying MinIO (minio/minio:$MINIO_VERSION)..."

    # Generate root password if not provided, or reuse existing secret
    local minio_password="${MINIO_ROOT_PASSWORD:-}"
    if [ -z "$minio_password" ]; then
        minio_password="$(kubectl get secret minio-credentials -n "$NAMESPACE" \
            -o jsonpath='{.data.root-password}' 2>/dev/null | base64 -d 2>/dev/null || true)"
        if [ -z "$minio_password" ]; then
            minio_password="$(generate_secret)"
            info "  Generated MinIO root password"
        else
            info "  Reusing existing MinIO password from secret"
        fi
    fi

    # Create/update secret
    kubectl create secret generic minio-credentials \
        --namespace "$NAMESPACE" \
        --from-literal=root-user="$MINIO_ROOT_USER" \
        --from-literal=root-password="$minio_password" \
        --from-literal=endpoint="http://s3-svc.${NAMESPACE}:9000" \
        --dry-run=client -o yaml | kubectl apply -f -

    # Apply deployment (patch image version if needed)
    sed "s|image: minio/minio:RELEASE.2024-06-13T22-53-53Z|image: minio/minio:${MINIO_VERSION}|" \
        "$OBJECTSTORE_DIR/minio-deployment.yaml" | kubectl apply -f -

    # Wait for rollout
    info "  Waiting for MinIO to be ready..."
    kubectl rollout status deployment/minio -n "$NAMESPACE" --timeout=180s

    ok "MinIO deployed at s3-svc.$NAMESPACE:9000"
}

# ===============================================================
# 4. Store platform-info ConfigMap for other modules
# ===============================================================
store_platform_info() {
    kubectl create configmap platform-info \
        --namespace "$NAMESPACE" \
        --from-literal=postgres-endpoint="postgres-svc.$NAMESPACE:5432" \
        --from-literal=postgres-user="packalares" \
        --from-literal=postgres-database="packalares" \
        --from-literal=nats-endpoint="nats-svc.$NAMESPACE:4222" \
        --from-literal=nats-monitoring="nats-svc.$NAMESPACE:8222" \
        --from-literal=s3-endpoint="http://s3-svc.$NAMESPACE:9000" \
        --from-literal=s3-console="http://s3-svc.$NAMESPACE:9001" \
        --dry-run=client -o yaml | kubectl apply -f -

    ok "Platform info ConfigMap updated"
}

# ===============================================================
# 5. Verification
# ===============================================================
verify() {
    info "Verifying platform services..."

    local errors=0

    # Check PostgreSQL
    local pg_ready
    pg_ready=$(kubectl get pods -n "$NAMESPACE" -l app=postgres --no-headers 2>/dev/null \
        | grep -c "Running" || true)
    if [ "$pg_ready" -eq 0 ]; then
        warn "PostgreSQL pod not running"
        errors=$(( errors + 1 ))
    fi

    # Check NATS
    local nats_ready
    nats_ready=$(kubectl get pods -n "$NAMESPACE" -l app=nats --no-headers 2>/dev/null \
        | grep -c "Running" || true)
    if [ "$nats_ready" -eq 0 ]; then
        warn "NATS pod not running"
        errors=$(( errors + 1 ))
    fi

    # Check MinIO
    local minio_ready
    minio_ready=$(kubectl get pods -n "$NAMESPACE" -l app=minio --no-headers 2>/dev/null \
        | grep -c "Running" || true)
    if [ "$minio_ready" -eq 0 ]; then
        warn "MinIO pod not running"
        errors=$(( errors + 1 ))
    fi

    if [ "$errors" -gt 0 ]; then
        warn "$errors service(s) not yet ready -- they may still be starting"
    else
        ok "All platform services verified"
    fi

    return "$errors"
}

# ===============================================================
# Main
# ===============================================================
main() {
    echo ""
    echo "========================================"
    echo "  Packalares -- Platform Services"
    echo "========================================"
    echo ""
    info "Versions:"
    info "  PostgreSQL: postgres:$POSTGRES_VERSION"
    info "  NATS:       nats:$NATS_VERSION"
    info "  MinIO:      minio/minio:$MINIO_VERSION"
    echo ""

    deploy_postgres
    deploy_nats
    deploy_minio
    store_platform_info

    local verify_errors=0
    verify || verify_errors=$?

    echo ""
    echo "========================================"
    echo "  Platform Services Summary"
    echo "========================================"
    echo ""
    echo "  Namespace:   $NAMESPACE"
    echo ""
    echo "  PostgreSQL:  postgres-svc.$NAMESPACE:5432"
    echo "    User:      packalares"
    echo "    Database:  packalares"
    echo "    Secret:    postgres-credentials (key: admin-password)"
    echo ""
    echo "  NATS:        nats-svc.$NAMESPACE:4222"
    echo "    Monitor:   nats-svc.$NAMESPACE:8222"
    echo ""
    echo "  MinIO (S3):  s3-svc.$NAMESPACE:9000"
    echo "    Console:   s3-svc.$NAMESPACE:9001"
    echo "    Secret:    minio-credentials (keys: root-user, root-password)"
    echo ""
    echo "  Apps can discover endpoints via the platform-info ConfigMap:"
    echo "    kubectl get configmap platform-info -n $NAMESPACE -o yaml"
    echo ""
    echo "========================================"

    if [ "$verify_errors" -gt 0 ]; then
        warn "Finished with $verify_errors verification warning(s)"
    else
        ok "Platform services setup complete"
    fi
}

main "$@"
