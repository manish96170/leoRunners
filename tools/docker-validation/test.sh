#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
sh -n "$script_dir/validate.sh"
sh -n "$script_dir/test.sh"
grep -F 'docker info' "$script_dir/validate.sh" >/dev/null
grep -F 'docker image inspect' "$script_dir/validate.sh" >/dev/null
grep -F 'docker run --rm --entrypoint /bin/sh' "$script_dir/validate.sh" >/dev/null
grep -F '65532:65532' "$script_dir/validate.sh" >/dev/null
grep -F '8080/tcp' "$script_dir/validate.sh" >/dev/null
grep -F '/usr/local/bin/runner-controller' "$script_dir/validate.sh" >/dev/null
grep -F -- '--run-smoke' "$script_dir/validate.sh" >/dev/null
printf '%s\n' 'Docker validation shell tests passed'
