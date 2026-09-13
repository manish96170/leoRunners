#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
validator="$script_dir/validate.sh"
fixture_bin="$script_dir/fixtures/bin"
test_tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-cloud-validation-test.XXXXXX")
trap 'rm -rf "$test_tmp"' EXIT HUP INT TERM

sh -n "$validator"
sh -n "$script_dir/fixtures/bin/aws"
sh -n "$script_dir/fixtures/bin/gcloud"
sh -n "$script_dir/fixtures/bin/curl"

export PATH="$fixture_bin:$PATH"
export AWS_REGION=us-east-1
export GCP_PROJECT=fixture-project
export GCP_ZONE=us-central1-a
export GITHUB_TOKEN=fixture-secret-token
export GITHUB_REPOSITORY=octo-org/fixture-repo
export GITHUB_RUNNER_GROUP_ID=1
export GITHUB_RUNNER_LABELS=self-hosted,linux,x64
export AWS_VALIDATION_LAUNCH_TEMPLATE_ID=lt-fixture
export GCP_VALIDATION_INSTANCE_TEMPLATE=fixture-template
export CLOUD_VALIDATION_MOCK_LOG="$test_tmp/mock.log"
: >"$CLOUD_VALIDATION_MOCK_LOG"

output=$("$validator" --provider all)
printf '%s\n' "$output" | grep -F 'read-only mode' >/dev/null
! grep -E 'run-instances|terminate-instances|instances create|instances delete|generate-jitconfig' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

if GITHUB_TOKEN= GH_TOKEN= "$validator" --provider github >/dev/null 2>&1; then
    printf '%s\n' 'missing-token test unexpectedly passed' >&2
    exit 1
fi

if "$validator" --provider aws --allow-live --live-action aws-ec2 >/dev/null 2>&1; then
    printf '%s\n' 'confirmation guard unexpectedly passed' >&2
    exit 1
fi
! grep -F 'run-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

report="$test_tmp/live.json"
"$validator" --provider aws --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
    --live-action aws-ec2 --report "$report" >/dev/null
test -s "$report"
grep -F '"mode": "live"' "$report" >/dev/null
grep -F '"cleanup": "pass"' "$report" >/dev/null
! grep -F 'fixture-secret-token' "$report" >/dev/null
grep -F 'run-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'terminate-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

all_report="$test_tmp/all-live.json"
: >"$CLOUD_VALIDATION_MOCK_LOG"
"$validator" --provider all --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
    --live-action all --report "$all_report" >/dev/null
grep -F '"cleanup": "pass"' "$all_report" >/dev/null
! grep -F 'fixture-secret-token' "$all_report" >/dev/null
grep -F 'instances create' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'instances delete' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F '/generate-jitconfig' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

printf '%s\n' 'cloud validation shell tests passed'
