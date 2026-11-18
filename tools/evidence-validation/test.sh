#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
validation="$root/../../validation"
output=$(mktemp)
trap 'rm -f "$output"' EXIT

"$root/validate.sh" >"$output"
grep -q '^PASS ' "$output"
grep -q 'controlled validation evidence passed for 1 file' "$output"

for fixture in evidence-unsafe-secret.v1.json evidence-unsafe-raw.v1.json; do
  if "$root/validate.sh" "$validation/$fixture" >"$output" 2>&1; then
    printf 'unsafe fixture unexpectedly passed: %s\n' "$fixture" >&2
    exit 1
  fi
  grep -q '^FAIL ' "$output"
done
! grep -q 'ghp_NOT_FOR_STORAGE' "$output"

status=0
"$root/validate.sh" "$validation/does-not-exist.json" >/dev/null 2>&1 || status=$?
[ "$status" -eq 2 ]
printf '%s\n' 'evidence validation tests passed'
