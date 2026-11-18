#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
validator="$root/tools/intelligence-validation/validate.sh"

"$validator"

bad=$(mktemp "${TMPDIR:-/tmp}/intelligence-evaluation-bad.XXXXXX.json")
trap 'rm -f "$bad"' EXIT HUP INT TERM
sed 's/"ai_enabled": false/"ai_enabled": true/' "$root/intelligence/evaluation-synthetic.v1.json" > "$bad"
if "$validator" "$bad" >/dev/null 2>&1; then
  echo "AI-enabled fixture unexpectedly passed" >&2
  exit 1
fi

if ! (cd "$root/controller" && go test ./internal/intelligence/...) >/dev/null; then
  echo "intelligence Go tests failed" >&2
  exit 1
fi

echo "intelligence evaluation tests passed"
