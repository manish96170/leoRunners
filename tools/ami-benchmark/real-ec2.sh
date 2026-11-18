#!/usr/bin/env bash
set -Eeuo pipefail

cat >&2 <<'NOTICE'
The real EC2 AMI startup measurement is an operator-run procedure.
This script is a guard and checklist only; it never invokes AWS or Packer.
Use the documented runbook and an approved isolated account before executing
equivalent commands manually.
NOTICE

confirm=${1:-}
if [ "$confirm" != "I_UNDERSTAND_EPHEMERAL_EC2" ]; then
  printf '%s\n' 'refusing: pass I_UNDERSTAND_EPHEMERAL_EC2 after approval' >&2
  exit 2
fi
printf '%s\n' 'guard passed; no EC2 command was executed by this script'
printf '%s\n' 'required checkpoints: launch_requested instance_running bootstrap_started bootstrap_completed runner_registered runner_ready job_started'
printf '%s\n' 'capture only relative durations and link the approved AMI digest/profile in the redacted report'
