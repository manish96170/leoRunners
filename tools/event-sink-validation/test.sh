#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
events="$root/../../events"

"$root/validate.sh" "$events/sink-contract.safe.v1.json" "$events/sink-behavior.v1.json" >/dev/null
if "$root/validate.sh" "$events/sink-contract.unsafe.v1.json" "$events/sink-behavior.v1.json" >/dev/null 2>&1; then
  printf '%s\n' 'unsafe sink contract unexpectedly passed' >&2
  exit 1
fi
if "$root/validate.sh" "$events/sink-contract.safe.v1.json" "$events/sink-behavior.unsafe.v1.json" >/dev/null 2>&1; then
  printf '%s\n' 'unsafe behavior evidence unexpectedly passed' >&2
  exit 1
fi
"$root/../../events/validate.sh" >/dev/null
printf '%s\n' 'event sink validation tests passed'
