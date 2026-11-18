#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
sh -n "$script_dir/smoke.sh"
test -s "$script_dir/fixtures/workflow_job_queued.json"
"$script_dir/smoke.sh"
printf '%s\n' 'smoke harness tests passed'
