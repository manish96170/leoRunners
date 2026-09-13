#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
activation="$root/../../activation"
output=$(mktemp)
trap 'rm -f "$output"' EXIT

export LEO_ACTIVATION_NOW=2026-09-14T10:00:00Z
"$root/validate.sh" "$activation/activation-request.v1.json" "$activation/activation-request-lifecycle.v1.json" >"$output"
grep -q 'activation requests passed for 2 file' "$output"

for fixture in unsafe-secret.v1.json unsafe-wildcard.v1.json unsafe-expired.v1.json unsafe-owner.v1.json unsafe-lifecycle-confirmation.v1.json; do
  if "$root/validate.sh" "$activation/fixtures/$fixture" >"$output" 2>&1; then
    printf 'unsafe fixture unexpectedly passed: %s\n' "$fixture" >&2
    exit 1
  fi
  grep -q '^FAIL ' "$output"
done
if "$root/validate.sh" "$activation/fixtures/unsafe-secret.v1.json" >"$output" 2>&1; then
  exit 1
fi
! grep -q 'ghp_NOT_FOR_STORAGE' "$output"

status=0
"$root/validate.sh" "$activation/missing.json" >/dev/null 2>&1 || status=$?
[ "$status" -eq 2 ]
printf '%s\n' 'activation validation tests passed'
