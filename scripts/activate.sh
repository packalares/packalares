#!/bin/bash
# ==========================================================================
# Packalares — User Activation
# ==========================================================================
# Creates the user account and configures access:
#   1. Creates/updates user in LLDAP via GraphQL API
#   2. Generates self-signed TLS certificate for the user zone
#   3. Configures Caddy with the certificate
#   4. Sets up mDNS (avahi) for local network discovery
#
# Expected environment variables (set by install.sh):
#   PACKALARES_USER       — username
#   PACKALARES_PASSWORD   — password
#   PACKALARES_DOMAIN     — base domain (e.g. olares.local)
#   USER_ZONE             — full zone (e.g. alice.olares.local)
#   NODE_IP               — server IP address
#   KUBECONFIG            — path to kubeconfig
# ==========================================================================

set -euo pipefail

# --------------------------------------------------------------------------
# Colors / helpers
# --------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[activate]${NC} $*"; }
ok()    { echo -e "${GREEN}[activate]${NC} $*"; }
warn()  { echo -e "${YELLOW}[activate]${NC} $*"; }
err()   { echo -e "${RED}[activate]${NC} $*"; }
die()   { err "$@"; exit 1; }

# --------------------------------------------------------------------------
# Validate required env vars
# --------------------------------------------------------------------------
: "${PACKALARES_USER:?PACKALARES_USER is not set}"
: "${PACKALARES_PASSWORD:?PACKALARES_PASSWORD is not set}"
: "${PACKALARES_DOMAIN:?PACKALARES_DOMAIN is not set}"
: "${USER_ZONE:=${PACKALARES_USER}.${PACKALARES_DOMAIN}}"
: "${NODE_IP:=$(hostname -I | awk '{print $1}')}"
: "${KUBECONFIG:=/etc/rancher/k3s/k3s.yaml}"

export KUBECONFIG

USERNAME="$PACKALARES_USER"
PASSWORD="$PACKALARES_PASSWORD"

# ==========================================================================
# 1. Create / update user in LLDAP
# ==========================================================================
info "Configuring LLDAP user '$USERNAME' ..."

# Discover LLDAP service IP and admin password from K8s
LLDAP_IP=""
LLDAP_ADMIN_PASS=""

if kubectl get svc lldap-service -n packalares-auth &>/dev/null; then
    LLDAP_IP="$(kubectl get svc lldap-service -n packalares-auth -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
    LLDAP_ADMIN_PASS="$(kubectl get secret lldap-credentials -n packalares-auth -o jsonpath='{.data.lldap-ldap-user-pass}' 2>/dev/null | base64 -d || true)"
elif kubectl get svc lldap-service -n os-platform &>/dev/null; then
    # Legacy namespace
    LLDAP_IP="$(kubectl get svc lldap-service -n os-platform -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)"
    LLDAP_ADMIN_PASS="$(kubectl get secret lldap-credentials -n os-platform -o jsonpath='{.data.lldap-ldap-user-pass}' 2>/dev/null | base64 -d || true)"
fi

if [ -n "$LLDAP_IP" ] && [ -n "$LLDAP_ADMIN_PASS" ]; then
    LLDAP_URL="http://${LLDAP_IP}:17170"

    # Authenticate as admin
    ADMIN_TOKEN=""
    for attempt in 1 2 3; do
        ADMIN_TOKEN="$(curl -s --max-time 10 -X POST "${LLDAP_URL}/auth/simple/login" \
            -H 'Content-Type: application/json' \
            -d "{\"username\":\"admin\",\"password\":\"${LLDAP_ADMIN_PASS}\"}" 2>/dev/null | \
            python3 -c 'import sys,json; print(json.load(sys.stdin).get("token",""))' 2>/dev/null || true)"
        [ -n "$ADMIN_TOKEN" ] && break
        sleep 3
    done

    if [ -n "$ADMIN_TOKEN" ]; then
        GQL_URL="${LLDAP_URL}/api/graphql"
        AUTH_HEADER="Authorization: Bearer ${ADMIN_TOKEN}"

        # Try to create the user first
        CREATE_QUERY='mutation { createUser(user: {id: "'"${USERNAME}"'", displayName: "'"${USERNAME}"'", email: "'"${USERNAME}@${PACKALARES_DOMAIN}"'"}) { id } }'
        CREATE_RESP="$(curl -s --max-time 10 -X POST "$GQL_URL" \
            -H 'Content-Type: application/json' \
            -H "$AUTH_HEADER" \
            -d "{\"query\": $(echo "$CREATE_QUERY" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read().strip()))')}" 2>/dev/null || true)"

        if echo "$CREATE_RESP" | grep -q '"id"'; then
            info "User '$USERNAME' created in LLDAP"
        else
            info "User '$USERNAME' may already exist in LLDAP — updating password"
        fi

        # Set/update the user password
        PASS_QUERY='mutation { modifyUser(user: {id: "'"${USERNAME}"'", password: "'"${PASSWORD}"'"}) { ok } }'
        curl -s --max-time 10 -X POST "$GQL_URL" \
            -H 'Content-Type: application/json' \
            -H "$AUTH_HEADER" \
            -d "{\"query\": $(echo "$PASS_QUERY" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read().strip()))')}" >/dev/null 2>&1

        ok "LLDAP user password set"
    else
        warn "Could not authenticate with LLDAP — user may need manual setup"
    fi
else
    warn "LLDAP service not found — skipping user creation"
fi

# ==========================================================================
# 2. Generate self-signed TLS certificate
# ==========================================================================
info "Generating TLS certificate for *.$USER_ZONE ..."

CERT_DIR="/etc/packalares/tls"
mkdir -p "$CERT_DIR"

CERT_FILE="${CERT_DIR}/${USER_ZONE}.crt"
KEY_FILE="${CERT_DIR}/${USER_ZONE}.key"

# Only regenerate if cert does not exist or is for a different domain
NEEDS_CERT="true"
if [ -f "$CERT_FILE" ] && [ -f "$KEY_FILE" ]; then
    EXISTING_CN="$(openssl x509 -in "$CERT_FILE" -noout -subject 2>/dev/null | sed 's/.*CN\s*=\s*//' || true)"
    if [ "$EXISTING_CN" = "*.$USER_ZONE" ]; then
        NEEDS_CERT="false"
        ok "TLS certificate already exists for *.$USER_ZONE"
    fi
fi

if [ "$NEEDS_CERT" = "true" ]; then
    openssl req -x509 -nodes -days 3650 \
        -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
        -keyout "$KEY_FILE" -out "$CERT_FILE" \
        -subj "/CN=*.$USER_ZONE" \
        -addext "subjectAltName=DNS:*.$USER_ZONE,DNS:$USER_ZONE,DNS:*.${PACKALARES_DOMAIN},IP:$NODE_IP" \
        2>/dev/null

    chmod 600 "$KEY_FILE"
    chmod 644 "$CERT_FILE"
    ok "TLS certificate generated at $CERT_FILE"
fi

# Store cert in a K8s secret so Caddy and other services can use it
TLS_NAMESPACE="packalares-system"
kubectl create namespace "$TLS_NAMESPACE" 2>/dev/null || true

if kubectl get secret tls-zone-cert -n "$TLS_NAMESPACE" &>/dev/null; then
    kubectl delete secret tls-zone-cert -n "$TLS_NAMESPACE" 2>/dev/null || true
fi

kubectl create secret tls tls-zone-cert \
    --cert="$CERT_FILE" \
    --key="$KEY_FILE" \
    -n "$TLS_NAMESPACE" 2>/dev/null

ok "TLS secret stored in K8s (tls-zone-cert in $TLS_NAMESPACE)"

# Also store as a ConfigMap for legacy compatibility
cat <<EOCM | kubectl apply -f - 2>/dev/null
apiVersion: v1
kind: ConfigMap
metadata:
  name: zone-ssl-config
  namespace: ${TLS_NAMESPACE}
data:
  zone: ${USER_ZONE}
  cert: |
$(sed 's/^/    /' "$CERT_FILE")
  key: |
$(sed 's/^/    /' "$KEY_FILE")
EOCM

ok "TLS ConfigMap zone-ssl-config applied"

# ==========================================================================
# 3. Configure Caddy with the TLS cert
# ==========================================================================
info "Configuring Caddy reverse proxy ..."

# Determine Caddy namespace
CADDY_NS="packalares-system"

# Process Caddyfile template with environment variables
CADDYFILE_TMPL=""
SCRIPT_DIR=""
if [ -f "${BASH_SOURCE[0]:-}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi

if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/proxy/Caddyfile.tmpl" ]; then
    CADDYFILE_TMPL="$SCRIPT_DIR/proxy/Caddyfile.tmpl"
elif [ -f "/tmp/packalares-Caddyfile.tmpl" ]; then
    CADDYFILE_TMPL="/tmp/packalares-Caddyfile.tmpl"
fi

if [ -n "$CADDYFILE_TMPL" ]; then
    # Determine auth service address
    AUTH_SERVICE=""
    if kubectl get svc authelia-svc -n packalares-auth &>/dev/null; then
        AUTH_SERVICE="authelia-svc.packalares-auth.svc.cluster.local:9091"
    elif kubectl get svc authelia-backend -n os-framework &>/dev/null; then
        AUTH_SERVICE="authelia-backend.os-framework.svc.cluster.local:9091"
    else
        AUTH_SERVICE="authelia-svc.packalares-auth.svc.cluster.local:9091"
    fi
    export AUTH_SERVICE

    RENDERED_CADDYFILE="$(envsubst '${USER_ZONE} ${USERNAME} ${NODE_IP} ${AUTH_SERVICE}' < "$CADDYFILE_TMPL")"

    # Apply as ConfigMap
    kubectl create configmap caddy-config \
        --from-literal=Caddyfile="$RENDERED_CADDYFILE" \
        -n "$CADDY_NS" \
        --dry-run=client -o yaml | kubectl apply -f - 2>/dev/null

    ok "Caddyfile rendered and applied"

    # Mount the TLS cert files into Caddy so it can serve them
    # Patch the Caddy DaemonSet to mount the TLS secret
    kubectl patch daemonset caddy -n "$CADDY_NS" --type='json' -p='[
        {"op": "add", "path": "/spec/template/spec/volumes/-", "value": {"name": "tls-cert", "secret": {"secretName": "tls-zone-cert"}}},
        {"op": "add", "path": "/spec/template/spec/containers/0/volumeMounts/-", "value": {"name": "tls-cert", "mountPath": "/etc/caddy/tls", "readOnly": true}}
    ]' 2>/dev/null || true

    # Restart Caddy to pick up new config
    kubectl rollout restart daemonset/caddy -n "$CADDY_NS" 2>/dev/null || true
    ok "Caddy restarted with new config"
else
    warn "Caddyfile template not found — Caddy config not updated"
fi

# ==========================================================================
# 4. Set up mDNS for local network discovery
# ==========================================================================
info "Setting up mDNS (avahi) ..."

# Install avahi if not present
if ! command -v avahi-daemon &>/dev/null; then
    if command -v apt-get &>/dev/null; then
        apt-get update -qq && apt-get install -y -qq avahi-daemon avahi-utils >/dev/null 2>&1 || true
    elif command -v dnf &>/dev/null; then
        dnf install -y -q avahi avahi-tools >/dev/null 2>&1 || true
    fi
fi

if command -v avahi-daemon &>/dev/null; then
    # Enable and start avahi
    systemctl enable avahi-daemon 2>/dev/null || true
    systemctl start avahi-daemon 2>/dev/null || true

    # Create CNAME aliases for all subdomains via avahi services
    AVAHI_SERVICE_DIR="/etc/avahi/services"
    mkdir -p "$AVAHI_SERVICE_DIR"

    cat > "${AVAHI_SERVICE_DIR}/packalares.service" <<EOAVAHI
<?xml version="1.0" standalone='no'?>
<!DOCTYPE service-group SYSTEM "avahi-service.dtd">
<service-group>
  <name replace-wildcards="yes">Packalares on %h</name>
  <service>
    <type>_http._tcp</type>
    <port>80</port>
    <txt-record>path=/</txt-record>
    <txt-record>zone=${USER_ZONE}</txt-record>
  </service>
  <service>
    <type>_https._tcp</type>
    <port>443</port>
    <txt-record>path=/</txt-record>
    <txt-record>zone=${USER_ZONE}</txt-record>
  </service>
</service-group>
EOAVAHI

    # If the domain ends in .local, avahi can resolve it natively.
    # Add hostname aliases so desktop.USER_ZONE etc. resolve.
    if [[ "$PACKALARES_DOMAIN" == *.local ]]; then
        # Configure avahi to publish CNAME records
        AVAHI_CNAME_CONF="/etc/avahi/packalares-aliases.conf"
        cat > "$AVAHI_CNAME_CONF" <<EOCNAME
# Packalares mDNS aliases
# These are published as CNAME records on the local network
${USER_ZONE}
desktop.${USER_ZONE}
auth.${USER_ZONE}
settings.${USER_ZONE}
market.${USER_ZONE}
files.${USER_ZONE}
EOCNAME

        # Install avahi-alias publishing script if possible
        # This runs a small daemon that publishes CNAME records
        ALIAS_SCRIPT="/usr/local/bin/packalares-mdns-publish"
        cat > "$ALIAS_SCRIPT" <<'EOPUBLISH'
#!/bin/bash
# Publish mDNS CNAME records for Packalares subdomains
# Runs as a systemd service

CONF="/etc/avahi/packalares-aliases.conf"
[ -f "$CONF" ] || exit 0

PIDS=()

cleanup() {
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    exit 0
}
trap cleanup SIGTERM SIGINT

while IFS= read -r alias; do
    # Skip comments and blank lines
    [[ "$alias" =~ ^#.*$ ]] && continue
    [[ -z "$alias" ]] && continue

    # avahi-publish-address publishes an A record for the alias
    # pointing to this machine's IP
    NODE_IP="$(hostname -I | awk '{print $1}')"
    avahi-publish-address -R "$alias" "$NODE_IP" &
    PIDS+=($!)
done < "$CONF"

# Wait for all publishers
wait
EOPUBLISH
        chmod +x "$ALIAS_SCRIPT"

        # Create systemd service for mDNS publishing
        cat > /etc/systemd/system/packalares-mdns.service <<EOSVC
[Unit]
Description=Packalares mDNS alias publisher
After=avahi-daemon.service
Requires=avahi-daemon.service

[Service]
Type=simple
ExecStart=/usr/local/bin/packalares-mdns-publish
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOSVC

        systemctl daemon-reload 2>/dev/null || true
        systemctl enable packalares-mdns 2>/dev/null || true
        systemctl restart packalares-mdns 2>/dev/null || true
        ok "mDNS aliases published for $USER_ZONE subdomains"
    else
        ok "Domain is not .local — mDNS service registered but CNAME aliases skipped"
    fi

    ok "avahi-daemon running"
else
    warn "avahi not available — mDNS not configured (local DNS or hosts file required)"
fi

# ==========================================================================
# Done
# ==========================================================================
ok "Activation complete for user '$USERNAME' at $USER_ZONE"
