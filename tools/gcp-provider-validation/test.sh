#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-gcp-provider-validation.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

sh -n "$script_dir/validate.sh"
sh -n "$script_dir/real-gcp.sh"
"$script_dir/validate.sh" >/dev/null
for fixture in provider-contract-unsafe.v1.json provider-contract-unsafe-scope.v1.json provider-contract-unsafe-secret.v1.json; do
  if "$script_dir/validate.sh" --contract "$repo_root/validation/gcp-provider/$fixture" >/dev/null 2>&1; then
    echo "unsafe contract unexpectedly passed: $fixture" >&2
    exit 1
  fi
done
printf '{' >"$tmp/malformed.json"
if "$script_dir/validate.sh" --contract "$tmp/malformed.json" >/dev/null 2>&1; then
  echo 'malformed contract unexpectedly passed' >&2
  exit 1
fi
if "$script_dir/real-gcp.sh" >/dev/null 2>&1; then
  echo 'live procedure ran without guard' >&2
  exit 1
fi
if "$script_dir/real-gcp.sh" --allow-live --confirm WRONG >/dev/null 2>&1; then
  echo 'live procedure accepted wrong confirmation' >&2
  exit 1
fi
if "$script_dir/real-gcp.sh" --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES >/dev/null 2>&1; then
  echo 'live procedure unexpectedly executed' >&2
  exit 1
fi
echo 'gcp-provider-validation: PASS'
