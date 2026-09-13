#!/usr/bin/env bash
set -euo pipefail

module_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$module_dir"

required_files=(main.tf variables.tf outputs.tf versions.tf README.md)
for file in "${required_files[@]}"; do
  test -f "$file" || { printf 'missing required file: %s\n' "$file" >&2; exit 2; }
done

grep -Fq 'iam-policy-autopilot' README.md || { echo 'README must reference iam-policy-autopilot' >&2; exit 2; }
grep -Fq 'iam:PassRole' README.md || { echo 'README must document iam:PassRole scoping' >&2; exit 2; }
grep -Fq 'ec2:RunInstances' README.md || { echo 'README must document ec2:RunInstances scoping' >&2; exit 2; }
grep -Fq 'generated_policy_json' main.tf || { echo 'generated policy input is missing' >&2; exit 2; }
grep -Fq 'generated_policy_arns' main.tf || { echo 'generated policy ARN input is missing' >&2; exit 2; }

if rg -n '"Action"\s*:\s*"\*"|"Resource"\s*:\s*"\*"' --glob '*.tf' .; then
  echo 'unscoped IAM wildcard found in Terraform files' >&2
  exit 2
fi

if rg -n '(AKIA[0-9A-Z]{16}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|gh[pousr]_[A-Za-z0-9_]+)' .; then
  echo 'credential-like material found in controller-iam module' >&2
  exit 2
fi

if command -v terraform >/dev/null 2>&1; then
  terraform fmt -check -recursive
else
  echo 'terraform not installed; skipped terraform fmt -check' >&2
fi

echo 'controller-iam offline validation passed'
