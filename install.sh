#!/bin/bash
set -e

echo "================================"
echo "  Packalares Installer"
echo "================================"
echo ""

# ============================================================
# Read config
# ============================================================
USERNAME="${PACKALARES_USER:-}"
PASSWORD="${PACKALARES_PASSWORD:-}"
DOMAIN="${PACKALARES_DOMAIN:-olares.local}"
NODE_IP=$(hostname -I | awk '{print $1}')
REPO="packalares/packalares"
OLARES_REPO="packalares/olares-fork"

if [ -z "$USERNAME" ]; then
    read -p "Username: " USERNAME
fi
if [ -z "$PASSWORD" ]; then
    read -s -p "Password: " PASSWORD
    echo ""
fi

USER_ZONE="${USERNAME}.${DOMAIN}"

echo ""
echo "  Server:  $NODE_IP"
echo "  User:    $USERNAME"
echo "  Domain:  $USER_ZONE"
echo ""

# ============================================================
# Phase 1: Download tools
# ============================================================
echo "[1/7] Downloading tools..."

RELEASE_URL="https://github.com/$OLARES_REPO/releases/latest/download"

# CLI
curl -sfL "$RELEASE_URL/olares-cli-linux-amd64" -o /usr/local/bin/olares-cli && chmod +x /usr/local/bin/olares-cli

# Olaresd
curl -sfL "$RELEASE_URL/olaresd-linux-amd64" -o /tmp/olaresd-linux-amd64

# Wizard tarball (contains Helm charts + manifests)
curl -sfL "$RELEASE_URL/install-wizard.tar.gz" -o /tmp/install-wizard.tar.gz

# Extract wizard and detect version from VERSION file inside
mkdir -p /tmp/wizard-extract
tar -xzf /tmp/install-wizard.tar.gz -C /tmp/wizard-extract/
VERSION=$(cat /tmp/wizard-extract/wizard/VERSION 2>/dev/null || echo "1.12.6-20260317")

# Move to correct location
mkdir -p "$HOME/.olares/versions/v$VERSION"
mv /tmp/wizard-extract/* "$HOME/.olares/versions/v$VERSION/" 2>/dev/null || true
rm -rf /tmp/wizard-extract /tmp/install-wizard.tar.gz

# Create olaresd tarball
mkdir -p /tmp/olaresd-pkg && cp /tmp/olaresd-linux-amd64 /tmp/olaresd-pkg/olaresd
chmod +x /tmp/olaresd-pkg/olaresd
mkdir -p "$HOME/.olares/versions/v$VERSION/pkg"
tar -czf "$HOME/.olares/versions/v$VERSION/pkg/olaresd-v$VERSION.tar.gz" -C /tmp/olaresd-pkg olaresd
rm -rf /tmp/olaresd-pkg /tmp/olaresd-linux-amd64

echo "  Tools downloaded (version: $VERSION)"

# ============================================================
# Phase 2: Remove old Docker/containerd conflicts
# ============================================================
echo "[2/7] Cleaning environment..."

systemctl stop docker 2>/dev/null || true
apt-get remove -y docker.io docker-ce containerd.io 2>/dev/null || true
nft flush ruleset 2>/dev/null || true

# ============================================================
# Phase 3: System precheck + download binaries
# ============================================================
echo "[3/7] Downloading system binaries..."

olares-cli precheck 2>/dev/null || true
olares-cli download component --version "$VERSION" 2>/dev/null || true

# ============================================================
# Phase 4: Prepare (containerd, redis, olaresd)
# ============================================================
echo "[4/7] Preparing system..."

olares-cli prepare --version "$VERSION" 2>/dev/null || true

# ============================================================
# Phase 5: Install K3s + all services
# ============================================================
echo "[5/7] Installing system (this takes a few minutes)..."

INSTALL_FLAGS="--version $VERSION --os-domainname $DOMAIN"
[ -n "$USERNAME" ] && INSTALL_FLAGS="$INSTALL_FLAGS --os-username $USERNAME"
[ -n "$PASSWORD" ] && INSTALL_FLAGS="$INSTALL_FLAGS --os-password $PASSWORD"
olares-cli install $INSTALL_FLAGS

# ============================================================
# Phase 6: Activate
# ============================================================
echo "[6/7] Activating..."

export KUBECONFIG=/root/.kube/config

# 6a. Set LLDAP password via HTTP API
LLDAP_IP=$(kubectl get svc lldap-service -n os-platform -o jsonpath='{.spec.clusterIP}' 2>/dev/null)
LLDAP_PASS=$(kubectl get secret lldap-credentials -n os-platform -o jsonpath='{.data.lldap-ldap-user-pass}' 2>/dev/null | base64 -d)

ADMIN_TOKEN=$(curl -s -X POST "http://$LLDAP_IP:17170/auth/simple/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$LLDAP_PASS\"}" 2>/dev/null | \
    python3 -c 'import sys,json; print(json.load(sys.stdin).get("token",""))' 2>/dev/null)

if [ -n "$ADMIN_TOKEN" ]; then
    curl -s -X POST "http://$LLDAP_IP:17170/api/graphql" \
        -H 'Content-Type: application/json' \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -d "{\"query\":\"mutation { modifyUser(user: {id: \\\"$USERNAME\\\", password: \\\"$PASSWORD\\\"}) { ok } }\"}" >/dev/null 2>&1
    echo "  Password set"
fi

# 6b. Generate self-signed TLS cert → zone-ssl-config configmap
openssl req -x509 -nodes -days 3650 \
    -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
    -keyout /tmp/tls.key -out /tmp/tls.crt \
    -subj "/CN=*.$USER_ZONE" \
    -addext "subjectAltName=DNS:*.$USER_ZONE,DNS:$USER_ZONE" 2>/dev/null

CERT_DATA=$(cat /tmp/tls.crt)
KEY_DATA=$(cat /tmp/tls.key)
rm -f /tmp/tls.key /tmp/tls.crt

cat <<EOCM | kubectl apply -f - 2>/dev/null
apiVersion: v1
kind: ConfigMap
metadata:
  name: zone-ssl-config
  namespace: user-space-$USERNAME
data:
  zone: $USER_ZONE
  cert: |
$(echo "$CERT_DATA" | sed 's/^/    /')
  key: |
$(echo "$KEY_DATA" | sed 's/^/    /')
EOCM
echo "  TLS certificate generated"

# 6c. Set user annotations
kubectl annotate user "$USERNAME" \
    "bytetrade.io/zone=$USER_ZONE" \
    "bytetrade.io/creator=$USERNAME" \
    "bytetrade.io/owner-role=owner" \
    "bytetrade.io/language=en-US" \
    "bytetrade.io/location=Europe/Amsterdam" \
    "bytetrade.io/theme=light" \
    "bytetrade.io/launcher-access-level=1" \
    "bytetrade.io/launcher-auth-policy=one_factor" \
    "bytetrade.io/is-ephemeral=false" \
    "bytetrade.io/local-domain-ip=$NODE_IP" \
    "bytetrade.io/local-domain-dns-record=$NODE_IP" \
    --overwrite 2>/dev/null
echo "  User configured"

# 6d. Patch authelia config
kubectl get configmap authelia-configs -n os-framework -o yaml > /tmp/authelia-patch.yaml 2>/dev/null
sed -i "s/example\.myterminus\.com/$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/files\.example\.myterminus\.com/files.$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/'example\.com'/$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/authelia-svc\.example\.com/auth.$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/www\.example\.com/desktop.$USER_ZONE/g" /tmp/authelia-patch.yaml
kubectl apply -f /tmp/authelia-patch.yaml 2>/dev/null
rm -f /tmp/authelia-patch.yaml
echo "  Auth configured"

# 6e. Restart services to pick up changes
kubectl delete pod -l app=authelia-backend -n os-framework --force 2>/dev/null
kubectl delete pod bfl-0 -n "user-space-$USERNAME" 2>/dev/null
sleep 15

# 6f. Trigger L4 proxy generation
kubectl annotate user "$USERNAME" "bytetrade.io/wizard-status=network_activating" --overwrite 2>/dev/null
for i in $(seq 1 60); do
    if kubectl get pods -n os-network -l app=l4-bfl-proxy --no-headers 2>/dev/null | grep -q Running; then
        break
    fi
    sleep 5
done
kubectl annotate user "$USERNAME" "bytetrade.io/wizard-status=completed" --overwrite 2>/dev/null

# 6g. Final BFL restart (cert + L4 proxy + apps all ready)
kubectl delete pod bfl-0 -n "user-space-$USERNAME" 2>/dev/null
sleep 15

echo "  Activation complete"

# ============================================================
# Phase 7: Deploy Caddy (future — currently using BFL + L4)
# ============================================================
echo "[7/7] Finalizing..."

# TODO: Deploy Caddy as replacement for BFL + L4 proxy
# For now, the system uses stock BFL + L4 proxy from Olares
# Caddy deployment will be added in Phase 2

echo ""
echo "================================"
echo "  Packalares is ready!"
echo "================================"
echo ""
echo "  Add to your hosts file:"
echo "  $NODE_IP  desktop.$USER_ZONE  auth.$USER_ZONE  settings.$USER_ZONE  market.$USER_ZONE  files.$USER_ZONE  $USER_ZONE"
echo ""
echo "  Then open: https://desktop.$USER_ZONE"
echo "  Login: $USERNAME / (your password)"
echo ""
echo "  Wizard: http://$NODE_IP:30180"
echo ""
