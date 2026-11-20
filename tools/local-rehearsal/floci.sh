#!/usr/bin/env bash
# Starts or stops the floci AWS emulator, bound to this machine only.
#   floci.sh up | down | status
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need docker

case "${1:-}" in
  up)
    if docker ps --format '{{.Names}}' | grep -qx "$FLOCI_CONTAINER"; then
      log "floci is already running"
    else
      docker rm -f "$FLOCI_CONTAINER" >/dev/null 2>&1 || true
      # The Docker socket lets floci start instance containers. That is
      # root-equivalent inside the Docker VM, which is acceptable for a local
      # emulator but is why the port is bound to 127.0.0.1 only.
      docker run -d --name "$FLOCI_CONTAINER" \
        -p "127.0.0.1:${FLOCI_PORT}:4566" \
        -v /var/run/docker.sock:/var/run/docker.sock -u root \
        -e FLOCI_DEFAULT_REGION=us-east-1 \
        "$FLOCI_IMAGE" >/dev/null
    fi
    for _ in $(seq 1 30); do
      curl -fsS -m 2 -o /dev/null "$AWS_ENDPOINT_URL" 2>/dev/null && break
      sleep 1
    done
    require_floci
    log "floci is up at $AWS_ENDPOINT_URL (account 000000000000)"
    ;;
  down)
    docker rm -f "$FLOCI_CONTAINER" >/dev/null 2>&1 || true
    ids=$(docker ps -aq --filter 'name=floci-aws-ec2-' || true)
    [ -z "$ids" ] || docker rm -f $ids >/dev/null 2>&1 || true
    log "floci stopped and its instance containers removed"
    ;;
  status)
    docker ps --filter "name=$FLOCI_CONTAINER" --format '{{.Names}}  {{.Status}}  {{.Ports}}'
    ;;
  *) die "usage: floci.sh up | down | status" ;;
esac
