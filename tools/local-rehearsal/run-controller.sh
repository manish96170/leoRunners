#!/usr/bin/env bash
# Runs the controller on this machine against floci.
#   run-controller.sh            webhook intake only (JIT off): fake runners, no GitHub credentials
#   run-controller.sh --jit      real runners: authenticates as your GitHub App
#
# JIT mode reads non-secret settings from $REHEARSAL_HOME/github.env (see README)
# and the App private key from floci's Secrets Manager, the way ECS would inject
# it. The key is held in this process's environment only; it is never printed.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need go; need aws
load_state
require_floci
resolve_arch

mode=basic
[ "${1:-}" = "--jit" ] && mode=jit

bin="$REHEARSAL_HOME/bin"
mkdir -p "$bin" "$REHEARSAL_HOME/logs"
(cd "$REPO_ROOT/controller" && go build -o "$bin/runner-controller" ./cmd/runner-controller)
log "built controller from $(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"

# The environment is built from scratch so nothing from your shell (company AWS
# or GitHub settings, tokens, proxies) can leak into the controller.
env_args=(
  PATH="$PATH" HOME="$REHEARSAL_HOME"
  AWS_ACCESS_KEY_ID=000000000000 AWS_SECRET_ACCESS_KEY=test AWS_REGION=us-east-1
  AWS_ENDPOINT_URL="$AWS_ENDPOINT_URL" AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null
  LISTEN_ADDR="127.0.0.1:${CONTROLLER_PORT}"
  GITHUB_WEBHOOK_SECRET="$WEBHOOK_SECRET"
  DYNAMODB_TABLE_NAME="$TABLE_NAME"
  CAPACITY_PROVIDER=aws CAPACITY_POOL_PROVIDER=aws
  AWS_LAUNCH_TEMPLATE_ID="$LAUNCH_TEMPLATE_ID" AWS_LAUNCH_TEMPLATE_VERSION='$Latest'
  RECONCILE_INTERVAL=15s
)

if [ "$mode" = jit ]; then
  cfg="$REHEARSAL_HOME/github.env"
  [ -f "$cfg" ] || die "missing $cfg; copy github.env.example there and fill it in"
  # shellcheck disable=SC1090
  . "$cfg"
  : "${GITHUB_APP_ID:?set in github.env}" "${GITHUB_APP_INSTALLATION_ID:?set in github.env}" "${GITHUB_RUNNER_GROUP_ID:?set in github.env}"
  [ -n "${GITHUB_ALLOWED_ORGANIZATIONS:-}${GITHUB_ALLOWED_REPOSITORIES:-}" ] || die "set GITHUB_ALLOWED_ORGANIZATIONS or GITHUB_ALLOWED_REPOSITORIES in github.env"
  labels=${LEO_RUNNER_LABELS:-self-hosted,linux,$RUNNER_ARCH_LABEL}
  key=$(aws secretsmanager get-secret-value --secret-id "$SECRET_NAME" --query SecretString --output text 2>/dev/null || true)
  [ -n "$key" ] && [ "$key" != None ] || die "no GitHub App private key in floci; run store-app-key.sh first"
  env_args+=(
    GITHUB_JIT_ENABLED=true
    GITHUB_APP_ID="$GITHUB_APP_ID" GITHUB_APP_INSTALLATION_ID="$GITHUB_APP_INSTALLATION_ID"
    GITHUB_APP_PRIVATE_KEY="$key"
    GITHUB_RUNNER_GROUP_ID="$GITHUB_RUNNER_GROUP_ID"
    GITHUB_ALLOWED_ORGANIZATIONS="${GITHUB_ALLOWED_ORGANIZATIONS:-}"
    GITHUB_ALLOWED_REPOSITORIES="${GITHUB_ALLOWED_REPOSITORIES:-}"
    GITHUB_ALLOWED_LABELS="$labels" CAPACITY_LABELS="$labels"
    GITHUB_ALLOW_FORKS=false
    RUNNER_USER_DATA_FILE="$REPO_ROOT/bootstrap/runner/user-data.sh"
  )
  [ -n "${GITHUB_APP_TOKEN_PERMISSIONS:-}" ] && env_args+=(GITHUB_APP_TOKEN_PERMISSIONS="$GITHUB_APP_TOKEN_PERMISSIONS")
  unset key
  log "JIT mode: runners register as $labels"
else
  log "basic mode: webhook intake with fake GitHub (no runner is registered)"
fi

pid_file="$REHEARSAL_HOME/controller.pid"
if [ -f "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
  die "a controller is already running (pid $(cat "$pid_file")); stop it with stop-controller.sh"
fi
logfile="$REHEARSAL_HOME/logs/controller.log"
: > "$logfile"
nohup env -i "${env_args[@]}" "$bin/runner-controller" >> "$logfile" 2>&1 &
echo $! > "$pid_file"
unset env_args
for _ in $(seq 1 20); do
  curl -fsS -m 1 "http://127.0.0.1:${CONTROLLER_PORT}/healthz" >/dev/null 2>&1 && { log "controller is healthy on 127.0.0.1:${CONTROLLER_PORT} (pid $(cat "$pid_file")); logs: $logfile"; exit 0; }
  kill -0 "$(cat "$pid_file")" 2>/dev/null || { cat "$logfile" >&2; die "controller exited at startup"; }
  sleep 0.5
done
die "controller did not become healthy"
