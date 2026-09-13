#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH= cd -- "$root/../.." && pwd)
validator="$root/validate.sh"

"$validator" "$repo/workload-intake/intake.v1.json" >/dev/null
"$validator" "$repo/workload-intake/intake.v1.json" "$repo/workload-intake/intake.v1.json" >/dev/null

for fixture in \
  intake-unsafe-secret.v1.json \
  intake-unsafe-floating-ref.v1.json \
  intake-unsafe-command.v1.json \
  intake-unsafe-no-approval.v1.json
do
  if "$validator" "$repo/workload-intake/$fixture" >/dev/null 2>&1; then
    echo "expected fixture to fail: $fixture" >&2
    exit 1
  fi
done

tmp=$(mktemp -d /tmp/leo-workload-intake.XXXXXX)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

jq '.spec.comparability.comparisonKey = "different-intake"' \
  "$repo/workload-intake/intake.v1.json" > "$tmp/incomparable.json"
if "$validator" "$repo/workload-intake/intake.v1.json" "$tmp/incomparable.json" >/dev/null 2>&1; then
  echo "expected incomparable intake to fail" >&2
  exit 1
fi

jq '.spec.workload.commandDigest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"' \
  "$repo/workload-intake/intake.v1.json" > "$tmp/digest-mismatch.json"
if "$validator" "$tmp/digest-mismatch.json" >/dev/null 2>&1; then
  echo "expected command digest mismatch to fail" >&2
  exit 1
fi

if grep -Eiq 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|secret[=:]|token[=:]|password[=:]|BEGIN .*PRIVATE KEY' \
  "$repo/workload-intake/intake.v1.json"; then
  echo "safe fixture contains secret-shaped content" >&2
  exit 1
fi

echo "workload intake validation tests passed"
