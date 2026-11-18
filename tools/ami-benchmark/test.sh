#!/usr/bin/env bash
set -Eeuo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH= cd -- "$root/../.." && pwd)
go_bin=${GO_BIN:-go}
cd "$root"
"$go_bin" test ./...
tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-ami-benchmark.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
report="$tmp/report.json"
PATH="$repo/tools/ami-benchmark:$PATH" "$go_bin" run "$root" -count 20 -contract "$repo/ami/image-contract.v1.json" -manifest "$repo/ami/manifest.example.v1.json" -output "$report"
test -s "$report"
grep -F '"status": "PASS"' "$report" >/dev/null
grep -F '"image_digest": "sha256:' "$report" >/dev/null
grep -F '"redacted": true' "$report" >/dev/null
if "$go_bin" run "$root" -mode real-ec2 >/dev/null 2>&1; then
  printf '%s\n' 'real EC2 mode unexpectedly executed' >&2
  exit 1
fi
if "$root/real-ec2.sh" WRONG_TOKEN >/dev/null 2>&1; then
  printf '%s\n' 'real EC2 guard accepted an incorrect token' >&2
  exit 1
fi
printf '%s\n' 'AMI benchmark tests passed'
