#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "${1:-$(dirname "$0")/../..}" && pwd)"
CONFIGMAP="$ROOT/deploy/kubernetes/configmap.yaml"
SECRET="$ROOT/deploy/kubernetes/secret.yaml"
DEPLOYMENT="$ROOT/deploy/kubernetes/deployment.yaml"
CONFIG="$ROOT/controller/internal/config/config.go"
CONTRACT="$ROOT/tools/config-validation/config-contract.v1.json"

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$1"; }

for file in "$CONFIGMAP" "$SECRET" "$DEPLOYMENT" "$CONFIG" "$CONTRACT"; do
  test -f "$file" || fail "missing required file: $file"
done

grep -q '"version": "v1"' "$CONTRACT" || fail "contract version is not v1"
for key in GITHUB_WEBHOOK_SECRET GITHUB_APP_INSTALLATION_TOKEN AI_API_KEY; do
  grep -q "$key" "$CONTRACT" || fail "$key missing from contract"
done
pass "versioned secret contract exists"

for key in GITHUB_WEBHOOK_SECRET GITHUB_APP_INSTALLATION_TOKEN AI_API_KEY; do
  grep -Eq "^[[:space:]]+$key:[[:space:]]*\"\"[[:space:]]*$" "$SECRET" || fail "$key must be an empty out-of-band Secret placeholder"
done
secret_values="$(awk '/^[[:space:]]*[A-Z][A-Z0-9_]+:[[:space:]]*/ { line=$0; sub(/^[^:]+:[[:space:]]*/, "", line); if (line !~ /^\"\"[[:space:]]*$/ && line !~ /^#/) print }' "$SECRET")"
test -z "$secret_values" || fail "Secret manifest contains a non-empty value"
pass "Secret manifest contains no committed values"

for key in GITHUB_WEBHOOK_SECRET GITHUB_APP_INSTALLATION_TOKEN AI_API_KEY; do
  grep -q "^[[:space:]]*$key:" "$SECRET" || fail "$key missing from Secret manifest"
done
pass "all contract keys exist in Kubernetes Secret"

for key in GITHUB_WEBHOOK_SECRET GITHUB_APP_INSTALLATION_TOKEN; do
  grep -q "key: $key" "$DEPLOYMENT" || fail "$key missing from Deployment secretKeyRef"
done
grep -q 'optional: false' "$DEPLOYMENT" || fail "required webhook reference is not mandatory"
pass "required runtime Secret references are present"

for pair in 'GITHUB_JIT_ENABLED: "false"' 'AI_ENABLED: "false"' 'EXTENSIONS_ENABLED: "false"' 'AI_CONSENT_APPROVED: "false"'; do
  grep -q "^[[:space:]]*${pair}$" "$CONFIGMAP" || fail "unsafe default missing: $pair"
done
pass "unsafe features are disabled by default"

for symbol in SecretContract SafeSnapshot Diagnostics; do
  grep -q "func (c Config) $symbol" "$CONFIG" || fail "$symbol missing from config boundary"
done
if grep -nE 'fmt\.Sprintf\([^)]*(Secret|Token|APIKey|Webhook)' "$CONFIG" >/dev/null; then
  fail "secret-bearing fields appear in diagnostics formatting"
fi
pass "safe diagnostics and secret-free snapshot hooks exist"

printf 'PASS: configuration contract validation complete\n'
