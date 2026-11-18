#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
validator="$script_dir/validate.sh"
cloud_fixture_bin="$repo_root/tools/cloud-validation/fixtures/bin"
tmp=$(mktemp -d "/tmp/leo-production-preflight-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

sh -n "$validator"
sh -n "$script_dir/test.sh"
grep -F '"$cloud_validator" --provider "$provider" --scope-file "$scope_path" --report "$cloud_report_path"' "$validator" >/dev/null
! grep -E 'cloud_validator.*--(allow-live|live-action|confirm|evidence-report)' "$validator" >/dev/null

export PATH="$cloud_fixture_bin:$PATH"
export AWS_REGION=us-east-1 GCP_PROJECT=fixture-project GCP_ZONE=us-central1-a
export GITHUB_TOKEN=fixture-secret-token GITHUB_REPOSITORY=octo-org/fixture-repo
export GITHUB_RUNNER_GROUP_ID=1 GITHUB_RUNNER_LABELS=self-hosted,linux,x64
export CLOUD_VALIDATION_MOCK_LOG="$tmp/mock.log"
: >"$CLOUD_VALIDATION_MOCK_LOG"

scope="$tmp/scope.env"
printf '%s\n' 'aws_account_id=000000000000' 'aws_region=us-east-1' \
    'gcp_project=fixture-project' 'gcp_zone=us-central1-a' \
    'github_repository=octo-org/fixture-repo' >"$scope"
request="$tmp/request.env"
printf '%s\n' 'request_version=production-preflight.v1' 'mode=preflight' \
    'provider=all' 'live_action=none' 'allow_live=false' \
    'confirmation=none' 'owner=release-review' >"$request"

report="$tmp/pass.json"
output=$("$validator" --request "$request" --scope-file "$scope" --report "$report" 2>&1)
printf '%s\n' "$output" | grep -F 'PASS production-preflight' >/dev/null
test -s "$report"
grep -F '"status": "PASS"' "$report" >/dev/null
grep -F '"read_only": true' "$report" >/dev/null
grep -F '"live_flags_forwarded": false' "$report" >/dev/null
grep -F '"resources_created": false' "$report" >/dev/null
grep -F '"resources_terminated": false' "$report" >/dev/null
grep -F '"identity": "withheld"' "$report" >/dev/null
grep -F '"mode": "preflight"' "$report" >/dev/null
! grep -F 'fixture-secret-token' "$report" >/dev/null
! grep -F 'aws_account_id' "$report" >/dev/null
! grep -E -- '--allow-live|--live-action|--confirm|--evidence-report|run-instances|instances create|generate-jitconfig' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

blocked_request="$tmp/blocked-request.env"
sed 's/allow_live=false/allow_live=true/' "$request" >"$blocked_request"
blocked_report="$tmp/blocked.json"
if "$validator" --request "$blocked_request" --scope-file "$scope" --report "$blocked_report" >"$tmp/blocked.out" 2>&1; then
    printf '%s\n' 'unsafe request unexpectedly passed' >&2
    exit 1
fi
grep -F 'BLOCKED production-preflight' "$tmp/blocked.out" >/dev/null
grep -F '"status": "BLOCKED"' "$blocked_report" >/dev/null
grep -F '"cloud_validation": {' "$blocked_report" >/dev/null
! grep -F 'fixture-secret-token' "$blocked_report" >/dev/null

bad_scope="$tmp/bad-scope.env"
printf '%s\n' 'aws_account_id=999999999999' >"$bad_scope"
cloud_blocked_report="$tmp/cloud-blocked.json"
if "$validator" --request "$request" --scope-file "$bad_scope" --report "$cloud_blocked_report" >"$tmp/cloud-blocked.out" 2>&1; then
    printf '%s\n' 'cloud preflight failure unexpectedly passed' >&2
    exit 1
fi
grep -F 'BLOCKED production-preflight' "$tmp/cloud-blocked.out" >/dev/null
grep -F '"cloud_validation": {' "$cloud_blocked_report" >/dev/null
grep -F '"status": "blocked"' "$cloud_blocked_report" >/dev/null
! grep -F '999999999999' "$cloud_blocked_report" >/dev/null

missing_scope="$tmp/missing-scope.env"
if "$validator" --request "$request" --scope-file "$missing_scope" >/dev/null 2>&1; then
    printf '%s\n' 'missing scope unexpectedly passed' >&2
    exit 1
fi

printf '%s\n' 'production preflight tests passed'
