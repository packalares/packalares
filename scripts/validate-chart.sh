#!/usr/bin/env bash
# validate-chart.sh — Pre-packaging validation for Packalares app charts
# Usage: ./scripts/validate-chart.sh market/charts/appname/

set -euo pipefail

CHART_DIR="${1:?Usage: validate-chart.sh <chart-dir>}"
CHART_NAME=$(basename "$CHART_DIR")
ERRORS=0

err() { echo "  ERROR: $1"; ((ERRORS++)); }
warn() { echo "  WARN:  $1"; }
info() { echo "  INFO:  $1"; }

echo "=== Validating chart: $CHART_NAME ==="
echo ""

# 1. Required files
echo "[1] Required files"
for f in Chart.yaml OlaresManifest.yaml values.yaml; do
  if [[ ! -f "$CHART_DIR/$f" ]]; then
    err "Missing $f"
  else
    info "$f exists"
  fi
done
if [[ ! -d "$CHART_DIR/templates" ]]; then
  err "Missing templates/ directory"
fi
echo ""

# 2. No sub-charts
echo "[2] No sub-charts"
if find "$CHART_DIR" -mindepth 1 -maxdepth 1 -type d ! -name templates | grep -q .; then
  err "Found sub-chart directories (must be flattened):"
  find "$CHART_DIR" -mindepth 1 -maxdepth 1 -type d ! -name templates -exec basename {} \;
else
  info "No sub-charts found"
fi
echo ""

# 3. Hardcoded passwords
echo "[3] Hardcoded passwords/secrets"
if grep -rn "password.*=.*\"[a-zA-Z0-9]" "$CHART_DIR/templates/" 2>/dev/null | grep -v "valueFrom" | grep -v "olaresEnv" | grep -v '""' | grep -v "{{"; then
  err "Found hardcoded passwords in templates"
else
  info "No hardcoded passwords"
fi
echo ""

# 4. Localhost references
echo "[4] Localhost references"
if grep -rn "localhost\|127\.0\.0\.1" "$CHART_DIR/templates/" 2>/dev/null | grep -v "#"; then
  err "Found localhost references (use service names)"
else
  info "No localhost references"
fi
echo ""

# 5. DATABASE_URL template vars
echo "[5] DATABASE_URL uses template vars"
if grep -rn "DATABASE_URL" "$CHART_DIR/templates/" 2>/dev/null | grep -v "olaresEnv\|Values\|UNIQUE_PASS" | grep -v "#"; then
  warn "DATABASE_URL may have hardcoded values"
fi
echo ""

# 6. Image verification
echo "[6] Image tags"
images=$(grep -rn "image:" "$CHART_DIR/templates/" 2>/dev/null | grep -v "#" | sed 's/.*image: *["]*//;s/["]*$//' | sort -u)
for img in $images; do
  if [[ "$img" == *"latest"* ]]; then
    err "Image uses :latest tag: $img (pin to specific version)"
  else
    info "Image: $img"
  fi
done
echo ""

# 7. DirectoryOrCreate
echo "[7] HostPath type"
if grep -rn "type: Directory$" "$CHART_DIR/templates/" 2>/dev/null; then
  err "Found 'type: Directory' (must be DirectoryOrCreate)"
else
  info "All hostPath volumes use DirectoryOrCreate"
fi
echo ""

# 8. Instance labels
echo "[8] Instance labels"
if ! grep -rn "app.kubernetes.io/instance:" "$CHART_DIR/templates/" 2>/dev/null | grep -q "$CHART_NAME"; then
  err "Missing app.kubernetes.io/instance: $CHART_NAME label"
else
  info "Instance label found"
fi
echo ""

# 9. Namespace
echo "[9] Namespace templating"
if grep -rn "namespace:" "$CHART_DIR/templates/" 2>/dev/null | grep -v "Release.Namespace" | grep -v "#" | grep -v "fieldRef"; then
  err "Found hardcoded namespace (use {{ .Release.Namespace }})"
else
  info "All namespaces use Release.Namespace"
fi
echo ""

# 10. ProviderRegistry
echo "[10] No ProviderRegistry"
if grep -rn "ProviderRegistry" "$CHART_DIR/templates/" 2>/dev/null; then
  err "Found ProviderRegistry (not supported in Packalares)"
else
  info "No ProviderRegistry"
fi
echo ""

# 11. ConfigMap name collisions
echo "[11] ConfigMap names"
cms=$(grep -rn "name:" "$CHART_DIR/templates/" 2>/dev/null | grep -i configmap -A1 | grep "name:" | sed 's/.*name: *//;s/"//g' | sort -u)
for cm in $cms; do
  case "$cm" in
    nginx-config|script|redis-config|env-config|config)
      warn "Generic ConfigMap name '$cm' — prefix with app name to avoid collisions"
      ;;
  esac
done
echo ""

# 12. Market assets
echo "[12] Market assets"
MARKET_DIR=$(dirname $(dirname "$CHART_DIR"))
if [[ ! -f "$MARKET_DIR/icons/${CHART_NAME}.png" ]]; then
  err "Missing icon: icons/${CHART_NAME}.png"
else
  info "Icon exists"
fi
if [[ ! -d "$MARKET_DIR/screenshots/${CHART_NAME}" ]]; then
  err "Missing screenshots directory: screenshots/${CHART_NAME}/"
else
  SC_COUNT=$(ls "$MARKET_DIR/screenshots/${CHART_NAME}/" 2>/dev/null | wc -l)
  info "Screenshots: $SC_COUNT files"
fi
echo ""

# 13. Catalog entry
echo "[13] Catalog entry"
if [[ -f "$MARKET_DIR/catalog.json" ]]; then
  if python3 -c "
import json, sys
data = json.load(open('$MARKET_DIR/catalog.json'))
apps = data.get('apps', [])
found = any(a.get('name') == '$CHART_NAME' for a in apps if isinstance(a, dict))
sys.exit(0 if found else 1)
" 2>/dev/null; then
    info "Found in catalog.json"
  else
    err "Not found in catalog.json"
  fi
else
  err "catalog.json not found"
fi
echo ""

# Summary
echo "=== Summary ==="
if [[ $ERRORS -gt 0 ]]; then
  echo "  FAILED: $ERRORS error(s) found"
  exit 1
else
  echo "  PASSED: All checks OK"
  exit 0
fi
