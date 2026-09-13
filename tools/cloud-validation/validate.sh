#!/bin/sh
set -u

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

confirmation='I_UNDERSTAND_EPHEMERAL_RESOURCES'
provider=all
allow_live=0
live_action=none
report_path=${CLOUD_VALIDATION_REPORT:-}
mode=preflight
overall_status=0
cleanup_status=not-run
aws_instance_id=
gcp_instance_name=
github_jit_status=not-run
aws_status=not-run
gcp_status=not-run
github_status=not-run
required_env_status=not-run
live_action_status=not-run

usage() {
    cat <<'USAGE'
Usage: validate.sh [options]

Read-only preflight is the default and never creates or terminates resources.

Options:
  --provider NAME       aws, gcp, github, or all (default: all)
  --allow-live          permit an explicitly selected live validation action
  --confirm VALUE       must equal I_UNDERSTAND_EPHEMERAL_RESOURCES
  --live-action NAME    aws-ec2, gcp-vm, github-jit, or all
  --report PATH         write the redacted live JSON report to PATH
  --help                show this help

Required live variables:
  AWS_VALIDATION_LAUNCH_TEMPLATE_ID for aws-ec2
  GCP_VALIDATION_INSTANCE_TEMPLATE for gcp-vm
  GITHUB_TOKEN, GITHUB_REPOSITORY, GITHUB_RUNNER_GROUP_ID, and
  GITHUB_RUNNER_LABELS for github-jit
USAGE
}

die_usage() {
    printf 'cloud validation invocation error: %s\n' "$1" >&2
    usage >&2
    exit 2
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --provider)
            [ "$#" -ge 2 ] || die_usage '--provider needs a value'
            provider=$2
            shift
            ;;
        --allow-live) allow_live=1 ;;
        --confirm)
            [ "$#" -ge 2 ] || die_usage '--confirm needs a value'
            supplied_confirmation=$2
            shift
            ;;
        --live-action)
            [ "$#" -ge 2 ] || die_usage '--live-action needs a value'
            live_action=$2
            shift
            ;;
        --report)
            [ "$#" -ge 2 ] || die_usage '--report needs a path'
            report_path=$2
            shift
            ;;
        --help|-h) usage; exit 0 ;;
        *) die_usage "unknown option: $1" ;;
    esac
    shift
done

supplied_confirmation=${supplied_confirmation:-}

case "$provider" in
    aws|gcp|github|all) ;;
    *) die_usage "unsupported provider: $provider" ;;
esac
case "$live_action" in
    none|aws-ec2|gcp-vm|github-jit|all) ;;
    *) die_usage "unsupported live action: $live_action" ;;
esac
case "$live_action:$provider" in
    aws-ec2:aws|gcp-vm:gcp|github-jit:github|all:all|all:aws|all:gcp|all:github|none:*) ;;
    *) die_usage "live action $live_action does not match provider $provider" ;;
esac

has_provider() {
    case "$provider:$1" in
        all:*|*:$1) return 0 ;;
        *) return 1 ;;
    esac
}

require_command() {
    command -v "$1" >/dev/null 2>&1
}

mark_check() {
    field=$1
    result=$2
    message=$3
    case "$field" in
        required_env) required_env_status=$result ;;
        aws) aws_status=$result ;;
        gcp) gcp_status=$result ;;
        github) github_status=$result ;;
    esac
    if [ "$result" = pass ]; then
        printf 'PASS %-16s %s\n' "$field" "$message"
    else
        printf 'FAIL %-16s %s\n' "$field" "$message" >&2
        overall_status=1
    fi
}

check_required_env() {
    missing=
    if has_provider aws; then
        [ -n "${AWS_REGION:-${AWS_DEFAULT_REGION:-}}" ] || missing="$missing AWS_REGION"
    fi
    if has_provider gcp; then
        [ -n "${GCP_PROJECT:-${GOOGLE_CLOUD_PROJECT:-}}" ] || missing="$missing GCP_PROJECT"
    fi
    if has_provider github; then
        [ -n "${GITHUB_TOKEN:-${GH_TOKEN:-}}" ] || missing="$missing GITHUB_TOKEN"
        [ -n "${GITHUB_REPOSITORY:-}" ] || missing="$missing GITHUB_REPOSITORY"
        [ -n "${GITHUB_RUNNER_GROUP_ID:-}" ] || missing="$missing GITHUB_RUNNER_GROUP_ID"
        [ -n "${GITHUB_RUNNER_LABELS:-}" ] || missing="$missing GITHUB_RUNNER_LABELS"
    fi
    if [ -n "$missing" ]; then
        mark_check required_env fail "missing required environment variables:$missing"
    else
        mark_check required_env pass 'provider prerequisites are present'
    fi
}

check_aws() {
    if ! require_command aws; then
        mark_check aws fail 'AWS CLI is not installed'
        return
    fi
    region=${AWS_REGION:-${AWS_DEFAULT_REGION:-}}
    if [ -z "$region" ]; then
        mark_check aws fail 'AWS_REGION or AWS_DEFAULT_REGION is required'
        return
    fi
    if ! aws sts get-caller-identity --output json >/dev/null 2>&1; then
        mark_check aws fail 'AWS identity check failed'
        return
    fi
    mark_check aws pass "AWS CLI, identity, and region $region are available"
}

gcp_project() {
    if [ -n "${GCP_PROJECT:-}" ]; then
        printf '%s' "$GCP_PROJECT"
    elif [ -n "${GOOGLE_CLOUD_PROJECT:-}" ]; then
        printf '%s' "$GOOGLE_CLOUD_PROJECT"
    else
        gcloud config get-value project 2>/dev/null | sed -n '1p'
    fi
}

check_gcp() {
    if ! require_command gcloud; then
        mark_check gcp fail 'gcloud CLI is not installed'
        return
    fi
    project=$(gcp_project)
    case "$project" in
        ''|'(unset)') mark_check gcp fail 'GCP project is not configured'; return ;;
    esac
    if ! gcloud auth application-default print-access-token >/dev/null 2>&1; then
        mark_check gcp fail 'Application Default Credentials are unavailable'
        return
    fi
    if ! gcloud projects describe "$project" --format='value(projectId)' >/dev/null 2>&1; then
        mark_check gcp fail "GCP project check failed for $project"
        return
    fi
    mark_check gcp pass "gcloud, ADC, and project $project are available"
}

valid_repository() {
    owner=${1%%/*}
    name=${1#*/}
    [ "$owner" != "$1" ] || return 1
    [ -n "$owner" ] && [ -n "$name" ] || return 1
    case "$owner" in
        *[!A-Za-z0-9_.-]*) return 1 ;;
    esac
    case "$name" in
        *[!A-Za-z0-9_.-]*) return 1 ;;
    esac
    return 0
}

valid_labels() {
    old_ifs=$IFS
    IFS=,
    for label in $1; do
        case "$label" in
            ''|*[!A-Za-z0-9._-]*) IFS=$old_ifs; return 1 ;;
        esac
    done
    IFS=$old_ifs
    return 0
}

github_request() {
    github_tmp=$(mktemp "${TMPDIR:-/tmp}/leo-github.XXXXXX") || return 1
    chmod 600 "$github_tmp" 2>/dev/null || true
    github_status_code=$(curl -sS -o "$github_tmp" -w '%{http_code}' \
        -H 'Accept: application/vnd.github+json' \
        -H 'X-GitHub-Api-Version: 2026-03-10' \
        -H "Authorization: Bearer ${GITHUB_TOKEN:-${GH_TOKEN:-}}" \
        "$1" 2>/dev/null) || github_status_code=000
    rm -f "$github_tmp"
    [ "$github_status_code" = 200 ]
}

check_github() {
    if ! require_command curl; then
        mark_check github fail 'curl is not installed'
        return
    fi
    token=${GITHUB_TOKEN:-${GH_TOKEN:-}}
    repo=${GITHUB_REPOSITORY:-}
    if [ -z "$token" ]; then
        mark_check github fail 'GITHUB_TOKEN or GH_TOKEN is required'
        return
    fi
    if ! valid_repository "$repo"; then
        mark_check github fail 'GITHUB_REPOSITORY must be OWNER/REPOSITORY'
        return
    fi
    case "${GITHUB_RUNNER_GROUP_ID:-}" in
        ''|*[!0-9]*) mark_check github fail 'GITHUB_RUNNER_GROUP_ID must be numeric'; return ;;
    esac
    if ! valid_labels "${GITHUB_RUNNER_LABELS:-}"; then
        mark_check github fail 'GITHUB_RUNNER_LABELS must be comma-separated safe labels'
        return
    fi
    api=${GITHUB_API_URL:-https://api.github.com}
    case "$api" in
        http://*|https://*) ;;
        *) mark_check github fail 'GITHUB_API_URL must use http or https'; return ;;
    esac
    if ! github_request "$api/repos/$repo"; then
        mark_check github fail "repository API check failed for $repo (HTTP $github_status_code)"
        return
    fi
    if ! github_request "$api/repos/$repo/actions/runners"; then
        mark_check github fail "Actions runner API/JIT prerequisite check failed (HTTP $github_status_code)"
        return
    fi
    mark_check github pass 'token, repository, Actions API, runner group, and labels are ready'
}

json_report() {
    printf '{\n'
    printf '  "mode": "%s",\n' "$mode"
    printf '  "allow_live": %s,\n' "$allow_live"
    printf '  "live_action": "%s",\n' "$live_action"
    printf '  "checks": {"required_env": "%s", "aws": "%s", "gcp": "%s", "github": "%s"},\n' \
        "$required_env_status" "$aws_status" "$gcp_status" "$github_status"
    printf '  "live": {"status": "%s", "github_jit": "%s"},\n' "$live_action_status" "$github_jit_status"
    printf '  "cleanup": "%s",\n' "$cleanup_status"
    printf '  "redaction": "secret values and provider response bodies are never emitted"\n'
    printf '}\n'
}

write_report() {
    if [ -n "$report_path" ]; then
        report_dir=$(dirname -- "$report_path")
        [ -d "$report_dir" ] || { printf 'report directory does not exist: %s\n' "$report_dir" >&2; return 1; }
        report_tmp=$(mktemp "$report_dir/.cloud-validation-report.XXXXXX") || return 1
        chmod 600 "$report_tmp" 2>/dev/null || true
        json_report >"$report_tmp" && mv "$report_tmp" "$report_path"
    else
        json_report
    fi
}

cleanup() {
    script_status=$?
    trap - EXIT INT TERM HUP
    cleanup_status=pass
    if [ -n "$aws_instance_id" ]; then
        if ! aws ec2 terminate-instances --instance-ids "$aws_instance_id" --output json >/dev/null 2>&1; then
            cleanup_status=fail
            script_status=1
            printf '%s\n' 'FAIL cleanup          AWS instance termination failed' >&2
        else
            printf '%s\n' 'PASS cleanup          AWS validation instance termination requested'
        fi
    fi
    if [ -n "$gcp_instance_name" ]; then
        if ! gcloud compute instances delete "$gcp_instance_name" \
            --project "$(gcp_project)" --zone "${GCP_ZONE:-us-central1-a}" --quiet >/dev/null 2>&1; then
            cleanup_status=fail
            script_status=1
            printf '%s\n' 'FAIL cleanup          GCP instance deletion failed' >&2
        else
            printf '%s\n' 'PASS cleanup          GCP validation VM deletion requested'
        fi
    fi
    if [ "$cleanup_status" = pass ] && [ "$live_action" = github-jit ]; then
        cleanup_status=pass-no-resource-jit-expires
    fi
    if [ -n "$report_path" ] || [ "$mode" = live ]; then
        if ! write_report; then
            script_status=1
            printf '%s\n' 'FAIL report           unable to write redacted JSON report' >&2
        fi
    fi
    exit "$script_status"
}

run_aws_live() {
    [ -n "${AWS_VALIDATION_LAUNCH_TEMPLATE_ID:-}" ] || {
        printf '%s\n' 'live AWS validation requires AWS_VALIDATION_LAUNCH_TEMPLATE_ID' >&2
        return 1
    }
    token="leo-cloud-validation-$(date -u +%Y%m%d%H%M%S)-$$"
    template_version=${AWS_VALIDATION_LAUNCH_TEMPLATE_VERSION:-'$Latest'}
    output=$(aws ec2 run-instances \
        --launch-template "LaunchTemplateId=${AWS_VALIDATION_LAUNCH_TEMPLATE_ID},Version=$template_version" \
        --min-count 1 --max-count 1 --client-token "$token" \
        --tag-specifications 'ResourceType=instance,Tags=[{Key=Purpose,Value=leo-cloud-validation},{Key=Ephemeral,Value=true}]' \
        --query 'Instances[0].InstanceId' --output text 2>/dev/null) || {
        printf '%s\n' 'live AWS validation launch failed' >&2
        return 1
    }
    case "$output" in
        i-[A-Za-z0-9]*) aws_instance_id=$output; printf '%s\n' 'PASS live-aws         launched validation instance (identifier withheld)'; return 0 ;;
        *) printf '%s\n' 'live AWS validation returned an invalid instance identifier' >&2; return 1 ;;
    esac
}

run_gcp_live() {
    [ -n "${GCP_VALIDATION_INSTANCE_TEMPLATE:-}" ] || {
        printf '%s\n' 'live GCP validation requires GCP_VALIDATION_INSTANCE_TEMPLATE' >&2
        return 1
    }
    project=$(gcp_project)
    zone=${GCP_ZONE:-us-central1-a}
    gcp_instance_name="leo-cloud-validation-$(date -u +%Y%m%d%H%M%S)-$$"
    gcloud compute instances create "$gcp_instance_name" \
        --project "$project" --zone "$zone" \
        --source-instance-template "${GCP_VALIDATION_INSTANCE_TEMPLATE}" \
        --labels purpose=leo-cloud-validation,ephemeral=true --quiet >/dev/null 2>&1 || {
        gcp_instance_name=
        printf '%s\n' 'live GCP validation VM creation failed' >&2
        return 1
    }
    printf '%s\n' 'PASS live-gcp         created validation VM (identifier withheld)'
}

labels_json() {
    old_ifs=$IFS
    IFS=,
    first=1
    for label in $GITHUB_RUNNER_LABELS; do
        [ "$first" -eq 1 ] || printf ','
        printf '"%s"' "$label"
        first=0
    done
    IFS=$old_ifs
}

run_github_live() {
    repo=${GITHUB_REPOSITORY:-}
    token=${GITHUB_TOKEN:-${GH_TOKEN:-}}
    api=${GITHUB_API_URL:-https://api.github.com}
    runner_name="leo-cloud-validation-$(date -u +%Y%m%d%H%M%S)-$$"
    body=$(printf '{"name":"%s","runner_group_id":%s,"labels":[%s],"work_folder":"_work"}' \
        "$runner_name" "$GITHUB_RUNNER_GROUP_ID" "$(labels_json)")
    jit_tmp=$(mktemp "${TMPDIR:-/tmp}/leo-jit.XXXXXX") || return 1
    chmod 600 "$jit_tmp" 2>/dev/null || true
    code=$(curl -sS -o "$jit_tmp" -w '%{http_code}' -X POST \
        -H 'Accept: application/vnd.github+json' \
        -H 'X-GitHub-Api-Version: 2026-03-10' \
        -H "Authorization: Bearer $token" \
        -H 'Content-Type: application/json' \
        --data "$body" "$api/repos/$repo/actions/runners/generate-jitconfig" 2>/dev/null) || code=000
    rm -f "$jit_tmp"
    if [ "$code" != 201 ]; then
        printf 'live GitHub JIT validation failed (HTTP %s)\n' "$code" >&2
        return 1
    fi
    github_jit_status=pass
    printf '%s\n' 'PASS live-github      generated short-lived JIT configuration (secret withheld)'
    return 0
}

check_required_env
has_provider aws && check_aws
has_provider gcp && check_gcp
has_provider github && check_github

if [ "$allow_live" -eq 0 ]; then
    [ "$live_action" = none ] || {
        printf '%s\n' 'live action requested without --allow-live; no resources were touched' >&2
        exit 2
    }
    printf '%s\n' 'Cloud preflight passed in read-only mode; no resources were created or terminated.'
    exit "$overall_status"
fi

[ "$supplied_confirmation" = "$confirmation" ] || {
    printf '%s\n' 'live validation requires --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES; no resources were touched' >&2
    exit 2
}
[ "$live_action" != none ] || {
    printf '%s\n' 'live validation requires an explicit --live-action; no resources were touched' >&2
    exit 2
}

mode=live
trap cleanup EXIT INT TERM HUP
live_action_status=pass
case "$live_action" in
    aws-ec2) run_aws_live || live_action_status=fail ;;
    gcp-vm) run_gcp_live || live_action_status=fail ;;
    github-jit) run_github_live || live_action_status=fail ;;
    all)
        run_aws_live || live_action_status=fail
        run_gcp_live || live_action_status=fail
        run_github_live || live_action_status=fail
        ;;
esac
[ "$live_action_status" = pass ] || overall_status=1
exit "$overall_status"
