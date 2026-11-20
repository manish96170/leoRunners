#!/usr/bin/env bash
# Opens a free Cloudflare quick tunnel so GitHub can deliver webhooks to this
# machine. No Cloudflare account is needed. The tunnel points at webhook-gate.py,
# which forwards only POST /webhooks/github; /healthz and /metrics stay private.
#   tunnel.sh        start (prints the URL to put in the GitHub App webhook)
#   tunnel.sh stop
# The URL changes every time the tunnel restarts; update the App's webhook URL
# when it does. Anyone with the URL can send requests to the controller, which
# rejects anything without a valid webhook signature.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
pid_file="$REHEARSAL_HOME/tunnel.pid"
gate_pid_file="$REHEARSAL_HOME/gate.pid"
GATE_PORT=${LEO_GATE_PORT:-$((CONTROLLER_PORT + 1))}
url_file="$REHEARSAL_HOME/tunnel.url"
logfile="$REHEARSAL_HOME/logs/tunnel.log"
mkdir -p "$REHEARSAL_HOME/logs"

if [ "${1:-}" = stop ]; then
  [ -f "$pid_file" ] && kill "$(cat "$pid_file")" 2>/dev/null || true
  [ -f "$gate_pid_file" ] && kill "$(cat "$gate_pid_file")" 2>/dev/null || true
  rm -f "$pid_file" "$gate_pid_file" "$url_file"; log "tunnel stopped"; exit 0
fi

need cloudflared; need python3
if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
  log "tunnel already running"; cat "$url_file" 2>/dev/null || true; exit 0
fi
: > "$logfile"
nohup python3 "$(dirname "$0")/webhook-gate.py" "$GATE_PORT" "$CONTROLLER_PORT" >> "$logfile" 2>&1 &
echo $! > "$gate_pid_file"
nohup cloudflared tunnel --no-autoupdate --url "http://127.0.0.1:${GATE_PORT}" >> "$logfile" 2>&1 &
echo $! > "$pid_file"
for _ in $(seq 1 40); do
  url=$(grep -Eo 'https://[a-z0-9-]+\.trycloudflare\.com' "$logfile" | head -1 || true)
  if [ -n "$url" ]; then
    echo "$url" > "$url_file"
    log "tunnel is up"
    printf 'Webhook URL for the GitHub App: %s/webhooks/github\n' "$url"
    exit 0
  fi
  kill -0 "$(cat "$pid_file")" 2>/dev/null || { cat "$logfile" >&2; die "cloudflared exited"; }
  sleep 1
done
die "no tunnel URL after 40s; see $logfile"
