#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
validation="$root/../../validation"
output=$(mktemp)
trap 'rm -f "$output"' EXIT

"$root/validate-scope.sh" >"$output"
grep -q '^PASS ' "$output"
grep -q 'approved validation scopes passed for 1 file' "$output"

for fixture in approved-scope-unsafe-secret.v1.json approved-scope-unsafe-bounds.v1.json; do
  if "$root/validate-scope.sh" "$validation/$fixture" >"$output" 2>&1; then
    printf 'unsafe scope unexpectedly passed: %s\n' "$fixture" >&2
    exit 1
  fi
  grep -q '^FAIL ' "$output"
done
! grep -q 'ghp_NOT_FOR_STORAGE' "$output"

status=0
"$root/validate-scope.sh" "$validation/does-not-exist-scope.json" >/dev/null 2>&1 || status=$?
[ "$status" -eq 2 ]
printf '%s\n' 'approved scope validation tests passed'
