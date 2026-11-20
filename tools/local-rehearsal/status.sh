#!/usr/bin/env bash
# Shows what the rehearsal currently has running.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need aws; need docker
require_floci
echo "== controller"; if [ -f "$REHEARSAL_HOME/controller.pid" ] && kill -0 "$(cat "$REHEARSAL_HOME/controller.pid")" 2>/dev/null; then echo "running (pid $(cat "$REHEARSAL_HOME/controller.pid"))"; else echo "not running"; fi
echo "== tunnel"; cat "$REHEARSAL_HOME/tunnel.url" 2>/dev/null || echo "none"
echo "== runner instances (floci)"
aws ec2 describe-instances --query 'Reservations[].Instances[].{id:InstanceId,state:State.Name,job:Tags[?Key==`leo-runners:job-id`]|[0].Value,repo:Tags[?Key==`leo-runners:repository`]|[0].Value}' --output table
echo "== jobs in state"
aws dynamodb scan --table-name "$TABLE_NAME" --filter-expression 'begins_with(pk, :p)' --expression-attribute-values '{":p":{"S":"job#"}}' --query 'Items[].{job:pk.S,state:state.S}' --output table 2>/dev/null || true
echo "== last controller log lines"; tail -n 8 "$REHEARSAL_HOME/logs/controller.log" 2>/dev/null || true
