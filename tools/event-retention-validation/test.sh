#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
"$root/validate.sh" "$root/config.v1.json"
if "$root/validate.sh" "$root/unsafe-config.v1.json" >/dev/null 2>&1; then
  printf '%s\n' 'unsafe retention fixture unexpectedly passed' >&2
  exit 1
fi
printf '%s\n' 'event retention validation tests passed'
