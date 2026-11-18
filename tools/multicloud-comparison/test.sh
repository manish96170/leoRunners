#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-multicloud.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

"$root/validate.sh" "$root/../../measurements/phase-51-comparison.v1.json" "$tmp/report.json" >/dev/null
jq -e '.status == "PASS" and .providers.aws.metrics.startup_ms.p95 == 110 and .providers.gcp.metrics.cost_usd.median == 0.022 and .real_world_claim == false' "$tmp/report.json" >/dev/null

if "$root/validate.sh" "$root/../../measurements/phase-51-comparison-regression.v1.json" >/dev/null 2>&1; then
  echo "expected regression fixture to fail" >&2
  exit 1
fi
if "$root/validate.sh" "$root/../../measurements/phase-51-comparison-secret.v1.json" >/dev/null 2>&1; then
  echo "expected secret fixture to fail" >&2
  exit 1
fi

"$root/validate.sh" "$root/../../measurements/phase-51-comparison-missing-gcp.v1.json" "$tmp/missing.json" >/dev/null
jq -e '.status == "WARN" and .providers.gcp.status == "unavailable" and all(.comparisons[]; .gcp.status == "unavailable")' "$tmp/missing.json" >/dev/null
echo "multi-cloud comparison tests passed"
