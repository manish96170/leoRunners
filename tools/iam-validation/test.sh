#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$root"
go test .
go run . --policy "$root/fixtures/policy-safe.json" --contract "$root/fixtures/contract-safe.json" >/dev/null
if go run . --policy "$root/fixtures/policy-unsafe.json" --contract "$root/fixtures/contract-safe.json" >/dev/null 2>&1; then
  echo 'unsafe policy unexpectedly passed' >&2
  exit 1
fi
if IAM_VALIDATION_APPLY=true "$root/validate.sh" >/dev/null 2>&1; then
  echo 'apply guard unexpectedly passed' >&2
  exit 1
fi
echo 'IAM validation tests passed'
