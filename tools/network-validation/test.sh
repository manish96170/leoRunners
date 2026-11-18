#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
validator="$root/tools/network-validation/validate.sh"
measure="$root/tools/network-validation/measure.sh"
"$validator" --matrix "$root/network/reachability-matrix.pass.json" >/dev/null
if "$validator" --matrix "$root/network/reachability-matrix.fail.json" >/dev/null 2>&1; then
  echo 'unsafe reachability fixture unexpectedly passed' >&2
  exit 1
fi
if "$measure" --matrix "$root/network/reachability-matrix.pass.json" >/dev/null 2>&1; then
  echo 'measurement ran without confirmation' >&2
  exit 1
fi
for file in "$validator" "$measure"; do bash -n "$file"; done
printf '%s\n' 'network validation tests passed'
