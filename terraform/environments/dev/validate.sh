#!/usr/bin/env bash
set -euo pipefail

environment_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$environment_dir"

required_files=(main.tf provider.tf variables.tf outputs.tf versions.tf README.md terraform.tfvars.example backend-disabled.tf.example)
for file in "${required_files[@]}"; do
  test -f "$file" || { printf 'missing required file: %s\n' "$file" >&2; exit 2; }
done

grep -Fq 'source = "../../modules/controller-state"' main.tf || { echo 'controller-state module is not composed' >&2; exit 2; }
grep -Fq 'source = "../../modules/controller-iam"' main.tf || { echo 'controller-iam module is not composed' >&2; exit 2; }
grep -Fq 'source = "../../modules/runner-runtime"' main.tf || { echo 'runner-runtime module is not composed' >&2; exit 2; }
grep -Fq 'controller_generated_policy_json' variables.tf || { echo 'reviewed policy input is missing' >&2; exit 2; }
grep -Fq 'nullable    = false' variables.tf || { echo 'required policy must not be nullable' >&2; exit 2; }
grep -Fq 'init -backend=false' README.md || { echo 'backend-disabled workflow is missing' >&2; exit 2; }

if rg -n 'backend\s+"|backend\s*=' --glob '*.tf' .; then
  echo 'unexpected configured Terraform backend found; use an explicit reviewed root' >&2
  exit 2
fi

if rg -n '(AKIA[0-9A-Z]{16}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|gh[pousr]_[A-Za-z0-9_]+)' .; then
  echo 'credential-like material found in dev composition' >&2
  exit 2
fi

if command -v terraform >/dev/null 2>&1; then
  terraform fmt -check -recursive
else
  echo 'terraform not installed; skipped terraform fmt -check' >&2
fi

echo 'dev Terraform composition offline validation passed'
