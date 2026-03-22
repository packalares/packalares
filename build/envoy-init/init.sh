#!/bin/sh
set -e
ENVOY_PORT="${ENVOY_PORT:-15001}"
iptables -t nat -N PACKALARES_INBOUND 2>/dev/null || true
iptables -t nat -N PACKALARES_REDIRECT 2>/dev/null || true
iptables -t nat -A PACKALARES_REDIRECT -p tcp -j REDIRECT --to-port ${ENVOY_PORT}
iptables -t nat -A PACKALARES_INBOUND -p tcp --dport ${ENVOY_PORT} -j RETURN
iptables -t nat -A PACKALARES_INBOUND -p tcp --dport 15000 -j RETURN
iptables -t nat -A PACKALARES_INBOUND -p tcp -s 127.0.0.1/32 -j RETURN
iptables -t nat -A PACKALARES_INBOUND -p tcp -j PACKALARES_REDIRECT
iptables -t nat -A PREROUTING -p tcp -j PACKALARES_INBOUND
echo "Envoy iptables init done (port ${ENVOY_PORT})"
