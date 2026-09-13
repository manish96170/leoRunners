#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "${script_dir}"

if ! command -v packer >/dev/null 2>&1; then
  printf '%s\n' 'packer is required for HCL validation; install it or run ./test.sh for offline checks' >&2
  exit 127
fi

packer fmt -check -diff .
packer init -upgrade .
packer validate -var-file=example.pkrvars.hcl runner.pkr.hcl
