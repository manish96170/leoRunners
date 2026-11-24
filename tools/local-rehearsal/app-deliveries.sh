#!/usr/bin/env bash
# Lists the most recent webhook deliveries GitHub attempted for your App: when,
# which event, and what status code the controller answered. It signs a
# short-lived App JWT with the private key stored in floci and calls
# GET /app/hook/deliveries. Neither the key nor the JWT is printed, and no
# payload is fetched.
#   app-deliveries.sh [app-id] [count]
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need aws; need openssl; need curl; need python3
load_state
require_floci

app_id=${1:-${GITHUB_APP_ID:-}}
count=${2:-10}
[ -n "$app_id" ] || die "usage: app-deliveries.sh <app-id> [count]"
case "$app_id" in *[!A-Za-z0-9._-]*) die "app id contains unexpected characters" ;; esac
case "$count" in '' | *[!0-9]*) die "count must be a number" ;; esac

b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
key_file=$(mktemp)
trap 'rm -f "$key_file"' EXIT
aws secretsmanager get-secret-value --secret-id "$SECRET_NAME" --query SecretString --output text > "$key_file" 2>/dev/null \
  || die "no key in floci; run store-app-key.sh first"

now=$(date +%s)
header=$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)
claims=$(printf '{"iat":%s,"exp":%s,"iss":"%s"}' "$((now - 60))" "$((now + 540))" "$app_id" | b64url)
signature=$(printf '%s.%s' "$header" "$claims" | openssl dgst -sha256 -sign "$key_file" | b64url)
jwt="$header.$claims.$signature"

response=$(curl -sS -m 20 -w '\n%{http_code}' "https://api.github.com/app/hook/deliveries?per_page=$count" \
  -H "Authorization: Bearer $jwt" -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2022-11-28")
status=${response##*$'\n'}
body=${response%$'\n'*}
[ "$status" = 200 ] || die "GitHub returned HTTP $status for the delivery list"

printf '%s' "$body" | python3 -c "$(cat <<'PY'
import json, sys
rows = json.load(sys.stdin)
if not rows:
    print("no deliveries yet")
    sys.exit(0)
print("%-22s %-14s %-12s %-5s %s" % ("delivered_at", "event", "action", "code", "status"))
for r in rows:
    print("%-22s %-14s %-12s %-5s %s" % (r.get("delivered_at", ""), r.get("event", ""), r.get("action") or "", r.get("status_code"), r.get("status")))
PY
)"
