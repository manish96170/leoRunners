#!/bin/sh
set -eu

# The JIT configuration is an opaque encoded_jit_config returned by GitHub.
# It is passed unchanged to run.sh; it is not decoded or re-encoded here.
umask 077

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
runner_dir=${RUNNER_DIR:-$script_dir}
runner_run_script=${RUNNER_RUN_SCRIPT:-$runner_dir/run.sh}
jit_config_file=${JIT_CONFIG_FILE:-}
jit_config_source_file=${GITHUB_JIT_CONFIG_FILE:-}
jit_config_source_fd=${GITHUB_JIT_CONFIG_FD:-}
jit_config_created=0
runner_pid=
terminated=0

log() {
    printf '%s\n' "runner-bootstrap: $*" >&2
}

fail() {
    log "ERROR: $*"
    exit 1
}

cleanup() {
    status=$?
    trap - EXIT INT TERM HUP

    if [ -n "${runner_pid:-}" ]; then
        kill "$runner_pid" 2>/dev/null || true
        wait "$runner_pid" 2>/dev/null || true
    fi
    if [ "${jit_config_created:-0}" -eq 1 ] && [ -n "${jit_config_file:-}" ] && [ -f "$jit_config_file" ]; then
        rm -f "$jit_config_file"
    fi
    exit "$status"
}

forward_signal() {
    signal=$1
    terminated=1
    log "forwarding $signal to runner"
    if [ -n "${runner_pid:-}" ]; then
        kill -"$signal" "$runner_pid" 2>/dev/null || true
    fi
}

trap cleanup EXIT
trap 'forward_signal TERM' TERM
trap 'forward_signal INT' INT
trap 'forward_signal HUP' HUP

source_count=0
[ -n "${GITHUB_JIT_CONFIG_B64:-}" ] && source_count=$((source_count + 1))
[ -n "$jit_config_source_file" ] && source_count=$((source_count + 1))
[ -n "$jit_config_source_fd" ] && source_count=$((source_count + 1))
[ "$source_count" -eq 1 ] || fail "provide exactly one of GITHUB_JIT_CONFIG_B64, GITHUB_JIT_CONFIG_FILE, or GITHUB_JIT_CONFIG_FD"
[ -d "$runner_dir" ] || fail "runner directory does not exist: $runner_dir"
case "$runner_run_script" in
    /*) ;;
    *) runner_run_script=$runner_dir/$runner_run_script ;;
esac
[ -x "$runner_run_script" ] || fail "runner script is not executable: $runner_run_script"

if [ -n "$jit_config_file" ]; then
    case "$jit_config_file" in
        /*) ;;
        *) fail "JIT_CONFIG_FILE must be an absolute path" ;;
    esac
    [ ! -e "$jit_config_file" ] || fail "JIT_CONFIG_FILE already exists"
    (umask 077 && set -C && : > "$jit_config_file") || fail "cannot create JIT_CONFIG_FILE"
    jit_config_created=1
else
    jit_config_file=$(mktemp "${TMPDIR:-/tmp}/runner-jit-config.XXXXXX") || fail "cannot create private JIT config file"
    jit_config_created=1
fi

if [ -n "${GITHUB_JIT_CONFIG_B64:-}" ]; then
    printf '%s' "$GITHUB_JIT_CONFIG_B64" > "$jit_config_file" || fail "cannot write JIT config"
elif [ -n "$jit_config_source_file" ]; then
    case "$jit_config_source_file" in
        /*) ;;
        *) fail "GITHUB_JIT_CONFIG_FILE must be an absolute path" ;;
    esac
    [ -f "$jit_config_source_file" ] || fail "GITHUB_JIT_CONFIG_FILE does not exist"
    cat "$jit_config_source_file" > "$jit_config_file" || fail "cannot read JIT config file"
    rm -f "$jit_config_source_file" || fail "cannot remove one-time JIT config file"
elif [ -n "$jit_config_source_fd" ]; then
    case "$jit_config_source_fd" in
        ''|*[!0-9]*) fail "GITHUB_JIT_CONFIG_FD must be a file descriptor number" ;;
    esac
    eval "cat <&$jit_config_source_fd" > "$jit_config_file" || fail "cannot read JIT config file descriptor"
    eval "exec $jit_config_source_fd<&-" 2>/dev/null || true
fi
chmod 600 "$jit_config_file" || fail "cannot restrict JIT config permissions"

[ -s "$jit_config_file" ] || fail "JIT config is empty"

# JIT mode consumes the encoded value directly and is inherently one-job.
unset GITHUB_JIT_CONFIG_B64
jit_config=$(cat "$jit_config_file")
[ -n "$jit_config" ] || fail "JIT config is empty"

log "starting ephemeral GitHub Actions runner"
cd "$runner_dir"
"$runner_run_script" --jitconfig "$jit_config" &
runner_pid=$!
# The child must receive the value as --jitconfig, but the parent no longer
# needs its shell copy once the process has been started.
unset jit_config
set +e
wait "$runner_pid"
status=$?
set -e
runner_pid=

if [ "$terminated" -eq 1 ]; then
    log "runner stopped after signal"
elif [ "$status" -ne 0 ]; then
    log "runner exited with status $status"
else
    log "ephemeral runner completed"
fi
exit "$status"
