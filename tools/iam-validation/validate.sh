#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
policy="${1:-$root/fixtures/policy-safe.json}"
contract="${2:-$root/fixtures/contract-safe.json}"

if [[ "${IAM_VALIDATION_APPLY:-false}" == "true" || "${IAM_VALIDATION_UPLOAD:-false}" == "true" ]]; then
  echo 'IAM validation is review-only; apply/upload is forbidden' >&2
  exit 2
fi
cd "$root"
exec go run . --policy "$policy" --contract "$contract"
