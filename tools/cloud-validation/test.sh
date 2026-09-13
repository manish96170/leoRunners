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

scope="$test_tmp/scope.env"
printf '%s\n' \
    '# Reviewed non-secret provider scope' \
    'aws_account_id=000000000000' \
    'aws_region=us-east-1' \
    'gcp_project=fixture-project' \
    'gcp_zone=us-central1-a' \
    'github_repository=octo-org/fixture-repo' >"$scope"

output=$("$validator" --provider all --scope-file "$scope")
printf '%s\n' "$output" | grep -F 'read-only mode' >/dev/null
! grep -E 'run-instances|terminate-instances|instances create|instances delete|generate-jitconfig' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

if "$validator" --provider aws --scope-file "$test_tmp/mismatch.env" >/dev/null 2>&1; then
    printf '%s\n' 'missing scope file unexpectedly passed' >&2
    exit 1
fi
printf '%s\n' 'aws_account_id=999999999999' >"$test_tmp/mismatch.env"
if "$validator" --provider aws --scope-file "$test_tmp/mismatch.env" >/dev/null 2>&1; then
    printf '%s\n' 'scope mismatch unexpectedly passed' >&2
    exit 1
fi
: >"$CLOUD_VALIDATION_MOCK_LOG"
if "$validator" --provider aws --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
    --live-action aws-ec2 --scope-file "$test_tmp/mismatch.env" >/dev/null 2>&1; then
    printf '%s\n' 'live scope mismatch unexpectedly passed' >&2
    exit 1
fi
! grep -E 'run-instances|terminate-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
printf '%s\n' 'gcp_project=other-project' 'gcp_zone=us-central1-a' >"$test_tmp/gcp-mismatch.env"
if "$validator" --provider gcp --scope-file "$test_tmp/gcp-mismatch.env" >/dev/null 2>&1; then
    printf '%s\n' 'GCP scope mismatch unexpectedly passed' >&2
    exit 1
fi
printf '%s\n' 'github_repository=octo-org/other-repo' >"$test_tmp/github-mismatch.env"
if "$validator" --provider github --scope-file "$test_tmp/github-mismatch.env" >/dev/null 2>&1; then
    printf '%s\n' 'GitHub scope mismatch unexpectedly passed' >&2
    exit 1
fi
printf '%s\n' 'aws_account_id=000000000000' 'unexpected=value' >"$test_tmp/unsafe-scope.env"
if "$validator" --provider aws --scope-file "$test_tmp/unsafe-scope.env" >/dev/null 2>&1; then
    printf '%s\n' 'unsupported scope key unexpectedly passed' >&2
    exit 1
fi
printf '%s\n' 'aws_account_id=000000000000' 'aws_account_id=000000000000' >"$test_tmp/duplicate-scope.env"
if "$validator" --provider aws --scope-file "$test_tmp/duplicate-scope.env" >/dev/null 2>&1; then
    printf '%s\n' 'duplicate scope key unexpectedly passed' >&2
    exit 1
fi
: >"$test_tmp/empty-scope.env"
if "$validator" --provider aws --scope-file "$test_tmp/empty-scope.env" >/dev/null 2>&1; then
    printf '%s\n' 'empty scope file unexpectedly passed' >&2
    exit 1
fi

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
    --live-action aws-ec2 --scope-file "$scope" --report "$report" >/dev/null
test -s "$report"
grep -F '"mode": "live"' "$report" >/dev/null
grep -F '"cleanup": "pass"' "$report" >/dev/null
! grep -F 'fixture-secret-token' "$report" >/dev/null
grep -F 'run-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'terminate-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

evidence="$test_tmp/evidence.json"
"$validator" --provider aws --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
    --live-action aws-ec2 --scope-file "$scope" --evidence-report "$evidence" >/dev/null
test -s "$evidence"
"$script_dir/../evidence-validation/validate.sh" "$evidence" >/dev/null
grep -F 'ControlledValidationEvidence' "$evidence" >/dev/null
! grep -E 'fixture-secret-token|i-0123456789abcdef0' "$evidence" >/dev/null

failed_evidence="$test_tmp/failed-evidence.json"
if CLOUD_VALIDATION_MOCK_CLEANUP_FAIL=1 "$validator" --provider aws --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action aws-ec2 \
    --evidence-report "$failed_evidence" >/dev/null 2>&1; then
    printf '%s\n' 'cleanup failure unexpectedly passed' >&2
    exit 1
fi
test ! -e "$failed_evidence"

all_report="$test_tmp/all-live.json"
: >"$CLOUD_VALIDATION_MOCK_LOG"
"$validator" --provider all --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
    --live-action all --scope-file "$scope" --report "$all_report" >/dev/null
grep -F '"cleanup": "pass"' "$all_report" >/dev/null
! grep -F 'fixture-secret-token' "$all_report" >/dev/null
grep -F 'instances create' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'instances delete' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F '/generate-jitconfig' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

printf '%s\n' 'cloud validation shell tests passed'
