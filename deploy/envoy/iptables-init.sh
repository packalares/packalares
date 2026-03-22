#!/bin/sh
# Redirect all inbound TCP traffic to Envoy's inbound listener (port 15001)
# Except:
# - Traffic from Envoy itself (UID 1337)
# - Traffic to Envoy admin port (15000)
# - Traffic to/from localhost

set -e

ENVOY_PORT=15001
ENVOY_UID=1337

# Create new chain
iptables -t nat -N PACKALARES_INBOUND 2>/dev/null || true
iptables -t nat -N PACKALARES_IN_REDIRECT 2>/dev/null || true

# Redirect inbound traffic to Envoy
iptables -t nat -A PACKALARES_IN_REDIRECT -p tcp -j REDIRECT --to-port ${ENVOY_PORT}

# Skip Envoy's own traffic
iptables -t nat -A PACKALARES_INBOUND -p tcp --dport ${ENVOY_PORT} -j RETURN
iptables -t nat -A PACKALARES_INBOUND -p tcp --dport 15000 -j RETURN
# Skip localhost
iptables -t nat -A PACKALARES_INBOUND -p tcp -s 127.0.0.1/32 -j RETURN
# Redirect everything else
iptables -t nat -A PACKALARES_INBOUND -p tcp -j PACKALARES_IN_REDIRECT

# Apply to PREROUTING
iptables -t nat -A PREROUTING -p tcp -j PACKALARES_INBOUND

echo "iptables rules configured for Envoy sidecar"
