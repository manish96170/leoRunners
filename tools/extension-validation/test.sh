#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
extensions="$root/../../extensions"
output=$(mktemp)
trap 'rm -f "$output"' EXIT

"$root/validate.sh" >"$output"
grep -q "PASS .*runtime policy default-runtime-policy@1.0.0" "$output"
grep -q "PASS .*: analytics" "$output"
grep -q "PASS .*: security" "$output"

for fixture in \
  runtime-policy-unsafe-wildcard.v1.yaml \
  runtime-policy-unsafe-bounds.v1.yaml \
  runtime-policy-unsafe-consumer.v1.yaml; do
  if "$root/validate.sh" "$extensions/$fixture" >"$output" 2>&1; then
    echo "unsafe fixture unexpectedly passed: $fixture" >&2
    exit 1
  fi
  grep -q '^FAIL ' "$output"
done

if "$root/validate.sh" "$extensions/runtime-policy.v1.yaml" "$extensions/runtime-policy.v1.yaml" >"$output" 2>&1; then
  echo 'duplicate policy unexpectedly passed' >&2
  exit 1
fi

echo 'extension validation tests passed'
