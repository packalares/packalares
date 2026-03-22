#!/bin/bash
# Packalares installer wrapper script
# Downloads the pre-built CLI from GitHub Release and runs the full installation.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/packalares/packalares/main/install.sh | bash
#
#   Or with options:
#   curl -fsSL ... | bash -s -- --username admin --domain mydomain.com
#
# Environment variables:
#   PACKALARES_VERSION     - Version to install (default: latest)
#   PACKALARES_REGISTRY    - Container image registry override
#   PACKALARES_BASE_DIR    - Base directory (default: /opt/packalares)
#   PACKALARES_USERNAME    - Admin username (default: admin)
#   PACKALARES_PASSWORD    - Admin password (auto-generated if empty)
#   PACKALARES_DOMAIN      - Domain name (default: olares.local)
#   OLARES_CERT_MODE       - Certificate mode: local or acme
#   OLARES_ACME_EMAIL      - Email for Let's Encrypt
#   OLARES_ACME_DNS_PROVIDER - DNS provider for ACME
#   OLARES_TAILSCALE_AUTH_KEY - Tailscale auth key
#   OLARES_TAILSCALE_CONTROL_URL - Tailscale/Headscale control URL

set -euo pipefail

# --- Configuration ---
PACKALARES_VERSION="${PACKALARES_VERSION:-1.0.0}"
PACKALARES_BASE_DIR="${PACKALARES_BASE_DIR:-/opt/packalares}"
PACKALARES_USERNAME="${PACKALARES_USERNAME:-admin}"
PACKALARES_DOMAIN="${PACKALARES_DOMAIN:-olares.local}"
RELEASE_BASE_URL="${RELEASE_BASE_URL:-https://github.com/packalares/packalares/releases/download}"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# --- Cleanup previous installations ---
cleanup_previous() {
    log_info "Checking for previous installations..."

    # Stop services
    systemctl stop k3s etcd olaresd containerd docker 2>/dev/null || true
    systemctl disable k3s etcd olaresd containerd docker 2>/dev/null || true

    # K3s uninstall
    if [ -f /usr/local/bin/k3s-killall.sh ]; then
        /usr/local/bin/k3s-killall.sh 2>/dev/null || true
    fi
    if [ -f /usr/local/bin/k3s-uninstall.sh ]; then
        /usr/local/bin/k3s-uninstall.sh 2>/dev/null || true
    fi

    # Kill remaining processes
    pkill -9 -f 'k3s|etcd|containerd|kubelet|kube|calico|bird|runc|shim|node_exporter' 2>/dev/null || true
    sleep 1

    # Unmount containerd overlays
    for m in $(mount | grep -E 'containerd|kubelet|overlay' | awk '{print $3}' | sort -r); do
        umount -l "$m" 2>/dev/null || true
    done

    # Remove old Docker
    if command -v docker &>/dev/null; then
        apt-get remove -y docker.io docker-ce containerd.io 2>/dev/null || true
    fi

    # Remove binaries
    rm -f /usr/local/bin/{k3s,k3s-killall.sh,k3s-uninstall.sh,helm,kubectl,kubelet,kubeadm,etcd,etcdctl,ctr,crictl,olares-cli}
    rm -f /usr/local/sbin/runc
    rm -f /usr/bin/{containerd,ctr,runc,crictl}

    # Remove data
    rm -rf /var/lib/rancher /var/lib/etcd /var/lib/kubelet /var/lib/cni /var/lib/calico /var/lib/containerd /var/lib/openebs
    rm -rf /etc/rancher /etc/cni /etc/etcd.env /etc/ssl/etcd /etc/calico /etc/containerd
    rm -rf /run/containerd /run/flannel /run/calico /var/run/calico
    rm -rf /root/.kube /root/.olares /olares

    # Remove systemd units
    rm -f /etc/systemd/system/{k3s,etcd,olaresd,containerd}.service
    rm -f /etc/systemd/system/etcd-backup*
    rm -f /lib/systemd/system/containerd.service
    systemctl daemon-reload 2>/dev/null || true

    # Clean network
    ip link delete cni0 2>/dev/null || true
    ip link delete kube-ipvs0 2>/dev/null || true
    ip link delete vxlan.calico 2>/dev/null || true
    ip link delete flannel.1 2>/dev/null || true
    nft flush ruleset 2>/dev/null || true

    log_ok "Cleanup complete"
}

# --- Pre-flight checks ---
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        log_error "This script must be run as root"
        echo "  Try: sudo bash install.sh"
        exit 1
    fi
}

check_os() {
    if [ "$(uname)" != "Linux" ]; then
        log_error "Packalares requires Linux"
        exit 1
    fi
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="arm" ;;
        *)
            log_error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
    log_info "Architecture: $ARCH"
}

check_commands() {
    local missing=()
    for cmd in curl tar systemctl; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing required commands: ${missing[*]}"
        exit 1
    fi
}

# --- Download CLI ---
download_cli() {
    local cli_url="${RELEASE_BASE_URL}/v${PACKALARES_VERSION}/packalares-${ARCH}"
    local cli_path="/usr/local/bin/packalares"

    if [ -f "$cli_path" ]; then
        local current_version
        current_version=$("$cli_path" --version 2>/dev/null || echo "unknown")
        log_info "Existing CLI found (version: $current_version)"
        if [ "$current_version" = "$PACKALARES_VERSION" ]; then
            log_ok "CLI is already up to date"
            return 0
        fi
    fi

    log_info "Downloading Packalares CLI v${PACKALARES_VERSION} ..."
    if curl -sSL -o "$cli_path" "$cli_url" 2>&1; then
        chmod +x "$cli_path"
        log_ok "CLI downloaded to $cli_path"
    else
        log_warn "Could not download pre-built CLI from $cli_url"
        log_info "Attempting to build from source ..."
        build_cli_from_source
    fi
}

build_cli_from_source() {
    if ! command -v go &>/dev/null; then
        log_error "Go is required to build from source. Install Go 1.22+ first."
        exit 1
    fi

    local src_dir="${PACKALARES_BASE_DIR}/src"
    if [ -d "${src_dir}/packalares" ]; then
        log_info "Using existing source at ${src_dir}/packalares"
    else
        log_info "Cloning Packalares source ..."
        mkdir -p "$src_dir"
        git clone https://github.com/packalares/packalares.git "${src_dir}/packalares"
    fi

    log_info "Building CLI ..."
    cd "${src_dir}/packalares"
    go build -o /usr/local/bin/packalares ./cmd/cli/
    chmod +x /usr/local/bin/packalares
    log_ok "CLI built successfully"
}

# --- Run installation ---
run_install() {
    log_info "Starting Packalares installation ..."
    echo ""

    local args=()

    # Pass through any script arguments
    args+=("$@")

    # Add defaults if not overridden by arguments
    local has_username=false
    local has_domain=false
    local has_basedir=false
    for arg in "${args[@]:-}"; do
        case "$arg" in
            --username*) has_username=true ;;
            --domain*)   has_domain=true ;;
            --base-dir*) has_basedir=true ;;
        esac
    done

    if [ "$has_username" = false ] && [ -n "${PACKALARES_USERNAME:-}" ]; then
        args+=("--username" "$PACKALARES_USERNAME")
    fi
    if [ "$has_domain" = false ] && [ -n "${PACKALARES_DOMAIN:-}" ]; then
        args+=("--domain" "$PACKALARES_DOMAIN")
    fi
    if [ "$has_basedir" = false ] && [ -n "${PACKALARES_BASE_DIR:-}" ]; then
        args+=("--base-dir" "$PACKALARES_BASE_DIR")
    fi
    if [ -n "${PACKALARES_REGISTRY:-}" ]; then
        args+=("--registry" "$PACKALARES_REGISTRY")
    fi
    if [ -n "${PACKALARES_PASSWORD:-}" ]; then
        args+=("--password" "$PACKALARES_PASSWORD")
    fi
    if [ -n "${OLARES_CERT_MODE:-}" ]; then
        args+=("--cert-mode" "$OLARES_CERT_MODE")
    fi
    if [ -n "${OLARES_ACME_EMAIL:-}" ]; then
        args+=("--acme-email" "$OLARES_ACME_EMAIL")
    fi
    if [ -n "${OLARES_ACME_DNS_PROVIDER:-}" ]; then
        args+=("--acme-dns-provider" "$OLARES_ACME_DNS_PROVIDER")
    fi
    if [ -n "${OLARES_TAILSCALE_AUTH_KEY:-}" ]; then
        args+=("--tailscale-auth-key" "$OLARES_TAILSCALE_AUTH_KEY")
    fi
    if [ -n "${OLARES_TAILSCALE_CONTROL_URL:-}" ]; then
        args+=("--tailscale-control-url" "$OLARES_TAILSCALE_CONTROL_URL")
    fi

    packalares install "${args[@]}"
}

# --- Activation ---
run_activation() {
    log_info "Running activation ..."

    # Read generated password if available
    local password_file="${PACKALARES_BASE_DIR}/state/generated_password"
    local password="${PACKALARES_PASSWORD:-}"

    if [ -z "$password" ] && [ -f "$password_file" ]; then
        password=$(cat "$password_file")
    fi

    # Generate self-signed cert if using local mode
    local cert_mode="${OLARES_CERT_MODE:-local}"
    if [ "$cert_mode" = "local" ]; then
        log_info "Generating self-signed TLS certificate ..."
        generate_self_signed_cert
    fi

    log_ok "Activation complete"
}

generate_self_signed_cert() {
    local cert_dir="/etc/packalares/ssl"
    mkdir -p "$cert_dir"

    local domain="${PACKALARES_DOMAIN:-olares.local}"
    local username="${PACKALARES_USERNAME:-admin}"
    local zone="${username}.${domain}"

    if [ -f "$cert_dir/tls.crt" ] && [ -f "$cert_dir/tls.key" ]; then
        log_info "TLS certificate already exists at $cert_dir"
        return 0
    fi

    # Generate using openssl
    openssl req -x509 -nodes -days 3650 \
        -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
        -keyout "$cert_dir/tls.key" \
        -out "$cert_dir/tls.crt" \
        -subj "/CN=*.${zone}/O=Packalares" \
        -addext "subjectAltName=DNS:${zone},DNS:*.${zone},DNS:localhost,IP:127.0.0.1,IP:::1" \
        2>/dev/null

    chmod 600 "$cert_dir/tls.key"
    chmod 644 "$cert_dir/tls.crt"

    # Create Kubernetes TLS secret
    if command -v kubectl &>/dev/null; then
        kubectl create secret tls zone-ssl-secret \
            --cert="$cert_dir/tls.crt" \
            --key="$cert_dir/tls.key" \
            -n os-system \
            --dry-run=client -o yaml | kubectl apply -f - 2>/dev/null || true
    fi

    log_ok "Self-signed TLS certificate generated for *.${zone}"
}

# --- Print access info ---
print_access_info() {
    echo ""
    echo "=============================================="
    echo "  Packalares Installation Complete"
    echo "=============================================="
    echo ""

    local domain="${PACKALARES_DOMAIN:-olares.local}"
    local username="${PACKALARES_USERNAME:-admin}"
    local zone="${username}.${domain}"

    # Get host IP
    local host_ip
    host_ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    if [ -z "$host_ip" ]; then
        host_ip="<your-server-ip>"
    fi

    echo "  Access URL:     https://${zone}"
    echo "  Host IP:        ${host_ip}"
    echo "  Username:       ${username}"

    # Show password if we have it
    local password_file="${PACKALARES_BASE_DIR}/state/generated_password"
    if [ -n "${PACKALARES_PASSWORD:-}" ]; then
        echo "  Password:       (as specified)"
    elif [ -f "$password_file" ]; then
        echo "  Password:       $(cat "$password_file")"
    else
        echo "  Password:       (check installation output above)"
    fi

    echo ""
    echo "  kubectl:        export KUBECONFIG=/etc/rancher/k3s/k3s.yaml"
    echo "  Status:         packalares status"
    echo "  Uninstall:      packalares uninstall"
    echo ""

    if [ "$domain" = "olares.local" ]; then
        echo "  NOTE: Add to /etc/hosts on your devices:"
        echo "    ${host_ip}  ${zone} desktop.${zone} wizard.${zone}"
        echo ""
    fi

    if [ -n "${OLARES_TAILSCALE_AUTH_KEY:-}" ]; then
        echo "  Tailscale:      enabled (check 'tailscale status')"
        echo ""
    fi

    echo "=============================================="
}

# --- Main ---
main() {
    echo ""
    echo "  ____            _         _"
    echo " |  _ \\ __ _  ___| | ____ _| | __ _ _ __ ___  ___"
    echo " | |_) / _\` |/ __| |/ / _\` | |/ _\` | '__/ _ \\/ __|"
    echo " |  __/ (_| | (__|   < (_| | | (_| | | |  __/\\__ \\"
    echo " |_|   \\__,_|\\___|_|\\_\\__,_|_|\\__,_|_|  \\___||___/"
    echo ""
    echo "  Self-hosted Olares installer — v${PACKALARES_VERSION}"
    echo ""

    check_root
    check_os
    detect_arch
    check_commands
    cleanup_previous
    download_cli
    run_install "$@"
    run_activation
    print_access_info
}

main "$@"
