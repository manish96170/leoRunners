#!/bin/sh
set -u

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

confirmation='I_UNDERSTAND_EPHEMERAL_RESOURCES'
provider=all
allow_live=0
live_action=none
report_path=${CLOUD_VALIDATION_REPORT:-}
evidence_report_path=${CLOUD_VALIDATION_EVIDENCE_REPORT:-}
scope_file=${CLOUD_VALIDATION_SCOPE_FILE:-}
mode=preflight
overall_status=0
cleanup_status=not-run
aws_cleanup_discovery_status=not-run
aws_input_status=not-run
validation_state=preflight
failure_reason=
aws_client_token=
aws_instance_id=
gcp_instance_name=
gcp_input_status=not-run
gcp_cleanup_discovery_status=not-run
github_jit_status=not-run
aws_status=not-run
gcp_status=not-run
github_status=not-run
required_env_status=not-run
live_action_status=not-run
preflight_status=not-run
exit_class=not-run
validation_started_at=
validation_started_epoch=
cleanup_started_at=
cleanup_completed_at=
validation_finished_at=
validation_finished_epoch=
scope_status=not-configured
scope_aws_match=not-checked
scope_gcp_match=not-checked
scope_github_match=not-checked
scope_aws_account=
scope_aws_region=
scope_gcp_project=
scope_gcp_zone=
scope_github_repository=
identity_aws_account=
identity_aws_region=
identity_gcp_project=
identity_gcp_zone=
identity_github_repository=

aws_operation_timeout_seconds=${AWS_VALIDATION_OPERATION_TIMEOUT_SECONDS:-300}
aws_cleanup_timeout_seconds=${AWS_VALIDATION_CLEANUP_TIMEOUT_SECONDS:-120}
aws_cli_timeout_seconds=${AWS_VALIDATION_CLI_TIMEOUT_SECONDS:-30}
aws_cleanup_poll_seconds=${AWS_VALIDATION_CLEANUP_POLL_SECONDS:-1}
aws_cleanup_poll_attempts=${AWS_VALIDATION_CLEANUP_POLL_ATTEMPTS:-3}
gcp_operation_timeout_seconds=${GCP_VALIDATION_OPERATION_TIMEOUT_SECONDS:-300}
gcp_cleanup_timeout_seconds=${GCP_VALIDATION_CLEANUP_TIMEOUT_SECONDS:-120}
gcp_cli_timeout_seconds=${GCP_VALIDATION_CLI_TIMEOUT_SECONDS:-30}
gcp_cleanup_poll_seconds=${GCP_VALIDATION_CLEANUP_POLL_SECONDS:-1}
gcp_cleanup_poll_attempts=${GCP_VALIDATION_CLEANUP_POLL_ATTEMPTS:-3}
validation_tag_purpose=leo-cloud-validation
validation_tag_ephemeral=true

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
  --evidence-report PATH write schema-compatible controlled-validation evidence
  --scope-file PATH     reviewed expected cloud scope (key=value, read-only checks)
  --help                show this help

Required live variables:
  AWS_VALIDATION_LAUNCH_TEMPLATE_ID and numeric AWS_VALIDATION_LAUNCH_TEMPLATE_VERSION for aws-ec2
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
        --evidence-report)
            [ "$#" -ge 2 ] || die_usage '--evidence-report needs a path'
            evidence_report_path=$2
            shift
            ;;
        --scope-file)
            [ "$#" -ge 2 ] || die_usage '--scope-file needs a path'
            scope_file=$2
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

valid_scope_value() {
    case "$1" in
        ''|*[!A-Za-z0-9_./:-]*) return 1 ;;
    esac
    return 0
}

valid_positive_integer() {
    case "$1" in
        ''|*[!0-9]*) return 1 ;;
    esac
    [ "$1" -gt 0 ] 2>/dev/null
}

validate_timeouts() {
    valid_positive_integer "$aws_operation_timeout_seconds" || overall_status=1
    valid_positive_integer "$aws_cleanup_timeout_seconds" || overall_status=1
    valid_positive_integer "$aws_cli_timeout_seconds" || overall_status=1
    valid_positive_integer "$aws_cleanup_poll_seconds" || overall_status=1
    valid_positive_integer "$aws_cleanup_poll_attempts" || overall_status=1
    if [ "$aws_cleanup_poll_attempts" -gt 60 ] 2>/dev/null; then
        overall_status=1
    fi
    valid_positive_integer "$gcp_operation_timeout_seconds" || overall_status=1
    valid_positive_integer "$gcp_cleanup_timeout_seconds" || overall_status=1
    valid_positive_integer "$gcp_cli_timeout_seconds" || overall_status=1
    valid_positive_integer "$gcp_cleanup_poll_seconds" || overall_status=1
    valid_positive_integer "$gcp_cleanup_poll_attempts" || overall_status=1
    if [ "$gcp_cleanup_poll_attempts" -gt 60 ] 2>/dev/null; then
        overall_status=1
    fi
    [ "$overall_status" -eq 0 ] || printf '%s\n' 'FAIL timeout          provider operation and cleanup timeout inputs are invalid or unbounded' >&2
}

load_scope_file() {
    [ -n "$scope_file" ] || return 0
    [ -f "$scope_file" ] || {
        scope_status=fail
        printf 'FAIL scope            scope file does not exist: %s\n' "$scope_file" >&2
        overall_status=1
        return 0
    }
    scope_status=pass
    scope_key_count=0
    while IFS= read -r scope_line || [ -n "$scope_line" ]; do
        case "$scope_line" in
            ''|'#'*) continue ;;
            *=*) ;;
            *) scope_status=fail; printf '%s\n' 'FAIL scope            scope file must contain key=value lines' >&2; continue ;;
        esac
        scope_key=${scope_line%%=*}
        scope_value=${scope_line#*=}
        scope_key_count=$((scope_key_count + 1))
        case "$scope_key" in
            aws_account_id)
                [ -z "$scope_aws_account" ] && valid_scope_value "$scope_value" || {
                    scope_status=fail; printf '%s\n' 'FAIL scope            duplicate or invalid aws_account_id' >&2; continue;
                }
                scope_aws_account=$scope_value ;;
            aws_region)
                [ -z "$scope_aws_region" ] && valid_scope_value "$scope_value" || {
                    scope_status=fail; printf '%s\n' 'FAIL scope            duplicate or invalid aws_region' >&2; continue;
                }
                scope_aws_region=$scope_value ;;
            gcp_project)
                [ -z "$scope_gcp_project" ] && valid_scope_value "$scope_value" || {
                    scope_status=fail; printf '%s\n' 'FAIL scope            duplicate or invalid gcp_project' >&2; continue;
                }
                scope_gcp_project=$scope_value ;;
            gcp_zone)
                [ -z "$scope_gcp_zone" ] && valid_scope_value "$scope_value" || {
                    scope_status=fail; printf '%s\n' 'FAIL scope            duplicate or invalid gcp_zone' >&2; continue;
                }
                scope_gcp_zone=$scope_value ;;
            github_repository)
                [ -z "$scope_github_repository" ] && valid_repository "$scope_value" || {
                    scope_status=fail; printf '%s\n' 'FAIL scope            duplicate or invalid github_repository' >&2; continue;
                }
                scope_github_repository=$scope_value ;;
            *) scope_status=fail; printf 'FAIL scope            unsupported scope key: %s\n' "$scope_key" >&2 ;;
        esac
    done <"$scope_file"
    if [ "$scope_key_count" -eq 0 ]; then
        scope_status=fail
        printf '%s\n' 'FAIL scope            scope file must contain at least one expected value' >&2
    fi
    if [ "$scope_status" = pass ]; then
        printf '%s\n' 'PASS scope            reviewed provider scope loaded'
    else
        overall_status=1
    fi
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
    aws_account=$(aws sts get-caller-identity --query Account --output text 2>/dev/null) || {
        mark_check aws fail 'AWS identity check failed'
        return
    }
    case "$aws_account" in
        ''|*[!0-9]*) mark_check aws fail 'AWS identity response was invalid'; return ;;
    esac
    identity_aws_account=$aws_account
    identity_aws_region=$region
    if [ -n "$scope_aws_account" ] && [ "$aws_account" != "$scope_aws_account" ]; then
        scope_aws_match=fail
        mark_check aws fail 'AWS account does not match reviewed scope'
        return
    fi
    if [ -n "$scope_aws_region" ] && [ "$region" != "$scope_aws_region" ]; then
        scope_aws_match=fail
        mark_check aws fail 'AWS region does not match reviewed scope'
        return
    fi
    scope_aws_match=pass
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
    described_project=$(gcloud projects describe "$project" --format='value(projectId)' 2>/dev/null) || {
        mark_check gcp fail "GCP project check failed for $project"
        return
    }
    [ "$described_project" = "$project" ] || {
        mark_check gcp fail 'GCP project identity does not match configured project'
        return
    }
    identity_gcp_project=$project
    zone=${GCP_ZONE:-us-central1-a}
    if [ -n "$scope_gcp_project" ] && [ "$project" != "$scope_gcp_project" ]; then
        scope_gcp_match=fail
        mark_check gcp fail 'GCP project does not match reviewed scope'
        return
    fi
    described_zone=$(gcloud compute zones describe "$zone" --project "$project" \
        --format='value(name)' --request-timeout "$gcp_cli_timeout_seconds" 2>/dev/null) || {
        mark_check gcp fail 'GCP zone read-only check failed'
        return
    }
    [ "$described_zone" = "$zone" ] || {
        mark_check gcp fail 'GCP zone identity does not match configured zone'
        return
    }
    if [ -n "$scope_gcp_zone" ] && [ "$zone" != "$scope_gcp_zone" ]; then
        scope_gcp_match=fail
        mark_check gcp fail 'GCP zone does not match reviewed scope'
        return
    fi
    identity_gcp_zone=$zone
    scope_gcp_match=pass
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

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
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
    identity_github_repository=$repo
    if [ -n "$scope_github_repository" ] && [ "$repo" != "$scope_github_repository" ]; then
        scope_github_match=fail
        mark_check github fail 'GitHub repository does not match reviewed scope'
        return
    fi
    scope_github_match=pass
    mark_check github pass 'token, repository, Actions API, runner group, and labels are ready'
}

json_report() {
    printf '{\n'
    printf '  "report_version": "cloud-validation-report.v2",\n'
    printf '  "mode": "%s",\n' "$mode"
    printf '  "allow_live": %s,\n' "$allow_live"
    printf '  "live_action": "%s",\n' "$live_action"
    printf '  "status": {"preflight": "%s", "live": "%s", "cleanup": "%s", "exit_class": "%s"},\n' \
        "$preflight_status" "$live_action_status" "$cleanup_status" "$exit_class"
    printf '  "checks": {"required_env": "%s", "scope": "%s", "aws": "%s", "gcp": "%s", "github": "%s"},\n' \
        "$required_env_status" "$scope_status" "$aws_status" "$gcp_status" "$github_status"
    if [ -n "$scope_file" ]; then scope_configured=true; else scope_configured=false; fi
    printf '  "scope": {"configured": %s, "aws": "%s", "gcp": "%s", "github": "%s"},\n' \
        "$scope_configured" "$scope_aws_match" "$scope_gcp_match" "$scope_github_match"
    printf '  "identity": {"aws": {"account_id": "%s", "region": "%s"}, "gcp": {"project": "%s", "zone": "%s"}, "github": {"repository": "%s"}},\n' \
        "$(json_escape "$identity_aws_account")" "$(json_escape "$identity_aws_region")" \
        "$(json_escape "$identity_gcp_project")" "$(json_escape "$identity_gcp_zone")" \
        "$(json_escape "$identity_github_repository")"
    printf '  "live": {"status": "%s", "github_jit": "%s"},\n' "$live_action_status" "$github_jit_status"
    printf '  "evidence_handoff": {"eligible": false, "reason": "read-only-preflight-is-not-live-evidence"},\n'
    printf '  "cleanup": "%s",\n' "$cleanup_status"
    printf '  "state": "%s",\n' "$validation_state"
    printf '  "failure_reason": "%s",\n' "$failure_reason"
    printf '  "aws": {"input_validation": "%s", "cleanup_discovery": "%s", "client_token": "withheld", "tags": {"Purpose": "leo-cloud-validation", "Ephemeral": "true"}},\n' "$aws_input_status" "$aws_cleanup_discovery_status"
    printf '  "gcp": {"input_validation": "%s", "cleanup_discovery": "%s", "template": "withheld", "image": "withheld", "labels": {"purpose": "leo-cloud-validation", "ephemeral": "true"}},\n' "$gcp_input_status" "$gcp_cleanup_discovery_status"
    printf '  "timeouts": {"aws": {"operation_seconds": %s, "cleanup_seconds": %s, "cli_seconds": %s}, "gcp": {"operation_seconds": %s, "cleanup_seconds": %s, "cli_seconds": %s}},\n' \
        "$aws_operation_timeout_seconds" "$aws_cleanup_timeout_seconds" "$aws_cli_timeout_seconds" \
        "$gcp_operation_timeout_seconds" "$gcp_cleanup_timeout_seconds" "$gcp_cli_timeout_seconds"
    printf '  "redaction": "secret values and provider response bodies are never emitted"\n'
    printf '}\n'
}

now_iso() {
    date -u '+%Y-%m-%dT%H:%M:%SZ'
}

now_epoch() {
    date -u '+%s'
}

evidence_provider() {
    case "$live_action" in
        aws-ec2) printf '%s' aws ;;
        gcp-vm) printf '%s' gcp ;;
        *) return 1 ;;
    esac
}

evidence_report() {
    evidence_provider_name=$(evidence_provider) || return 1
    evidence_region=${AWS_REGION:-${AWS_DEFAULT_REGION:-}}
    [ "$evidence_provider_name" = gcp ] && evidence_region=${GCP_REGION:-${GCP_REGION_NAME:-us-central1}}
    evidence_run_id=${LEO_EVIDENCE_RUN_ID:-cloud-validation-$(date -u '+%Y%m%dT%H%M%SZ')-$$}
    evidence_repository=${LEO_EVIDENCE_REPOSITORY:-controlled/validation}
    evidence_workflow=${LEO_EVIDENCE_WORKFLOW:-controlled-live-validation}
    evidence_duration_ms=$(( (validation_finished_epoch - validation_started_epoch) * 1000 ))
    [ "$evidence_duration_ms" -ge 0 ] || return 1
    resource_ref="${evidence_provider_name}:ephemeral:controlled-validation"
    printf '{\n'
    printf '  "apiVersion": "validation.leorunners.io/v1",\n'
    printf '  "kind": "ControlledValidationEvidence",\n'
    printf '  "metadata": {"name": "%s-ephemeral-run", "version": "1.0.0"},\n' "$evidence_provider_name"
    printf '  "spec": {\n'
    printf '    "run": {"run_id": "%s", "repository": "%s", "workflow": "%s", "provider": "%s", "region": "%s", "started_at": "%s", "finished_at": "%s", "duration_ms": %s},\n' \
        "$evidence_run_id" "$evidence_repository" "$evidence_workflow" "$evidence_provider_name" "$evidence_region" \
        "$validation_started_at" "$validation_finished_at" "$evidence_duration_ms"
    printf '    "lifecycle": ['
    printf '{"name":"queued","status":"skipped","observed_at":"%s","duration_ms":0},' "$validation_started_at"
    printf '{"name":"provisioning","status":"observed","observed_at":"%s","duration_ms":0},' "$validation_started_at"
    printf '{"name":"ready","status":"skipped","observed_at":"%s","duration_ms":0},' "$validation_started_at"
    printf '{"name":"running","status":"skipped","observed_at":"%s","duration_ms":0},' "$validation_started_at"
    printf '{"name":"completed","status":"skipped","observed_at":"%s","duration_ms":0},' "$validation_finished_at"
    printf '{"name":"cleanup_started","status":"observed","observed_at":"%s","duration_ms":0},' "$cleanup_started_at"
    printf '{"name":"cleanup_completed","status":"observed","observed_at":"%s","duration_ms":0}' "$cleanup_completed_at"
    printf '],\n'
    printf '    "cleanup": {"attempted": true, "completed": true, "resource_refs": ["%s"], "verified_at": "%s"},\n' "$resource_ref" "$cleanup_completed_at"
    printf '    "evidence": {"metrics": [], "logs": [], "alarms": [], "dashboards": []}\n'
    printf '  }\n}\n'
}

write_evidence_report() {
    [ -n "$evidence_report_path" ] || return 0
    [ "$mode" = live ] || { printf '%s\n' 'FAIL evidence        evidence requires live mode' >&2; return 1; }
    [ "$live_action" = aws-ec2 ] || [ "$live_action" = gcp-vm ] || {
        printf '%s\n' 'FAIL evidence        evidence requires exactly one AWS or GCP live action' >&2
        return 1
    }
    [ "$script_status" -eq 0 ] || { printf '%s\n' 'FAIL evidence        failed validation cannot produce evidence' >&2; return 1; }
    [ "$cleanup_status" = pass ] || { printf '%s\n' 'FAIL evidence        incomplete cleanup cannot produce evidence' >&2; return 1; }
    [ -n "$validation_started_at" ] && [ -n "$cleanup_started_at" ] && [ -n "$cleanup_completed_at" ] || {
        printf '%s\n' 'FAIL evidence        required lifecycle checkpoints are unavailable' >&2
        return 1
    }
    report_dir=$(dirname -- "$evidence_report_path")
    [ -d "$report_dir" ] || { printf 'evidence directory does not exist: %s\n' "$report_dir" >&2; return 1; }
    evidence_tmp=$(mktemp "$report_dir/.cloud-validation-evidence.XXXXXX") || return 1
    chmod 600 "$evidence_tmp" 2>/dev/null || true
    evidence_report >"$evidence_tmp" || { rm -f "$evidence_tmp"; return 1; }
    evidence_validator="$script_dir/../evidence-validation/validate.sh"
    [ -x "$evidence_validator" ] || { rm -f "$evidence_tmp"; printf '%s\n' 'FAIL evidence        evidence validator is unavailable' >&2; return 1; }
    "$evidence_validator" "$evidence_tmp" >/dev/null 2>&1 || {
        rm -f "$evidence_tmp"
        printf '%s\n' 'FAIL evidence        generated evidence failed schema validation' >&2
        return 1
    }
    mv "$evidence_tmp" "$evidence_report_path"
    printf 'PASS evidence        wrote controlled-validation evidence to %s\n' "$evidence_report_path"
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

write_preflight_report() {
    [ -n "$report_path" ] || return 0
    validation_finished_at=$(now_iso)
    validation_finished_epoch=$(now_epoch)
    if [ "$overall_status" -eq 0 ]; then
        preflight_status=pass
        validation_state=preflight-passed
        [ "$exit_class" = not-run ] && exit_class=success
    else
        preflight_status=fail
        validation_state=preflight-failed
        [ "$exit_class" = not-run ] && exit_class=preflight-failure
    fi
    write_report
}

cleanup() {
    script_status=$?
    trap - EXIT INT TERM HUP
    if [ "$script_status" -eq 0 ]; then
        exit_class=success
        validation_state=live-succeeded
    else
        exit_class=live-failure
        validation_state=live-failed
    fi
    cleanup_started_at=$(now_iso)
    cleanup_status=pass
    if [ -n "$aws_instance_id" ]; then
        if ! aws ec2 terminate-instances --instance-ids "$aws_instance_id" \
            --cli-read-timeout "$aws_cli_timeout_seconds" \
            --cli-connect-timeout "$aws_cli_timeout_seconds" --output json >/dev/null 2>&1; then
            cleanup_status=fail
            aws_cleanup_discovery_status=fail
            script_status=1
            exit_class=cleanup-failure
            printf '%s\n' 'FAIL cleanup          AWS instance termination failed' >&2
        else
            cleanup_deadline=$(( $(now_epoch) + aws_cleanup_timeout_seconds ))
            aws_cleanup_discovery_status=timeout
            cleanup_attempt=0
            while [ "$cleanup_attempt" -lt "$aws_cleanup_poll_attempts" ]; do
                cleanup_attempt=$((cleanup_attempt + 1))
                [ "$(now_epoch)" -le "$cleanup_deadline" ] || break
                discovery_output=$(aws ec2 describe-instances --instance-ids "$aws_instance_id" \
                    --query "Reservations[].Instances[?State.Name!='terminated' && State.Name!='shutting-down'].InstanceId" \
                    --cli-read-timeout "$aws_cli_timeout_seconds" \
                    --cli-connect-timeout "$aws_cli_timeout_seconds" --output text 2>/dev/null) || discovery_output=__ERROR__
                if [ "$discovery_output" = __ERROR__ ]; then
                    aws_cleanup_discovery_status=fail
                    break
                fi
                case "$discovery_output" in
                    ''|None|null|'[]') aws_cleanup_discovery_status=pass; break ;;
                esac
                [ "$cleanup_attempt" -lt "$aws_cleanup_poll_attempts" ] && sleep "$aws_cleanup_poll_seconds"
            done
            if [ "$aws_cleanup_discovery_status" != pass ]; then
                cleanup_status=fail
                script_status=1
                exit_class=cleanup-failure
                printf '%s\n' 'FAIL cleanup          AWS cleanup discovery did not verify termination' >&2
            else
                printf '%s\n' 'PASS cleanup          AWS validation termination and discovery verified'
            fi
        fi
    fi
    if [ -n "$gcp_instance_name" ]; then
        if ! gcloud compute instances delete "$gcp_instance_name" \
            --project "$(gcp_project)" --zone "${GCP_ZONE:-us-central1-a}" --quiet \
            --request-timeout "$gcp_cli_timeout_seconds" >/dev/null 2>&1; then
            cleanup_status=fail
            gcp_cleanup_discovery_status=fail
            script_status=1
            exit_class=cleanup-failure
            printf '%s\n' 'FAIL cleanup          GCP instance deletion failed' >&2
        else
            gcp_cleanup_deadline=$(( $(now_epoch) + gcp_cleanup_timeout_seconds ))
            gcp_cleanup_discovery_status=timeout
            gcp_cleanup_attempt=0
            while [ "$gcp_cleanup_attempt" -lt "$gcp_cleanup_poll_attempts" ]; do
                gcp_cleanup_attempt=$((gcp_cleanup_attempt + 1))
                [ "$(now_epoch)" -le "$gcp_cleanup_deadline" ] || break
                gcp_discovery_output=$(gcloud compute instances describe "$gcp_instance_name" \
                    --project "$(gcp_project)" --zone "${GCP_ZONE:-us-central1-a}" \
                    --format='value(name)' --request-timeout "$gcp_cli_timeout_seconds" 2>&1) || gcp_discovery_status=$?
                gcp_discovery_status=${gcp_discovery_status:-0}
                if [ "$gcp_discovery_status" -ne 0 ]; then
                    case "$gcp_discovery_output" in
                        *NOT_FOUND*|*'not found'*) gcp_cleanup_discovery_status=pass; break ;;
                        *) gcp_cleanup_discovery_status=fail; break ;;
                    esac
                fi
                gcp_discovery_status=
                [ "$gcp_cleanup_attempt" -lt "$gcp_cleanup_poll_attempts" ] && sleep "$gcp_cleanup_poll_seconds"
            done
            if [ "$gcp_cleanup_discovery_status" != pass ]; then
                cleanup_status=fail
                script_status=1
                exit_class=cleanup-failure
                printf '%s\n' 'FAIL cleanup          GCP cleanup discovery did not verify deletion' >&2
            else
                printf '%s\n' 'PASS cleanup          GCP validation deletion and discovery verified'
            fi
        fi
    fi
    if [ "$cleanup_status" = pass ] && [ "$live_action" = github-jit ]; then
        cleanup_status=pass-no-resource-jit-expires
    fi
    if [ "$cleanup_status" = pass ]; then
        cleanup_completed_at=$(now_iso)
    fi
    validation_finished_at=$(now_iso)
    validation_finished_epoch=$(now_epoch)
    if ! write_evidence_report; then
        script_status=1
    fi
    if [ -n "$report_path" ] || [ "$mode" = live ]; then
        if ! write_report; then
            script_status=1
            printf '%s\n' 'FAIL report           unable to write redacted JSON report' >&2
        fi
    fi
    exit "$script_status"
}

validate_aws_inputs() {
    launch_template_id=$(printenv AWS_VALIDATION_LAUNCH_TEMPLATE_ID 2>/dev/null || true)
    template_version=$(printenv AWS_VALIDATION_LAUNCH_TEMPLATE_VERSION 2>/dev/null || true)
    [ -n "$launch_template_id" ] || { aws_input_status=fail; printf '%s\n' 'live AWS validation requires AWS_VALIDATION_LAUNCH_TEMPLATE_ID' >&2; return 1; }
    case "$launch_template_id" in lt-[A-Za-z0-9]*) ;; *) aws_input_status=fail; printf '%s\n' 'AWS launch template ID must be an EC2 lt-* identifier' >&2; return 1 ;; esac
    case "$template_version" in ''|*[!0-9]*) aws_input_status=fail; printf '%s\n' 'AWS_VALIDATION_LAUNCH_TEMPLATE_VERSION must be a pinned numeric version' >&2; return 1 ;; esac
    [ "$template_version" -gt 0 ] 2>/dev/null || { aws_input_status=fail; return 1; }
    template_image_id=$(aws ec2 describe-launch-template-versions --launch-template-id "$launch_template_id" --versions "$template_version" --query 'LaunchTemplateVersions[0].LaunchTemplateData.ImageId' --cli-read-timeout "$aws_cli_timeout_seconds" --cli-connect-timeout "$aws_cli_timeout_seconds" --output text 2>/dev/null) || { aws_input_status=fail; printf '%s\n' 'AWS launch template version could not be read' >&2; return 1; }
    case "$template_image_id" in ami-[A-Za-z0-9]*) ;; *) aws_input_status=fail; printf '%s\n' 'AWS launch template must resolve to an AMI image' >&2; return 1 ;; esac
    template_instance_type=$(aws ec2 describe-launch-template-versions --launch-template-id "$launch_template_id" --versions "$template_version" --query 'LaunchTemplateVersions[0].LaunchTemplateData.InstanceType' --cli-read-timeout "$aws_cli_timeout_seconds" --cli-connect-timeout "$aws_cli_timeout_seconds" --output text 2>/dev/null) || { aws_input_status=fail; printf '%s\n' 'AWS launch template instance type could not be read' >&2; return 1; }
    case "$template_instance_type" in t3.nano|t3.micro|t3.small|t4g.nano|t4g.micro|t4g.small) ;; *) aws_input_status=fail; printf '%s\n' 'AWS launch template instance type is outside the disposable validation allowlist' >&2; return 1 ;; esac
    aws_input_status=pass
    printf '%s\n' 'PASS aws-input        pinned disposable Launch Template inputs verified'
}

run_aws_live() {
    validate_aws_inputs || return 1
    token="leo-cloud-validation-$(date -u +%Y%m%d%H%M%S)-$$"
    output=$(aws ec2 run-instances \
        --launch-template "LaunchTemplateId=$launch_template_id,Version=$template_version" \
        --min-count 1 --max-count 1 --client-token "$token" \
        --tag-specifications 'ResourceType=instance,Tags=[{Key=Purpose,Value=leo-cloud-validation},{Key=Ephemeral,Value=true}]' \
        --cli-read-timeout "$aws_cli_timeout_seconds" --cli-connect-timeout "$aws_cli_timeout_seconds" \
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
    template=${GCP_VALIDATION_INSTANCE_TEMPLATE}
    case "$template" in
        ''|*[!A-Za-z0-9._-]*)
            gcp_input_status=fail
            printf '%s\n' 'GCP_VALIDATION_INSTANCE_TEMPLATE must be a safe pinned template name' >&2
            return 1
            ;;
    esac
    template_name=$(gcloud compute instance-templates describe "$template" --project "$project" \
        --format='value(name)' --request-timeout "$gcp_cli_timeout_seconds" 2>/dev/null) || {
        gcp_input_status=fail
        printf '%s\n' 'GCP instance template could not be read' >&2
        return 1
    }
    [ "$template_name" = "$template" ] || {
        gcp_input_status=fail
        printf '%s\n' 'GCP instance template identity did not match requested template' >&2
        return 1
    }
    template_image=$(gcloud compute instance-templates describe "$template" --project "$project" \
        --format='value(properties.disks[0].initializeParams.sourceImage)' \
        --request-timeout "$gcp_cli_timeout_seconds" 2>/dev/null) || {
        gcp_input_status=fail
        printf '%s\n' 'GCP instance template image could not be read' >&2
        return 1
    }
    case "$template_image" in
        */images/*) ;;
        *)
            gcp_input_status=fail
            printf '%s\n' 'GCP instance template must resolve to a pinned image reference' >&2
            return 1
            ;;
    esac
    gcp_input_status=pass
    gcp_instance_name="leo-cloud-validation-$(date -u +%Y%m%d%H%M%S)-$$"
    gcloud compute instances create "$gcp_instance_name" \
        --project "$project" --zone "$zone" \
        --source-instance-template "$template" \
        --labels purpose=leo-cloud-validation,ephemeral=true --quiet \
        --request-timeout "$gcp_operation_timeout_seconds" >/dev/null 2>&1 || {
        printf '%s\n' 'live GCP validation VM creation failed' >&2
        return 1
    }
    observed_purpose=$(gcloud compute instances describe "$gcp_instance_name" --project "$project" --zone "$zone" \
        --format='value(labels.purpose)' --request-timeout "$gcp_cli_timeout_seconds" 2>/dev/null) || {
        printf '%s\n' 'GCP validation VM labels could not be read' >&2
        return 1
    }
    observed_ephemeral=$(gcloud compute instances describe "$gcp_instance_name" --project "$project" --zone "$zone" \
        --format='value(labels.ephemeral)' --request-timeout "$gcp_cli_timeout_seconds" 2>/dev/null) || {
        printf '%s\n' 'GCP validation VM ephemeral label could not be read' >&2
        return 1
    }
    [ "$observed_purpose" = "$validation_tag_purpose" ] && [ "$observed_ephemeral" = "$validation_tag_ephemeral" ] || {
        printf '%s\n' 'GCP validation VM labels did not match the exact ephemeral contract' >&2
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

validate_timeouts
load_scope_file
check_required_env
has_provider aws && check_aws
has_provider gcp && check_gcp
has_provider github && check_github

if [ "$allow_live" -eq 0 ]; then
    [ "$live_action" = none ] || {
        printf '%s\n' 'live action requested without --allow-live; no resources were touched' >&2
        preflight_status=fail
        exit_class=invocation-error
        write_preflight_report || true
        exit 2
    }
    if [ "$overall_status" -eq 0 ]; then
        printf '%s\n' 'Cloud preflight passed in read-only mode; no resources were created or terminated.'
    else
        printf '%s\n' 'Cloud preflight failed in read-only mode; no resources were created or terminated.' >&2
    fi
    write_preflight_report || {
        exit_class=report-failure
        exit 1
    }
    exit "$overall_status"
fi

[ "$overall_status" -eq 0 ] || {
    printf '%s\n' 'live validation blocked by failed preflight; no resources were touched' >&2
    exit_class=preflight-failure
    write_preflight_report || true
    exit 1
}

[ "$supplied_confirmation" = "$confirmation" ] || {
    printf '%s\n' 'live validation requires --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES; no resources were touched' >&2
    exit_class=invocation-error
    write_preflight_report || true
    exit 2
}
[ -n "$scope_file" ] && [ "$scope_status" = pass ] || {
    printf '%s\n' 'live validation requires a reviewed scope file; no resources were touched' >&2
    exit_class=invocation-error
    write_preflight_report || true
    exit 2
}
case "$live_action" in
    aws-ec2)
        [ -n "$scope_aws_account" ] && [ -n "$scope_aws_region" ] || { printf '%s\n' 'AWS live validation requires aws_account_id and aws_region in the reviewed scope file' >&2; exit 2; }
        ;;
    gcp-vm)
        [ -n "$scope_gcp_project" ] && [ -n "$scope_gcp_zone" ] || { printf '%s\n' 'GCP live validation requires gcp_project and gcp_zone in the reviewed scope file' >&2; exit 2; }
        ;;
    github-jit)
        [ -n "$scope_github_repository" ] || { printf '%s\n' 'GitHub live validation requires github_repository in the reviewed scope file' >&2; exit 2; }
        ;;
esac
[ "$live_action" != none ] || {
    printf '%s\n' 'live validation requires an explicit --live-action; no resources were touched' >&2
    exit_class=invocation-error
    write_preflight_report || true
    exit 2
}

mode=live
validation_started_at=$(now_iso)
validation_started_epoch=$(now_epoch)
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
