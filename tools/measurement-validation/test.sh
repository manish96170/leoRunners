#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH= cd -- "$root/../.." && pwd)
"$root/validate.sh" "$repo/measurements/evidence.v1.json" >/dev/null

tmp=$(mktemp -d /tmp/leo-measurement.XXXXXX)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

if "$root/validate.sh" "$repo/measurements/evidence-unsafe-secret.v1.json" >/dev/null 2>&1; then
  echo "expected secret fixture to fail" >&2
  exit 1
fi
if "$root/validate.sh" "$repo/measurements/evidence-unsafe-units.v1.json" >/dev/null 2>&1; then
  echo "expected unit fixture to fail" >&2
  exit 1
fi

jq '.spec.provenance.execution_class = "real" | .spec.provenance.evidence_level = "measured" | .spec.provenance.source = "controlled-run" | .spec.comparability.pricing_source = "aws-pricing-api"' \
  "$repo/measurements/evidence.v1.json" > "$tmp/real.json"
"$root/validate.sh" "$tmp/real.json" >/dev/null
"$root/validate.sh" "$repo/measurements/evidence.v1.json" "$repo/measurements/evidence.v1.json" >/dev/null

jq '.spec.cache.operations.hit_count = 6' "$repo/measurements/evidence.v1.json" > "$tmp/inconsistent.json"
if "$root/validate.sh" "$tmp/inconsistent.json" >/dev/null 2>&1; then
  echo "expected cache counter mismatch to fail" >&2
  exit 1
fi

jq '.spec.comparability.comparison_key = "different-key"' "$repo/measurements/evidence.v1.json" > "$tmp/incomparable.json"
if "$root/validate.sh" "$repo/measurements/evidence.v1.json" "$tmp/incomparable.json" >/dev/null 2>&1; then
  echo "expected incomparable records to fail" >&2
  exit 1
fi

if grep -Eiq 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|secret[=:]|token[=:]|password[=:]' "$repo/measurements/evidence.v1.json"; then
  echo "safe fixture contains secret-shaped content" >&2
  exit 1
fi

echo "measurement validation tests passed"
