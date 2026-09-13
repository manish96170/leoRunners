#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
validator="$root/tools/controlled-workload-validation/validate.sh"
schema="$root/controlled-workload/execution.schema.v1.json"

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 2; }
jq empty "$schema" >/dev/null
"$validator" "$root/controlled-workload/execution.v1.json" >/dev/null

for fixture in \
  execution-unsafe-command.v1.json \
  execution-unsafe-network.v1.json \
  execution-unsafe-secret.v1.json \
  execution-unsafe-floating-ref.v1.json
do
  if "$validator" "$root/controlled-workload/$fixture" >/dev/null 2>&1; then
    echo "expected fixture to fail: $fixture" >&2
    exit 1
  fi
done

tmp=$(mktemp -d /tmp/leo-controlled-workload.XXXXXX)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
jq '.spec.request.commands[0].commandDigest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"' \
  "$root/controlled-workload/execution.v1.json" > "$tmp/digest-mismatch.json"
if "$validator" "$tmp/digest-mismatch.json" >/dev/null 2>&1; then
  echo "expected command digest mismatch to fail" >&2
  exit 1
fi

jq '.spec.request.limits.maxOutputBytes = 1023' "$root/controlled-workload/execution.v1.json" > "$tmp/unsafe-limit.json"
if "$validator" "$tmp/unsafe-limit.json" >/dev/null 2>&1; then
  echo "expected unsafe output limit to fail" >&2
  exit 1
fi

if grep -Eiq 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|secret[=:]|token[=:]|password[=:]|BEGIN .*PRIVATE KEY' \
  "$root/controlled-workload/execution.v1.json"; then
  echo "safe fixture contains secret-shaped content" >&2
  exit 1
fi

echo "controlled workload validation tests passed"
