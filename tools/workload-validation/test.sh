#!/bin/sh
set -eu

go test ./...
go run . --mode fake --format json --output "${TMPDIR:-/tmp}/leo-workload-evidence.json" >/dev/null
echo "workload validation tests passed"
