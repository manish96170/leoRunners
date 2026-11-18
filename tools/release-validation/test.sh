#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$root/../.." && pwd)
command -v ruby >/dev/null 2>&1 || { echo 'Ruby is required for report checks' >&2; exit 2; }
sh -n "$root/validate.sh"
bash -n "$root/validate.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-release-validation-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
report="$tmp/release.json"
set +e
"$root/validate.sh" --report "$report" --format json >"$tmp/output" 2>"$tmp/errors"
status=$?
set -e
[ "$status" -eq 0 ] || [ "$status" -eq 2 ] || { cat "$tmp/output" "$tmp/errors" >&2; exit 1; }
test -s "$report"
grep -F 'release validation summary' "$tmp/output" >/dev/null
! grep -E 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]+|BEGIN .*PRIVATE KEY|fixture-secret-token' "$report" >/dev/null
ruby -rjson - "$repo_root/validation/release-report.schema.v1.json" "$report" <<'RUBY'
schema = JSON.parse(File.read(ARGV[0]))
data = JSON.parse(File.read(ARGV[1]))
raise "wrong apiVersion" unless data["apiVersion"] == "release-validation.leorunners.io/v1"
raise "wrong kind" unless data["kind"] == "ReleaseValidationReport"
raise "report is not read-only" unless data.dig("spec", "read_only") == true
raise "live provisioning was recorded" unless data.dig("spec", "live_provisioning_invoked") == false
raise "missing checks" unless data.dig("spec", "checks").is_a?(Array) && data["spec"]["checks"].length >= 10
raise "invalid check status" unless data["spec"]["checks"].all? { |c| %w[PASS WARN FAIL].include?(c["status"]) }
raise "schema id missing" unless schema["$id"].to_s.include?("release-validation")
puts "release report fixture validation passed"
RUBY
printf '%s\n' 'release validation tests passed'
