#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
sh -n "$root/validate.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-final-readiness-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
report="$tmp/readiness.json"
set +e
FINAL_READINESS_REPORT="$report" "$root/validate.sh" >"$tmp/out" 2>&1
status=$?
set -e
[ "$status" -eq 0 ] || [ "$status" -eq 1 ]
test -s "$report"
grep -F '"read_only": true' "$report" >/dev/null
grep -F '"live_mutation_invoked": false' "$report" >/dev/null
! grep -E 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]+' "$report" >/dev/null
printf '%s\n' 'final readiness tests passed'
