#!/bin/sh
set -eu

test_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
bootstrap=$test_dir/bootstrap.sh
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/runner-bootstrap-test.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT INT TERM HUP

fail() {
    printf 'test: FAIL: %s\n' "$*" >&2
    exit 1
}

assert_file_absent() {
    [ ! -e "$1" ] || fail "sensitive file remains: $1"
}

mkdir "$tmp_dir/runner"
apply_fake_runner() {
    printf '%s\n' '#!/bin/sh' 'set -eu' '[ "$1" = "--jitconfig" ]' '[ -n "$2" ]' 'if [ "${RUNNER_EXIT:-0}" -ne 0 ]; then exit "$RUNNER_EXIT"; fi' 'if [ "${RUNNER_SLEEP:-0}" = 1 ]; then trap '\''printf terminated > "$RUNNER_TERMINATED"; exit 143'\'' TERM INT HUP; printf started > "$RUNNER_STARTED"; while :; do :; done; fi' 'printf "%s\\n" "$2" > "$RUNNER_CAPTURE"' > "$tmp_dir/runner/run.sh"
    chmod 700 "$tmp_dir/runner/run.sh"
}
apply_fake_runner

# This is deliberately opaque: encoded_jit_config is already encoded for the
# runner and must be passed byte-for-byte (apart from transport newlines).
payload='not-a-base64-token+/_=.opaque'
capture=$tmp_dir/capture
config=$tmp_dir/config

RUNNER_CAPTURE=$capture \
RUNNER_DIR=$tmp_dir/runner \
JIT_CONFIG_FILE=$config \
GITHUB_JIT_CONFIG_B64=$payload \
"$bootstrap"

[ "$(cat "$capture")" = "$payload" ] || fail "runner did not receive opaque encoded JIT config"
assert_file_absent "$config"

if RUNNER_DIR=$tmp_dir/runner "$bootstrap" >/dev/null 2>&1; then
    fail "missing configuration was accepted"
fi

source_file=$tmp_dir/source-config
printf '%s\n' "$payload" > "$source_file"
file_capture=$tmp_dir/file-capture
RUNNER_CAPTURE=$file_capture RUNNER_DIR=$tmp_dir/runner \
GITHUB_JIT_CONFIG_FILE=$source_file "$bootstrap"
[ "$(cat "$file_capture")" = "$payload" ] || fail "file input was not passed unchanged"
[ ! -e "$source_file" ] || fail "one-time source file remains"

fd_capture=$tmp_dir/fd-capture
RUNNER_CAPTURE=$fd_capture RUNNER_DIR=$tmp_dir/runner \
    GITHUB_JIT_CONFIG_FD=3 "$bootstrap" 3<<EOF
$payload
EOF
[ "$(cat "$fd_capture")" = "$payload" ] || fail "FD input was not passed unchanged"

terminated=$tmp_dir/terminated
started=$tmp_dir/started
signal_config=$tmp_dir/signal-config
signal_log=$tmp_dir/signal-log
RUNNER_SLEEP=1 \
RUNNER_TERMINATED=$terminated \
RUNNER_STARTED=$started \
RUNNER_DIR=$tmp_dir/runner \
JIT_CONFIG_FILE=$signal_config \
GITHUB_JIT_CONFIG_B64=$payload \
"$bootstrap" >/dev/null 2>"$signal_log" &
bootstrap_pid=$!
attempt=0
while [ ! -f "$started" ] && [ "$attempt" -lt 20 ]; do
    sleep 0.1
    attempt=$((attempt + 1))
done
[ -f "$started" ] || fail "fake runner did not start"
kill -TERM "$bootstrap_pid"
set +e
wait "$bootstrap_pid"
signal_status=$?
set -e
[ "$signal_status" -ne 0 ] || fail "TERM unexpectedly returned success"
[ -f "$terminated" ] || { cat "$tmp_dir/signal-log" >&2 2>/dev/null || true; fail "TERM was not forwarded to runner"; }
assert_file_absent "$signal_config"

failure_log=$tmp_dir/failure-log
set +e
RUNNER_EXIT=23 RUNNER_DIR=$tmp_dir/runner GITHUB_JIT_CONFIG_B64=$payload "$bootstrap" > /dev/null 2>"$failure_log"
failure_status=$?
set -e
[ "$failure_status" -eq 23 ] || fail "runner failure status was not preserved"
grep -q 'runner exited with status 23' "$failure_log" || fail "runner failure was not clearly reported"

# user-data.sh template: static checks. The controller's Go tests render it
# through the real renderer; these keep the file honest when edited by hand.
user_data=$test_dir/user-data.sh
bash -n "$user_data" || fail "user-data.sh is not valid bash"
sh -n "$test_dir/pin-latest-runner.sh" || fail "pin-latest-runner.sh is not valid shell"
placeholders=$(grep -o '{{GITHUB_JIT_CONFIG_B64}}' "$user_data" | wc -l | tr -d ' ')
[ "$placeholders" = 1 ] || fail "user-data.sh must contain the JIT placeholder exactly once (found $placeholders)"
size=$(wc -c < "$user_data" | tr -d ' ')
[ "$size" -lt 16384 ] || fail "user-data.sh is $size bytes; EC2 allows 16384"
grep -Eq '^RUNNER_SHA256_X64="[0-9a-f]{64}"$' "$user_data" || fail "x64 runner checksum is not pinned"
grep -Eq '^RUNNER_SHA256_ARM64="[0-9a-f]{64}"$' "$user_data" || fail "arm64 runner checksum is not pinned"
if grep -Eq 'set -x|xtrace' "$user_data"; then fail "user-data.sh must never enable tracing"; fi
if grep -Eq '\|[[:space:]]*(ba|z)?sh([[:space:]]|$)' "$user_data"; then fail "user-data.sh must not pipe content into a shell"; fi

printf 'test: PASS\n'
