#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
fixture_root="$repo_root/validation/security-artifacts"
validator="$script_dir/validate.sh"

bash -n "$validator"
bash -n "$script_dir/test.sh"
safe_report=$(mktemp "${TMPDIR:-/tmp}/artifact-security-safe.XXXXXX")
warn_report=$(mktemp "${TMPDIR:-/tmp}/artifact-security-warn.XXXXXX")
fail_output=$(mktemp "${TMPDIR:-/tmp}/artifact-security-fail.XXXXXX")
trap 'rm -f "$safe_report" "$warn_report" "$fail_output"' EXIT

"$validator" --dockerfile "$fixture_root/safe/Dockerfile" --manifest-dir "$fixture_root/safe" \
  --metadata "$fixture_root/safe/image-metadata.v1.json" --report "$safe_report" >/dev/null
grep -F '"status": "PASS"' "$safe_report" >/dev/null

"$validator" --dockerfile "$fixture_root/warn/Dockerfile" --manifest-dir "$fixture_root/warn" \
  --metadata "$fixture_root/warn/image-metadata.v1.json" --report "$warn_report" >/dev/null
grep -F '"status": "WARN"' "$warn_report" >/dev/null

if "$validator" --dockerfile "$fixture_root/fail/Dockerfile" --manifest-dir "$fixture_root/fail" \
  --metadata "$fixture_root/fail/image-metadata.v1.json" >"$fail_output" 2>&1; then
  echo 'unsafe artifact fixture unexpectedly passed' >&2
  exit 1
fi
grep -F 'FAIL ' "$fail_output" >/dev/null
grep -F 'kubernetes.host_access' "$fail_output" >/dev/null
if grep -E 'super-secret|sk-[A-Za-z0-9]+' "$fail_output" >/dev/null; then
  echo 'validator leaked fixture secret material' >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  "$validator" --dockerfile "$fixture_root/safe/Dockerfile" --manifest-dir "$fixture_root/safe" \
    --metadata "$fixture_root/safe/image-metadata.v1.json" --image unavailable:test >/dev/null
fi

mode=$(stat -f '%Lp' "$safe_report" 2>/dev/null || stat -c '%a' "$safe_report")
test "$mode" = 600
printf '%s\n' 'Artifact security validation tests passed'
