#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  printf '%s\n' 'validate-image.sh must run as root' >&2
  exit 1
fi

required_commands=(curl git gzip jq make openssl tar unzip which)
for command_name in "${required_commands[@]}"; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf 'missing required command: %s\n' "${command_name}" >&2
    exit 1
  }
done

[[ -f /etc/leo-runners/image-profile ]] || {
  printf '%s\n' 'image profile marker is missing' >&2
  exit 1
}

grep -Fxq 'amazon-linux-2023-base-tools' /etc/leo-runners/image-profile || {
  printf '%s\n' 'unexpected image profile marker' >&2
  exit 1
}

# The AMI must not contain a persistent GitHub runner installation or common
# credential files. The JIT payload is injected only after the instance starts.
for path in /opt/actions-runner /actions-runner /root/.aws /home/ec2-user/.aws; do
  [[ ! -e "${path}" ]] || {
    printf 'forbidden persistent path exists: %s\n' "${path}" >&2
    exit 1
  }
done

for path in /etc/leo-runners/github-token /etc/leo-runners/jit-config /root/.ssh/id_rsa /home/ec2-user/.ssh/id_rsa; do
  [[ ! -e "${path}" ]] || {
    printf 'forbidden credential path exists: %s\n' "${path}" >&2
    exit 1
  }
done

printf '%s\n' 'AMI image validation passed'
