#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

command -v go >/dev/null 2>&1 || {
    printf '%s\n' 'security validation tests require Go' >&2
    exit 2
}

gofmt -w main.go main_test.go
go test ./...
go test -race ./...
go vet ./...

report=$(mktemp "${TMPDIR:-/tmp}/leo-security-validation.XXXXXX")
trap 'rm -f "$report"' EXIT HUP INT TERM
if go run . --root fixtures/unsafe --format json --report "$report" >/dev/null 2>&1; then
    printf '%s\n' 'unsafe fixture unexpectedly passed' >&2
    exit 1
else
    status=$?
    [ "$status" -eq 1 ] || { printf 'unexpected unsafe fixture exit: %s\n' "$status" >&2; exit 1; }
fi
grep -F '"status": "fail"' "$report" >/dev/null
! grep -F 'super-secret-value' "$report" >/dev/null
go run . --root fixtures/clean --format json >/dev/null

printf '%s\n' 'security validation tests passed'
