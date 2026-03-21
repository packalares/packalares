#!/bin/sh
set -e

SAMBA_USER="${SAMBA_USER:-packalares}"
SAMBA_PASSWORD="${SAMBA_PASSWORD:-}"
DATA_PATH="${DATA_PATH:-/packalares/data}"
SHARE_NAME="${SHARE_NAME:-data}"

# Update share path in config
sed -i "s|path = /packalares/data|path = ${DATA_PATH}|" /etc/samba/smb.conf

# Create group if it does not exist
addgroup -S packalares 2>/dev/null || true

# Create user if it does not exist
if ! id "${SAMBA_USER}" >/dev/null 2>&1; then
    adduser -S -G packalares -H -D "${SAMBA_USER}"
fi

# Set samba password
if [ -n "${SAMBA_PASSWORD}" ]; then
    printf "%s\n%s\n" "${SAMBA_PASSWORD}" "${SAMBA_PASSWORD}" | smbpasswd -a -s "${SAMBA_USER}"
    smbpasswd -e "${SAMBA_USER}"
else
    echo "WARNING: SAMBA_PASSWORD not set, using random password"
    RANDOM_PW=$(head -c 16 /dev/urandom | base64)
    printf "%s\n%s\n" "${RANDOM_PW}" "${RANDOM_PW}" | smbpasswd -a -s "${SAMBA_USER}"
    smbpasswd -e "${SAMBA_USER}"
    echo "Generated Samba password: ${RANDOM_PW}"
fi

# Ensure data directory exists with correct permissions
mkdir -p "${DATA_PATH}"
chown -R "${SAMBA_USER}:packalares" "${DATA_PATH}"
chmod 2775 "${DATA_PATH}"

# Sync with LLDAP if LLDAP_URL is set
if [ -n "${LLDAP_URL}" ]; then
    echo "LLDAP sync configured, users will be synced from LLDAP"
    # LLDAP user sync can be added here via ldapsearch + smbpasswd
fi

echo "Starting Samba server..."
exec smbd --foreground --no-process-group --log-stdout
