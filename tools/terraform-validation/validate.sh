#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH= cd -- "$root/../.." && pwd)

if [[ "${TERRAFORM_VALIDATION_APPLY:-false}" == "true" || "${TERRAFORM_VALIDATION_INIT:-false}" == "true" || "${TERRAFORM_VALIDATION_UPLOAD:-false}" == "true" ]]; then
  echo 'Terraform validation is offline and review-only; init/apply/upload are forbidden' >&2
  exit 2
fi

cd "$root"
exec go run . --root "$repo/terraform" "$@"
