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
export AWS_VALIDATION_LAUNCH_TEMPLATE_VERSION=1
export AWS_VALIDATION_OPERATION_TIMEOUT_SECONDS=10
export AWS_VALIDATION_CLEANUP_TIMEOUT_SECONDS=10
export AWS_VALIDATION_CLI_TIMEOUT_SECONDS=2
export AWS_VALIDATION_CLEANUP_POLL_SECONDS=1
export AWS_VALIDATION_CLEANUP_POLL_ATTEMPTS=2
export GCP_VALIDATION_INSTANCE_TEMPLATE=fixture-template
export GCP_VALIDATION_OPERATION_TIMEOUT_SECONDS=10
export GCP_VALIDATION_CLEANUP_TIMEOUT_SECONDS=10
export GCP_VALIDATION_CLI_TIMEOUT_SECONDS=2
export GCP_VALIDATION_CLEANUP_POLL_SECONDS=1
export GCP_VALIDATION_CLEANUP_POLL_ATTEMPTS=2
export CLOUD_VALIDATION_MOCK_LOG="$test_tmp/mock.log"
export CLOUD_VALIDATION_GCP_INSTANCE_STATE="$test_tmp/gcp-instance.state"
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

preflight_report="$test_tmp/preflight.json"
"$validator" --provider all --scope-file "$scope" --report "$preflight_report" >/dev/null
test -s "$preflight_report"
grep -F '"mode": "preflight"' "$preflight_report" >/dev/null
grep -F '"report_version": "cloud-validation-report.v2"' "$preflight_report" >/dev/null
grep -F '"preflight": "pass"' "$preflight_report" >/dev/null
grep -F '"account_id": "000000000000"' "$preflight_report" >/dev/null
grep -F '"project": "fixture-project"' "$preflight_report" >/dev/null
grep -F '"repository": "octo-org/fixture-repo"' "$preflight_report" >/dev/null
grep -F '"evidence_handoff": {"eligible": false' "$preflight_report" >/dev/null
! grep -F 'fixture-secret-token' "$preflight_report" >/dev/null
! grep -F 'aws_account_id' "$preflight_report" >/dev/null

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

failed_preflight_report="$test_tmp/failed-preflight.json"
if "$validator" --provider aws --scope-file "$test_tmp/mismatch.env" \
    --report "$failed_preflight_report" >/dev/null 2>&1; then
    printf '%s\n' 'failed preflight unexpectedly passed' >&2
    exit 1
fi
test -s "$failed_preflight_report"
grep -F '"preflight": "fail"' "$failed_preflight_report" >/dev/null
grep -F '"exit_class": "preflight-failure"' "$failed_preflight_report" >/dev/null
grep -F '"aws": "fail"' "$failed_preflight_report" >/dev/null
! grep -F '999999999999' "$failed_preflight_report" >/dev/null
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

guard_report="$test_tmp/confirmation-guard.json"
if "$validator" --provider aws --allow-live --live-action aws-ec2 \
    --report "$guard_report" >/dev/null 2>&1; then
    printf '%s\n' 'confirmation guard report test unexpectedly passed' >&2
    exit 1
else
    guard_status=$?
fi
[ "$guard_status" -eq 2 ]
test -s "$guard_report"
grep -F '"exit_class": "invocation-error"' "$guard_report" >/dev/null
grep -F '"preflight": "pass"' "$guard_report" >/dev/null
! grep -F 'fixture-secret-token' "$guard_report" >/dev/null

report="$test_tmp/live.json"
"$validator" --provider aws --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
    --live-action aws-ec2 --scope-file "$scope" --report "$report" >/dev/null
test -s "$report"
grep -F '"mode": "live"' "$report" >/dev/null
grep -F '"exit_class": "success"' "$report" >/dev/null
grep -F '"cleanup": "pass"' "$report" >/dev/null
grep -F '"state": "live-succeeded"' "$report" >/dev/null
grep -F '"input_validation": "pass"' "$report" >/dev/null
grep -F '"cleanup_discovery": "pass"' "$report" >/dev/null
! grep -F 'fixture-secret-token' "$report" >/dev/null
grep -F 'run-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'terminate-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'describe-launch-template-versions' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'describe-instances' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F -- '--client-token leo-cloud-validation-' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F -- 'Purpose,Value=leo-cloud-validation' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F -- 'Ephemeral,Value=true' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null

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

discovery_failed_report="$test_tmp/discovery-failed.json"
if CLOUD_VALIDATION_MOCK_DISCOVERY_FAIL=1 "$validator" --provider aws --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action aws-ec2 \
    --scope-file "$scope" --report "$discovery_failed_report" >/dev/null 2>&1; then
    printf '%s\n' 'cleanup discovery failure unexpectedly passed' >&2
    exit 1
fi
grep -F '"cleanup": "fail"' "$discovery_failed_report" >/dev/null
grep -F '"cleanup_discovery": "fail"' "$discovery_failed_report" >/dev/null
grep -F '"exit_class": "cleanup-failure"' "$discovery_failed_report" >/dev/null

if AWS_VALIDATION_LAUNCH_TEMPLATE_VERSION='$Latest' "$validator" --provider aws --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action aws-ec2 \
    --scope-file "$scope" >/dev/null 2>&1; then
    printf '%s\n' 'unpinned launch template unexpectedly passed' >&2
    exit 1
fi
if CLOUD_VALIDATION_MOCK_UNSAFE_TEMPLATE=1 "$validator" --provider aws --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action aws-ec2 \
    --scope-file "$scope" >/dev/null 2>&1; then
    printf '%s\n' 'unsafe launch template unexpectedly passed' >&2
    exit 1
fi
if CLOUD_VALIDATION_MOCK_UNSAFE_IMAGE=1 "$validator" --provider gcp --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action gcp-vm \
    --scope-file "$scope" >/dev/null 2>&1; then
    printf '%s\n' 'unsafe GCP template image unexpectedly passed' >&2
    exit 1
fi
if CLOUD_VALIDATION_MOCK_UNSAFE_LABELS=1 "$validator" --provider gcp --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action gcp-vm \
    --scope-file "$scope" >/dev/null 2>&1; then
    printf '%s\n' 'unsafe GCP labels unexpectedly passed' >&2
    exit 1
fi
if AWS_VALIDATION_CLEANUP_POLL_ATTEMPTS=61 "$validator" --provider aws --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action aws-ec2 \
    --scope-file "$scope" >/dev/null 2>&1; then
    printf '%s\n' 'unbounded cleanup polling unexpectedly passed' >&2
    exit 1
fi
if GCP_VALIDATION_CLEANUP_POLL_ATTEMPTS=61 "$validator" --provider gcp --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action gcp-vm \
    --scope-file "$scope" >/dev/null 2>&1; then
    printf '%s\n' 'unbounded GCP cleanup polling unexpectedly passed' >&2
    exit 1
fi

gcp_report="$test_tmp/gcp-live.json"
: >"$CLOUD_VALIDATION_MOCK_LOG"
rm -f "$CLOUD_VALIDATION_GCP_INSTANCE_STATE"
"$validator" --provider gcp --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
    --live-action gcp-vm --scope-file "$scope" --report "$gcp_report" >/dev/null
grep -F 'instance-templates describe' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'instances create' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'instances delete' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F 'instances describe' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F -- '--request-timeout 2' "$CLOUD_VALIDATION_MOCK_LOG" >/dev/null
grep -F '"input_validation": "pass"' "$gcp_report" >/dev/null
grep -F '"cleanup_discovery": "pass"' "$gcp_report" >/dev/null
grep -F '"labels": {"purpose": "leo-cloud-validation", "ephemeral": "true"}' "$gcp_report" >/dev/null

gcp_cleanup_report="$test_tmp/gcp-cleanup-failed.json"
if CLOUD_VALIDATION_MOCK_CLEANUP_FAIL=1 "$validator" --provider gcp --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action gcp-vm \
    --scope-file "$scope" --report "$gcp_cleanup_report" >/dev/null 2>&1; then
    printf '%s\n' 'GCP cleanup failure unexpectedly passed' >&2
    exit 1
fi
grep -F '"cleanup": "fail"' "$gcp_cleanup_report" >/dev/null
grep -F '"cleanup_discovery": "fail"' "$gcp_cleanup_report" >/dev/null

gcp_discovery_report="$test_tmp/gcp-discovery-failed.json"
if CLOUD_VALIDATION_MOCK_DISCOVERY_FAIL=1 "$validator" --provider gcp --allow-live \
    --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES --live-action gcp-vm \
    --scope-file "$scope" --report "$gcp_discovery_report" >/dev/null 2>&1; then
    printf '%s\n' 'GCP cleanup discovery failure unexpectedly passed' >&2
    exit 1
fi
grep -F '"cleanup": "fail"' "$gcp_discovery_report" >/dev/null
grep -F '"cleanup_discovery": "fail"' "$gcp_discovery_report" >/dev/null

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
