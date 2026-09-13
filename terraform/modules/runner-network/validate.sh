#!/usr/bin/env bash
set -euo pipefail

module_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$module_dir"

required_files=(main.tf variables.tf outputs.tf versions.tf README.md variables.example.tfvars)
for file in "${required_files[@]}"; do
  test -f "$file" || { printf 'missing required file: %s\n' "$file" >&2; exit 2; }
done

grep -Fq 'enable_vpc_endpoints' variables.tf || { echo 'endpoint opt-in variable is missing' >&2; exit 2; }
grep -Fq 'enable_nat_gateway' variables.tf || { echo 'NAT opt-in variable is missing' >&2; exit 2; }
grep -Fq 'default     = false' variables.tf || { echo 'a network creation guard is not disabled by default' >&2; exit 2; }
grep -Fq 'No ingress blocks are intentional' main.tf || { echo 'runner ingress guard is missing' >&2; exit 2; }
grep -Fq 'protocol    = "tcp"' main.tf || { echo 'TCP egress rule is missing' >&2; exit 2; }
grep -Fq 'from_port   = 443' main.tf || { echo 'HTTPS egress rule is missing' >&2; exit 2; }
grep -Fq 'from_port   = 53' main.tf || { echo 'DNS egress rule is missing' >&2; exit 2; }
grep -Fq 'cost_warnings' outputs.tf || { echo 'cost warnings output is missing' >&2; exit 2; }
grep -Fq 'reachability_labels' outputs.tf || { echo 'reachability labels output is missing' >&2; exit 2; }
grep -Fq 'variable "egress_mode"' variables.tf || { echo 'egress mode metadata variable is missing' >&2; exit 2; }

if rg -n 'resource\s+"aws_(vpc|subnet|route_table|internet_gateway)"' --glob '*.tf' .; then
  echo 'module must not create VPC, subnet, route-table, or internet-gateway resources' >&2
  exit 2
fi

if rg -n '(AKIA[0-9A-Z]{16}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|gh[pousr]_[A-Za-z0-9_]+)' .; then
  echo 'credential-like material found in runner-network module' >&2
  exit 2
fi

if command -v terraform >/dev/null 2>&1; then
  terraform fmt -check -recursive
else
  echo 'terraform not installed; skipped terraform fmt -check' >&2
fi

echo 'runner-network offline validation passed'
