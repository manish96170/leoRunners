#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fixtures="$root/../../validation/recovery"

"$root/validate.sh" "$fixtures/recovery-evidence.v1.json" >/dev/null
for unsafe in recovery-evidence-unsafe-secret.v1.json recovery-evidence-unsafe-invariants.v1.json
do
  if "$root/validate.sh" "$fixtures/$unsafe" >/dev/null 2>&1
  then
    printf 'unsafe recovery fixture unexpectedly passed: %s\n' "$unsafe" >&2
    exit 1
  fi
done

if "$root/validate.sh" "$fixtures/does-not-exist.json" >/dev/null 2>&1
then
  printf '%s\n' 'missing recovery fixture unexpectedly passed' >&2
  exit 1
fi

ruby -rjson -e 'JSON.parse(File.read(ARGV[0])); JSON.parse(File.read(ARGV[1]))' \
  "$fixtures/recovery-evidence.schema.v1.json" "$fixtures/recovery-evidence.v1.json"
sh -n "$root/validate.sh" "$root/test.sh"
printf '%s\n' 'recovery validation tests passed'
