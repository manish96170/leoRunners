#!/usr/bin/env bash
# Offline checks for the rehearsal tooling. Needs no Docker, floci, AWS or network.
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$here"

fail() { printf 'test: FAIL: %s\n' "$*" >&2; exit 1; }

for script in *.sh; do bash -n "$script" || fail "$script has a syntax error"; done
python3 -m py_compile webhook-gate.py && rm -rf __pycache__

# The scripts must never inherit real AWS settings: a profile, credentials or an
# endpoint from the caller's shell must not survive sourcing lib.sh.
leak=$(AWS_PROFILE=some-real-profile AWS_SESSION_TOKEN=tok AWS_ENDPOINT_URL=https://example.invalid \
  AWS_ACCESS_KEY_ID=AKIAREALKEY AWS_CONFIG_FILE=/etc/real-config LEO_REHEARSAL_HOME="$(mktemp -d)" \
  bash -c '. ./lib.sh; printf "%s|%s|%s|%s|%s|%s" "${AWS_PROFILE:-}" "${AWS_SESSION_TOKEN:-}" "$AWS_ENDPOINT_URL" "$AWS_ACCESS_KEY_ID" "$AWS_CONFIG_FILE" "$AWS_SHARED_CREDENTIALS_FILE"')
[ "$leak" = "||http://127.0.0.1:4566|000000000000|/dev/null|/dev/null" ] || fail "lib.sh let caller AWS settings through: $leak"

# webhook-gate.py must forward only POST /webhooks/github.
port_up=$((20000 + $$ % 20000)); port_gate=$((port_up + 1))
tmp=$(mktemp -d)
cleanup() {
  for pid in "${up_pid:-}" "${gate_pid:-}"; do
    if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  done
  rm -rf "$tmp"
}
trap cleanup EXIT
python3 - "$port_up" "$tmp/upstream.log" > /dev/null 2>&1 <<'PY' &
import http.server, sys
port, log = int(sys.argv[1]), sys.argv[2]
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        open(log, "a").write("%s sig=%s event=%s bytes=%d\n" % (self.path, self.headers.get("X-Hub-Signature-256"), self.headers.get("X-GitHub-Event"), len(body)))
        self.send_response(202); self.send_header("Content-Length", "0"); self.end_headers()
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", port), H).serve_forever()
PY
up_pid=$!
disown "$up_pid"
python3 webhook-gate.py "$port_gate" "$port_up" > /dev/null 2>&1 &
gate_pid=$!
disown "$gate_pid"
for _ in $(seq 1 30); do curl -s -o /dev/null "http://127.0.0.1:$port_gate/x" 2>/dev/null && break; sleep 0.2; done

code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
gate="http://127.0.0.1:$port_gate"
[ "$(code -X POST "$gate/webhooks/github" -H 'X-GitHub-Event: workflow_job' -H 'X-Hub-Signature-256: sha256=abc' --data '{"a":1}')" = 202 ] || fail "webhook was not forwarded"
grep -q '^/webhooks/github sig=sha256=abc event=workflow_job bytes=7$' "$tmp/upstream.log" || fail "headers or body were not forwarded intact"
for path in /healthz /metrics / /webhooks/github/x; do
  [ "$(code "$gate$path")" = 404 ] || fail "GET $path was not blocked"
done
[ "$(code -X POST "$gate/metrics" --data x)" = 404 ] || fail "POST /metrics was not blocked"
[ "$(code -X POST "$gate/webhooks/github" --data-binary @<(head -c 3000000 /dev/zero))" = 413 ] || fail "oversized body was not rejected"
[ "$(wc -l < "$tmp/upstream.log" | tr -d ' ')" = 1 ] || fail "blocked requests reached the controller"

# No machine-specific paths, real account IDs, or credential-shaped values.
if grep -rInE '(/Users/[A-Za-z]|/home/[a-z]|AKIA[0-9A-Z]{16}|BEGIN [A-Z ]*PRIVATE KEY|gh[pousr]_[A-Za-z0-9]{30,})' --exclude=test.sh .; then fail "environment-specific or secret-like value found"; fi

printf 'test: PASS\n'
