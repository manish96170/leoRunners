#!/bin/sh
set -eu

test "$(basename "$0")" = "test.sh"
go test ./...
go run . --manifest fixtures/minimal.json --root . > /tmp/workload-preflight-output.json
grep -q '"checks"' /tmp/workload-preflight-output.json
grep -q '"summary"' /tmp/workload-preflight-output.json
printf '%s\n' 'workload-preflight tests passed'
