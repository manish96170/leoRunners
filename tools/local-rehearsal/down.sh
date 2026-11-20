#!/usr/bin/env bash
# Stops everything the rehearsal started. With --purge also deletes the local
# working directory (state, logs, built controller) and the floci container.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
"$(dirname "$0")/stop-controller.sh" || true
"$(dirname "$0")/tunnel.sh" stop || true
if [ "${1:-}" = "--purge" ]; then
  "$(dirname "$0")/floci.sh" down || true
  rm -rf "$REHEARSAL_HOME"
  log "purged $REHEARSAL_HOME"
else
  log "controller and tunnel stopped; floci keeps running (floci.sh down to stop it)"
fi
