#!/usr/bin/env bash
# Checks that expired runners are removed, against the local emulator. Two cases:
#
#   1. state-driven: a runner's expiry passes while the controller is down; after
#      the controller restarts, the reconciler terminates it and keeps its record.
#   2. provider-side backstop: instances the controller has no record of are
#      removed only when they are ours AND past their own expiry marker.
#
# Takes about two minutes. Creates and removes its own instances. Needs setup.sh
# to have run. Stops any running controller and leaves none running.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need aws; need python3
load_state
require_floci
here=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

instance_state() { aws ec2 describe-instances --instance-ids "$1" --query 'Reservations[0].Instances[0].State.Name' --output text; }
instance_for_job() { aws ec2 describe-instances --filters "Name=tag:leo-runners:job-id,Values=$1" --query 'Reservations[].Instances[0].InstanceId' --output text; }
iso_offset() { python3 -c 'import sys,datetime as d; print((d.datetime.now(d.timezone.utc)+d.timedelta(hours=float(sys.argv[1]))).strftime("%Y-%m-%dT%H:%M:%SZ"))' "$1"; }
epoch_offset() { python3 -c 'import sys,time; print(int(time.time()+float(sys.argv[1])*3600))' "$1"; }
wait_for() { # description timeout-seconds command...
  local what=$1 limit=$2; shift 2
  local waited=0
  until "$@"; do
    [ "$waited" -lt "$limit" ] || die "timed out waiting for $what"
    sleep 5; waited=$((waited + 5))
  done
}
is_terminated() { case "$(instance_state "$1")" in terminated | shutting-down) return 0 ;; *) return 1 ;; esac; }
is_running() { [ "$(instance_state "$1")" = running ]; }

created=()
cleanup() {
  "$here/stop-controller.sh" > /dev/null 2>&1 || true
  for id in ${created[@]+"${created[@]}"}; do aws ec2 terminate-instances --instance-ids "$id" > /dev/null 2>&1 || true; done
}
trap cleanup EXIT
"$here/stop-controller.sh" > /dev/null 2>&1 || true

log "case 1: a runner expires while the controller is down"
job=$((900000 + RANDOM))
"$here/run-controller.sh" > /dev/null
"$here/send-webhook.sh" queued "$job" "$job" > /dev/null
wait_for "the runner to launch" 120 test -n "$(instance_for_job "$job")"
runner=$(instance_for_job "$job"); created+=("$runner")
wait_for "the runner to be running" 120 is_running "$runner"
"$here/stop-controller.sh" > /dev/null

runner_id=$(aws dynamodb scan --table-name "$TABLE_NAME" --filter-expression 'begins_with(pk, :r) AND provider_instance_id = :i' \
  --expression-attribute-values "{\":r\":{\"S\":\"runner#\"},\":i\":{\"S\":\"$runner\"}}" --query 'Items[0].id.S' --output text)
[ -n "$runner_id" ] && [ "$runner_id" != None ] || die "the runner record was not written"
expired=$(epoch_offset -1)
sort_key=$(printf '%020d#%s' "$expired" "$runner_id")
created_at=$(python3 -c 'import datetime as d; print((d.datetime.now(d.timezone.utc)-d.timedelta(hours=3)).strftime("%Y-%m-%dT%H:%M:%S.000000Z"))')
aws dynamodb update-item --table-name "$TABLE_NAME" \
  --key "{\"pk\":{\"S\":\"runner#$runner_id\"},\"sk\":{\"S\":\"META\"}}" \
  --update-expression 'SET expires_at = :e, gsi2sk = :k, created_at = :c' \
  --expression-attribute-values "{\":e\":{\"N\":\"$expired\"},\":k\":{\"S\":\"$sort_key\"},\":c\":{\"S\":\"$created_at\"}}" > /dev/null
"$here/run-controller.sh" > /dev/null
wait_for "the expired runner to be terminated" 90 is_terminated "$runner"
record=$(aws dynamodb get-item --table-name "$TABLE_NAME" --key "{\"pk\":{\"S\":\"runner#$runner_id\"},\"sk\":{\"S\":\"META\"}}" --query 'Item.state.S' --output text)
[ "$record" = terminated ] || die "case 1 FAILED: runner record is '$record', want terminated"
log "case 1 passed: expired runner terminated after restart; its record survived and is marked terminated"
"$here/stop-controller.sh" > /dev/null

log "case 2: instances with no record"
past=$(iso_offset -2); future=$(iso_offset 2)
own='{Key=leo-runners:managed-by,Value=leo-runners}'
launch() {
  aws ec2 run-instances --launch-template "LaunchTemplateId=$LAUNCH_TEMPLATE_ID" \
    --tag-specifications "ResourceType=instance,Tags=[$1]" --query 'Instances[0].InstanceId' --output text
}
orphan=$(launch "$own,{Key=leo-runners:runner-id,Value=runner-check-expired},{Key=leo-runners:expires-at,Value=$past}"); created+=("$orphan")
fresh=$(launch "$own,{Key=leo-runners:runner-id,Value=runner-check-fresh},{Key=leo-runners:expires-at,Value=$future}"); created+=("$fresh")
nomarker=$(launch "$own,{Key=leo-runners:runner-id,Value=runner-check-nomarker}"); created+=("$nomarker")
foreign=$(launch "{Key=Name,Value=not-ours},{Key=leo-runners:expires-at,Value=$past}"); created+=("$foreign")
"$here/run-controller.sh" > /dev/null
wait_for "the expired, unrecorded instance to be terminated" 90 is_terminated "$orphan"
for pair in "kept-while-valid:$fresh" "kept-without-an-expiry-marker:$nomarker" "kept-when-not-ours:$foreign"; do
  [ "$(instance_state "${pair#*:}")" = running ] || die "case 2 FAILED: ${pair%%:*} instance was removed"
done
log "case 2 passed: the expired owned instance was removed; valid, unmarked and foreign instances were left alone"
log "reaper checks passed"
