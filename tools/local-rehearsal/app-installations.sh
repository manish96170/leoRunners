#!/usr/bin/env bash
# Lists the installations of your GitHub App (account and installation ID), so
# you can fill in GITHUB_APP_INSTALLATION_ID. It signs a short-lived App JWT with
# the private key stored in floci and calls GET /app/installations. The key and
# the JWT are never printed.
#   app-installations.sh <app-id-or-client-id>
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need aws; need openssl; need curl; need python3
load_state
require_floci

app_id=${1:-${GITHUB_APP_ID:-}}
[ -n "$app_id" ] || die "usage: app-installations.sh <app-id-or-client-id>"
case "$app_id" in *[!A-Za-z0-9._-]*) die "app id contains unexpected characters" ;; esac

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

response=$(curl -sS -m 20 -w '\n%{http_code}' https://api.github.com/app/installations \
  -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2022-11-28" \
  -H "Authorization: Bearer $jwt")
code=${response##*$'\n'}
body=${response%$'\n'*}
[ "$code" = 200 ] || die "GitHub answered HTTP $code: $(printf '%s' "$body" | head -c 300)"
printf '%s' "$body" | python3 -c '
import json, sys
installs = json.load(sys.stdin)
if not installs:
    print("The App has no installations yet. Install it on your test org or repo first.")
for i in installs:
    print("installation id: %s   account: %s (%s)   repositories: %s" % (i["id"], i["account"]["login"], i["account"]["type"], i.get("repository_selection")))
'
