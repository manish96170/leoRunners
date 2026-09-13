#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
command -v rg >/dev/null
bash -n "$root/validate.sh"
"$root/validate.sh"
terraform fmt -check -recursive "$root"
printf 'Observability module tests passed\n'
