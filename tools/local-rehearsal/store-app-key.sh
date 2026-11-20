#!/usr/bin/env bash
# Stores a GitHub App private key (PEM) in floci's Secrets Manager.
#   store-app-key.sh /path/to/key.pem
# Emulates what ECS does in AWS: the controller never reads Secrets Manager
# itself; run-controller.sh fetches the value and injects it as an environment
# variable. The key is never printed.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need aws; need openssl
load_state
require_floci
key=${1:?usage: store-app-key.sh /path/to/private-key.pem}
[ -f "$key" ] || die "no such file: $key"
openssl rsa -in "$key" -check -noout >/dev/null 2>&1 || die "$key is not a readable RSA private key"
aws secretsmanager put-secret-value --secret-id "$SECRET_NAME" --secret-string "file://$key" >/dev/null
log "stored $(openssl rsa -in "$key" -noout -text 2>/dev/null | head -1) in $SECRET_NAME"
log "you can delete the downloaded key file now"
