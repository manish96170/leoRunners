#!/usr/bin/env bash
set -Eeuo pipefail

if [ "${1:-}" != "--allow-live" ] || [ "${2:-}" != "--confirm" ] || [ "${3:-}" != "I_UNDERSTAND_EPHEMERAL_RESOURCES" ]; then
  echo 'blocked: live AWS validation requires --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES' >&2
  exit 2
fi
echo 'blocked: reviewed scope, launch inputs, and cleanup procedure must be supplied before live execution' >&2
exit 2
