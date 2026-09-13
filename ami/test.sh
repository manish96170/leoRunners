#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "${script_dir}"

for script in scripts/*.sh; do
  bash -n "${script}"
done

grep -Fq 'source "amazon-ebs" "amazon_linux_x86_64"' runner.pkr.hcl
grep -Fq 'http_tokens                 = "required"' runner.pkr.hcl
grep -Fq 'imds_support = "v2.0"' runner.pkr.hcl
grep -Fq 'source_ami' runner.pkr.hcl
grep -Fq 'vpc_id' runner.pkr.hcl
grep -Fq 'subnet_id' runner.pkr.hcl
grep -Fq 'iam_instance_profile' runner.pkr.hcl
grep -Fq 'script            = "${path.root}/scripts/install-base-tools.sh"' runner.pkr.hcl

if rg -n -i 'aws_access_key|aws_secret|github_pat|github_token|encoded_jit_config|BEGIN (RSA|OPENSSH|EC)' \
  --glob '!test.sh' --glob '!validate.sh' --glob '!README.md' .; then
  printf '%s\n' 'credential-like material found in AMI assets' >&2
  exit 1
fi

if rg -n 'curl[^\n]*actions-runner|config\.sh|run\.sh' scripts runner.pkr.hcl; then
  printf '%s\n' 'runner installation or registration found in base-image assets' >&2
  exit 1
fi

printf '%s\n' 'offline AMI asset validation passed'
