#!/usr/bin/env bash
# Brings the temporary environment up or down with whatever Terraform is on PATH.
#   terraform-environment.sh up|down <run-id> <state-dir>
set -euo pipefail
action=${1:?up|down}; run_id=${2:?run id}; state_dir=${3:?state dir}
here=$(CDPATH= cd -- "$(dirname -- "$0")/../terraform" && pwd)
mkdir -p "$state_dir"
export TF_DATA_DIR="$state_dir/data" TF_IN_AUTOMATION=1 TF_INPUT=0
args=(-var "run_id=$run_id" -var "out_dir=$state_dir/out" -state="$state_dir/terraform.tfstate")
cd "$here"
case "$action" in
  up)
    terraform version
    terraform init -no-color
    terraform apply -no-color -auto-approve "${args[@]}"
    test -s "$state_dir/out/environment.json"
    ;;
  down)
    terraform destroy -no-color -auto-approve "${args[@]}"
    if [ -e "$state_dir/out/environment.json" ]; then echo "environment marker still exists after destroy" >&2; exit 1; fi
    ;;
  *) echo "usage: $0 up|down <run-id> <state-dir>" >&2; exit 2 ;;
esac
