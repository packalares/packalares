#!/bin/bash
set -euo pipefail

echo ""
echo "=========================================="
echo "  Packalares Installer"
echo "=========================================="
echo ""

# Config — all from env vars
USERNAME="${PACKALARES_USER:-}"
PASSWORD="${PACKALARES_PASSWORD:-}"
DOMAIN="${PACKALARES_DOMAIN:-olares.local}"

if [ -z "$USERNAME" ]; then read -rp "Username: " USERNAME; fi
if [ -z "$PASSWORD" ]; then read -rs -p "Password: " PASSWORD; echo; fi
[ -z "$USERNAME" ] && { echo "Username required"; exit 1; }
[ -z "$PASSWORD" ] && { echo "Password required"; exit 1; }

NODE_IP=$(hostname -I | awk '{print $1}')
USER_ZONE="${USERNAME}.${DOMAIN}"
VERSION=$(cat VERSION 2>/dev/null || echo "1.12.6-20260317")

echo "  Server:  $NODE_IP"
echo "  User:    $USERNAME"
echo "  Domain:  $USER_ZONE"
echo "  Version: $VERSION"
echo ""

# Step 1: Build CLI if not already built
echo "[1/5] Building CLI..."
RELEASE_URL="https://github.com/packalares/packalares/releases/latest/download"

# Download CLI
if [ ! -f /usr/local/bin/olares-cli ]; then
    curl -sfL "$RELEASE_URL/olares-cli-linux-amd64" -o /usr/local/bin/olares-cli
    chmod +x /usr/local/bin/olares-cli
    echo "  CLI downloaded"
else
    echo "  CLI already installed"
fi

# Download daemon
curl -sfL "$RELEASE_URL/olaresd-linux-amd64" -o /tmp/olaresd-linux-amd64
echo "  Daemon downloaded"

# Download and extract wizard tarball
curl -sfL "$RELEASE_URL/install-wizard.tar.gz" -o /tmp/install-wizard.tar.gz
mkdir -p /tmp/wizard-extract
tar -xzf /tmp/install-wizard.tar.gz -C /tmp/wizard-extract/
VERSION=$(cat /tmp/wizard-extract/version.hint 2>/dev/null || echo "$VERSION")
mkdir -p "$HOME/.olares/versions/v$VERSION"
cp -a /tmp/wizard-extract/* "$HOME/.olares/versions/v$VERSION/"
rm -rf /tmp/wizard-extract /tmp/install-wizard.tar.gz
echo "  Wizard extracted (version: $VERSION)"

# Create olaresd tarball for prepare step
mkdir -p /tmp/olaresd-pkg "$HOME/.olares/versions/v$VERSION/pkg"
cp /tmp/olaresd-linux-amd64 /tmp/olaresd-pkg/olaresd
chmod +x /tmp/olaresd-pkg/olaresd
tar -czf "$HOME/.olares/versions/v$VERSION/pkg/olaresd-v$VERSION.tar.gz" -C /tmp/olaresd-pkg olaresd
rm -rf /tmp/olaresd-pkg /tmp/olaresd-linux-amd64
echo "  All tools ready"

# Step 2: Precheck + Download
echo ""
echo "[2/5] Precheck and download..."
olares-cli precheck 2>&1 || true
olares-cli download component --version "$VERSION" --cdn-service https://cdn.olares.com 2>&1 || echo "  Some components will be pulled on demand"

# Step 3: Prepare (containerd, redis, olaresd)
echo ""
echo "[3/5] Preparing system..."
olares-cli prepare --version "$VERSION"

# Step 4: Install (K3s, Helm charts, all services)
echo ""
echo "[4/5] Installing system..."
olares-cli install --version "$VERSION" --os-domainname "$DOMAIN" --os-username "$USERNAME" --os-password "$PASSWORD"

# Step 5: Activate
echo ""
echo "[5/5] Activating..."
export KUBECONFIG=/root/.kube/config

# Set LLDAP password
LLDAP_IP=$(kubectl get svc lldap-service -n os-platform -o jsonpath='{.spec.clusterIP}' 2>/dev/null)
LLDAP_PASS=$(kubectl get secret lldap-credentials -n os-platform -o jsonpath='{.data.lldap-ldap-user-pass}' 2>/dev/null | base64 -d)
ADMIN_TOKEN=$(curl -s -X POST "http://$LLDAP_IP:17170/auth/simple/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$LLDAP_PASS\"}" 2>/dev/null | \
    python3 -c 'import sys,json; print(json.load(sys.stdin).get("token",""))' 2>/dev/null || true)
if [ -n "$ADMIN_TOKEN" ]; then
    curl -s -X POST "http://$LLDAP_IP:17170/api/graphql" \
        -H 'Content-Type: application/json' \
        -H "Authorization: Bearer $ADMIN_TOKEN" \
        -d "{\"query\":\"mutation { modifyUser(user: {id: \\\"$USERNAME\\\", password: \\\"$PASSWORD\\\"}) { ok } }\"}" >/dev/null 2>&1
    echo "  Password set"
fi

# Generate TLS cert
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

# Set annotations
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

# Patch authelia config
kubectl get configmap authelia-configs -n os-framework -o yaml > /tmp/authelia-patch.yaml 2>/dev/null
sed -i "s/example\.myterminus\.com/$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/files\.example\.myterminus\.com/files.$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/'example\.com'/$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/authelia-svc\.example\.com/auth.$USER_ZONE/g" /tmp/authelia-patch.yaml
sed -i "s/www\.example\.com/desktop.$USER_ZONE/g" /tmp/authelia-patch.yaml
kubectl apply -f /tmp/authelia-patch.yaml 2>/dev/null
rm -f /tmp/authelia-patch.yaml
echo "  Auth configured"

# Restart services + trigger L4 proxy
kubectl delete pod -l app=authelia-backend -n os-framework --force 2>/dev/null
kubectl delete pod bfl-0 -n "user-space-$USERNAME" 2>/dev/null
sleep 15
kubectl annotate user "$USERNAME" "bytetrade.io/wizard-status=network_activating" --overwrite 2>/dev/null
for i in $(seq 1 60); do
    kubectl get pods -n os-network -l app=l4-bfl-proxy --no-headers 2>/dev/null | grep -q Running && break
    sleep 5
done
kubectl annotate user "$USERNAME" "bytetrade.io/wizard-status=completed" --overwrite 2>/dev/null
kubectl delete pod bfl-0 -n "user-space-$USERNAME" 2>/dev/null
sleep 15
echo "  Activation complete"

echo ""
echo "=========================================="
echo "  Packalares is ready!"
echo "=========================================="
echo ""
echo "  Hosts file:"
echo "  $NODE_IP  desktop.$USER_ZONE  auth.$USER_ZONE  settings.$USER_ZONE  market.$USER_ZONE  files.$USER_ZONE  $USER_ZONE"
echo ""
echo "  Open: https://desktop.$USER_ZONE"
echo "  Login: $USERNAME"
echo ""
