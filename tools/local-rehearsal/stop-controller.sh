#!/usr/bin/env bash
# Stops the controller started by run-controller.sh and waits for it to drain.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
pid_file="$REHEARSAL_HOME/controller.pid"
[ -f "$pid_file" ] || { log "no controller pid file"; exit 0; }
pid=$(cat "$pid_file")
if kill -0 "$pid" 2>/dev/null; then
  kill -TERM "$pid"
  for _ in $(seq 1 40); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
  kill -0 "$pid" 2>/dev/null && { kill -KILL "$pid"; log "controller did not stop in 40s; killed"; }
fi
rm -f "$pid_file"
log "controller stopped"
