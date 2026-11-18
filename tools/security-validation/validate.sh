#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
command -v go >/dev/null 2>&1 || {
    printf '%s\n' 'security validation requires Go 1.27 or newer' >&2
    exit 2
}

exec go run "$script_dir" --root "${SECURITY_VALIDATION_ROOT:-$script_dir/../..}" "$@"
