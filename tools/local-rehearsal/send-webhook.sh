#!/usr/bin/env bash
# Sends a signed workflow_job webhook to the local controller, like GitHub would.
#   send-webhook.sh <queued|in_progress|completed> <job-id> [run-id] [owner/repo] [labels,csv]
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need curl; need openssl
action=${1:?usage: send-webhook.sh <queued|in_progress|completed> <job-id> [run-id] [owner/repo] [labels]}
job=${2:?job id}
run=${3:-9000}
repo=${4:-example-org/example-repo}
labels=${5:-self-hosted,linux,x64}
url=${WEBHOOK_URL:-http://127.0.0.1:${CONTROLLER_PORT}/webhooks/github}
conclusion=null; status=queued
[ "$action" = in_progress ] && status=in_progress
[ "$action" = completed ] && { status=completed; conclusion='"success"'; }
json_labels=$(printf '%s' "$labels" | awk -F, '{for(i=1;i<=NF;i++){printf "%s\"%s\"", (i>1?",":""), $i}}')
name=${repo#*/}
body=$(printf '{"action":"%s","workflow_job":{"id":%s,"run_id":%s,"run_attempt":1,"name":"rehearsal","workflow_name":"Rehearsal","status":"%s","conclusion":%s,"labels":[%s],"repository":{"id":1,"full_name":"%s","name":"%s"}},"repository":{"id":1,"full_name":"%s","name":"%s"}}' \
  "$action" "$job" "$run" "$status" "$conclusion" "$json_labels" "$repo" "$name" "$repo" "$name")
sig=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | awk '{print $NF}')
curl -sS -o /dev/null -w "HTTP %{http_code} in %{time_total}s\n" -X POST "$url" \
  -H "Content-Type: application/json" -H "X-GitHub-Event: workflow_job" \
  -H "X-GitHub-Delivery: $(uuidgen)" -H "X-Hub-Signature-256: sha256=$sig" --data "$body"
