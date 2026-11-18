#!/bin/sh
set -eu

# This harness is deliberately offline: the environment below cannot select
# AWS, GCP, GitHub JIT, or an AI provider.
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/leo-runner-smoke.XXXXXX")
controller_pid=""
port="${SMOKE_PORT:-$((18080 + ($$ % 1000)))}"
base_url="http://127.0.0.1:${port}"
secret="leo-smoke-secret"
payload="$repo_root/tools/smoke/fixtures/workflow_job_queued.json"
binary="$work_dir/runner-controller"
log_file="$work_dir/controller.log"

cleanup() {
    status=$?
    if [ -n "$controller_pid" ] && kill -0 "$controller_pid" 2>/dev/null; then
        kill -TERM "$controller_pid" 2>/dev/null || true
        i=0
        while kill -0 "$controller_pid" 2>/dev/null && [ "$i" -lt 50 ]; do
            i=$((i + 1))
            sleep 0.1
        done
        if kill -0 "$controller_pid" 2>/dev/null; then
            kill -KILL "$controller_pid" 2>/dev/null || true
        fi
    fi
    if [ -n "$controller_pid" ]; then
        wait "$controller_pid" 2>/dev/null || true
    fi
    rm -rf "$work_dir"
    exit "$status"
}
trap cleanup EXIT INT TERM HUP

require_command() {
    command -v "$1" >/dev/null 2>&1 || {
        printf 'missing required command: %s\n' "$1" >&2
        exit 2
    }
}

require_command go
require_command curl
require_command openssl

printf '%s\n' 'building controller'
(cd "$repo_root/controller" && go build -o "$binary" ./cmd/runner-controller)

# Keep only local-controller settings. In particular, do not inherit cloud,
# GitHub, model, proxy, or credential variables from the caller.
env -i \
    PATH="${PATH}" \
    HOME="${HOME:-$work_dir}" \
    LISTEN_ADDR="127.0.0.1:${port}" \
    GITHUB_WEBHOOK_SECRET="$secret" \
    STATE_PATH="$work_dir/state.json" \
    RECONCILE_INTERVAL="1h" \
    "$binary" >"$log_file" 2>&1 &
controller_pid=$!

wait_for_health() {
    i=0
    while [ "$i" -lt 80 ]; do
        if curl -fsS "$base_url/healthz" >/dev/null 2>&1; then
            return 0
        fi
        if ! kill -0 "$controller_pid" 2>/dev/null; then
            printf '%s\n' 'controller exited before becoming healthy' >&2
            sed -n '1,120p' "$log_file" >&2 || true
            return 1
        fi
        i=$((i + 1))
        sleep 0.1
    done
    printf '%s\n' 'controller did not become healthy' >&2
    sed -n '1,120p' "$log_file" >&2 || true
    return 1
}

status_code() {
    curl -sS -o /dev/null -w '%{http_code}' "$@"
}

assert_status() {
    expected=$1
    shift
    actual=$(status_code "$@")
    [ "$actual" = "$expected" ] || {
        printf 'expected HTTP %s, got %s\n' "$expected" "$actual" >&2
        exit 1
    }
}

wait_for_health
assert_status 200 "$base_url/healthz"
assert_status 401 -X POST "$base_url/webhooks/github"

bad_signature=$(printf '%s' 'not-json' | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print $NF}')
assert_status 400 \
    -X POST \
    -H 'X-Hub-Signature-256: sha256='"$bad_signature" \
    -H 'X-GitHub-Event: workflow_job' \
    --data-binary 'not-json' \
    "$base_url/webhooks/github"

signature=$(openssl dgst -sha256 -hmac "$secret" "$payload" | awk '{print $NF}')
assert_status 202 \
    -X POST \
    -H 'Content-Type: application/json' \
    -H 'X-GitHub-Event: workflow_job' \
    -H 'X-GitHub-Delivery: smoke-queued-001' \
    -H 'X-Hub-Signature-256: sha256='"$signature" \
    --data-binary "@$payload" \
    "$base_url/webhooks/github"

printf '%s\n' 'offline smoke checks passed (health, signature, normalization, acceptance, cleanup)'
