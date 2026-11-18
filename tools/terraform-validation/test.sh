#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$root"

go test ./...
output=$(./validate.sh)
printf '%s\n' "$output"
grep -q 'provider-plugin-status:' <<<"$output"
grep -q 'status: WARN\|status: PASS' <<<"$output"

if TERRAFORM_VALIDATION_APPLY=true ./validate.sh >/dev/null 2>&1; then
  echo 'apply guard unexpectedly passed' >&2
  exit 1
fi
if TERRAFORM_VALIDATION_INIT=true ./validate.sh >/dev/null 2>&1; then
  echo 'init guard unexpectedly passed' >&2
  exit 1
fi

json=$(./validate.sh --json)
grep -q '"no_apply_or_init_performed": true' <<<"$json"
grep -q '"provider_plugin_status"' <<<"$json"

echo 'Terraform validation tests passed'
