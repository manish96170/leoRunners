#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
config_root="$root/../../observability"
output=$(mktemp)
secret_output=$(mktemp)
trap 'rm -f "$output" "$secret_output"' EXIT

"$root/validate.sh" >"$output" 2>&1
grep -q '^PASS ' "$output"
grep -q '^WARN ' "$output"
grep -q 'passed with 1 warning' "$output"

if "$root/validate.sh" "$config_root/alert-config-unsafe-secret.v1.yaml" >"$secret_output" 2>&1; then
  printf '%s\n' 'unsafe secret fixture unexpectedly passed' >&2
  exit 1
fi
grep -q '^FAIL ' "$secret_output"
! grep -q 'ghp_' "$secret_output"

if "$root/validate.sh" "$config_root/alert-config-unsafe-cardinality.v1.yaml" >/dev/null 2>&1; then
  printf '%s\n' 'unsafe cardinality fixture unexpectedly passed' >&2
  exit 1
fi

if ! "$root/validate.sh" "$config_root/alert-config-extensions.v1.yaml" >"$output" 2>&1; then
  printf '%s\n' 'extension alert fixture unexpectedly failed' >&2
  cat "$output" >&2
  exit 1
fi
grep -q '^PASS ' "$output"
grep -q 'passed with 0 warning' "$output"

if "$root/validate.sh" "$config_root/alert-config-unsafe-extension-action.v1.yaml" >/dev/null 2>&1; then
  printf '%s\n' 'unsafe extension action fixture unexpectedly passed' >&2
  exit 1
fi

if "$root/validate.sh" "$config_root/alert-config-unsafe-extension-cardinality.v1.yaml" >/dev/null 2>&1; then
  printf '%s\n' 'unsafe extension cardinality fixture unexpectedly passed' >&2
  exit 1
fi

printf '%s\n' 'observability validation tests passed'
