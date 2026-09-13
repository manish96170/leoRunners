#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
"$root/validate.sh" "$root/baseline.json" "$root/candidate.json" >/dev/null

tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-benchmark.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

if jq '.provenance.execution_class = "real"' "$root/candidate.json" >"$tmp/mixed.json" && \
  "$root/validate.sh" "$root/baseline.json" "$tmp/mixed.json" >/dev/null 2>&1; then
  echo "expected synthetic/real mismatch to fail" >&2
  exit 1
fi

if jq '.cost.total_usd = 99 | .cost.compute_usd = 99 | .cost.storage_usd = 0 | .cost.network_usd = 0' "$root/candidate.json" >"$tmp/cost.json" && \
  "$root/validate.sh" "$root/baseline.json" "$tmp/cost.json" >/dev/null 2>&1; then
  echo "expected cost regression to fail" >&2
  exit 1
fi

if jq '.redaction.raw_payloads_excluded = false' "$root/candidate.json" >"$tmp/unredacted.json" && \
  "$root/validate.sh" "$root/baseline.json" "$tmp/unredacted.json" >/dev/null 2>&1; then
  echo "expected unredacted evidence to fail" >&2
  exit 1
fi

echo "benchmark evidence tests passed"
