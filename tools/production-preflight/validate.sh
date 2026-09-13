#!/bin/sh
set -u

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
cloud_validator="$repo_root/tools/cloud-validation/validate.sh"
request_path=
scope_path=
report_path=
cloud_report_path=
provider=
request_version=
request_mode=
request_live_action=
request_allow_live=
request_confirmation=
request_owner=
overall_status=0
request_status=blocked
cloud_status=not-run
status=BLOCKED

usage() {
    cat <<'USAGE'
Usage: validate.sh --request PATH --scope-file PATH [--report PATH]

Validate a production activation request and run cloud validation in read-only
preflight mode. This wrapper never forwards live-action, allow-live,
confirmation, or evidence-report options.
USAGE
}

die_usage() {
    printf 'production preflight invocation error: %s\n' "$1" >&2
    usage >&2
    exit 2
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --request)
            [ "$#" -ge 2 ] || die_usage '--request needs a path'
            request_path=$2
            shift
            ;;
        --scope-file)
            [ "$#" -ge 2 ] || die_usage '--scope-file needs a path'
            scope_path=$2
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

valid_value() {
    case "$1" in
        ''|*[!A-Za-z0-9_./:@+-]*) return 1 ;;
    esac
    return 0
}

redact() {
    sed -E \
        -e 's/(AKIA[0-9A-Z]{16})/[REDACTED_AWS_KEY]/g' \
        -e 's/(gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|Bearer[[:space:]]+[^[:space:]]+)/[REDACTED_TOKEN]/g' \
        -e 's/(token|secret|password|authorization)[=:][^[:space:],}]*/\1=[REDACTED]/Ig'
}

load_request() {
    [ -n "$request_path" ] || { printf '%s\n' 'BLOCKED request      --request is required' >&2; return 1; }
    [ -f "$request_path" ] || { printf '%s\n' 'BLOCKED request      request file does not exist' >&2; return 1; }
    request_key_count=0
    while IFS= read -r request_line || [ -n "$request_line" ]; do
        case "$request_line" in
            ''|'#'*) continue ;;
            *=*) ;;
            *) printf '%s\n' 'BLOCKED request      request must contain key=value lines' >&2; overall_status=1; continue ;;
        esac
        request_key=$(printf '%s\n' "$request_line" | sed 's/=.*//')
        request_value=$(printf '%s\n' "$request_line" | sed 's/^[^=]*=//')
        request_key_count=$((request_key_count + 1))
        valid_value "$request_value" || { printf 'BLOCKED request      unsafe value for %s\n' "$request_key" >&2; overall_status=1; continue; }
        case "$request_key" in
            request_version) [ -z "$request_version" ] && request_version=$request_value || { printf '%s\n' 'BLOCKED request      duplicate request_version' >&2; overall_status=1; } ;;
            mode) [ -z "$request_mode" ] && request_mode=$request_value || { printf '%s\n' 'BLOCKED request      duplicate mode' >&2; overall_status=1; } ;;
            provider) [ -z "$provider" ] && provider=$request_value || { printf '%s\n' 'BLOCKED request      duplicate provider' >&2; overall_status=1; } ;;
            live_action) [ -z "$request_live_action" ] && request_live_action=$request_value || { printf '%s\n' 'BLOCKED request      duplicate live_action' >&2; overall_status=1; } ;;
            allow_live) [ -z "$request_allow_live" ] && request_allow_live=$request_value || { printf '%s\n' 'BLOCKED request      duplicate allow_live' >&2; overall_status=1; } ;;
            confirmation) [ -z "$request_confirmation" ] && request_confirmation=$request_value || { printf '%s\n' 'BLOCKED request      duplicate confirmation' >&2; overall_status=1; } ;;
            owner) [ -z "$request_owner" ] && request_owner=$request_value || { printf '%s\n' 'BLOCKED request      duplicate owner' >&2; overall_status=1; } ;;
            *) printf 'BLOCKED request      unsupported key: %s\n' "$request_key" >&2; overall_status=1 ;;
        esac
    done <"$request_path"
    [ "$request_key_count" -gt 0 ] || { printf '%s\n' 'BLOCKED request      request is empty' >&2; overall_status=1; }
    [ "$request_version" = production-preflight.v1 ] || { printf '%s\n' 'BLOCKED request      request_version must be production-preflight.v1' >&2; overall_status=1; }
    [ "$request_mode" = preflight ] || { printf '%s\n' 'BLOCKED request      mode must be preflight' >&2; overall_status=1; }
    case "$provider" in aws|gcp|github|all) ;; *) printf '%s\n' 'BLOCKED request      provider must be aws, gcp, github, or all' >&2; overall_status=1 ;; esac
    [ "$request_live_action" = none ] || { printf '%s\n' 'BLOCKED request      live_action must be none' >&2; overall_status=1; }
    [ "$request_allow_live" = false ] || { printf '%s\n' 'BLOCKED request      allow_live must be false' >&2; overall_status=1; }
    [ "$request_confirmation" = none ] || { printf '%s\n' 'BLOCKED request      confirmation must be none' >&2; overall_status=1; }
    [ -n "$request_owner" ] || { printf '%s\n' 'BLOCKED request      owner is required' >&2; overall_status=1; }
    [ -n "$scope_path" ] || { printf '%s\n' 'BLOCKED scope        --scope-file is required' >&2; overall_status=1; }
    [ -f "$scope_path" ] || { printf '%s\n' 'BLOCKED scope        reviewed scope file does not exist' >&2; overall_status=1; }
    request_status=pass
    [ "$overall_status" -eq 0 ] || request_status=fail
}

write_report() {
    [ -n "$report_path" ] || return 0
    report_dir=$(dirname -- "$report_path")
    [ -d "$report_dir" ] || { printf '%s\n' 'BLOCKED report       report directory does not exist' >&2; return 1; }
    report_tmp=$(mktemp "$report_dir/.production-preflight.XXXXXX") || return 1
    chmod 600 "$report_tmp" 2>/dev/null || true
    export REPORT_STATUS="$status" REPORT_REQUEST_STATUS="$request_status"
    export REPORT_CLOUD_STATUS="$cloud_status" REPORT_PROVIDER="$provider"
    export REPORT_OWNER="$request_owner" REPORT_SCOPE="$scope_path"
    export REPORT_CLOUD_REPORT="$cloud_report_path" REPORT_TRANSCRIPT="$transcript_path"
    ruby -rjson - "$report_tmp" <<'RUBY'
destination = ARGV.fetch(0)
redact = lambda do |value|
  value.to_s.gsub(/AKIA[0-9A-Z]{16}/, "[REDACTED_AWS_KEY]")
    .gsub(/gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+/, "[REDACTED_TOKEN]")
    .gsub(/(token|secret|password|authorization)[=:][^\s,}]+/i, '\1=[REDACTED]')
end
transcript = File.exist?(ENV.fetch("REPORT_TRANSCRIPT")) ? File.readlines(ENV.fetch("REPORT_TRANSCRIPT"), chomp: true).map { |line| redact.call(line) }.last(80) : []
cloud = {}
cloud_path = ENV.fetch("REPORT_CLOUD_REPORT")
if File.exist?(cloud_path)
  begin
    cloud = JSON.parse(File.read(cloud_path))
    cloud["identity"] = "withheld" if cloud.is_a?(Hash)
  rescue JSON::ParserError
    cloud = {"parse_status" => "unavailable"}
  end
end
document = {
  "apiVersion" => "preflight.leorunners.io/v1",
  "kind" => "ProductionPreflightHandoff",
  "metadata" => {"name" => "leo-runners-production-preflight", "version" => "1.0.0"},
  "spec" => {
    "status" => ENV.fetch("REPORT_STATUS"),
    "read_only" => true,
    "request" => {"status" => ENV.fetch("REPORT_REQUEST_STATUS"), "provider" => ENV.fetch("REPORT_PROVIDER"), "owner" => redact.call(ENV.fetch("REPORT_OWNER"))},
    "scope" => {"provided" => !ENV.fetch("REPORT_SCOPE").empty?, "values" => "withheld"},
    "cloud_validation" => {"status" => ENV.fetch("REPORT_CLOUD_STATUS"), "report" => cloud},
    "transcript" => transcript,
    "mutation_guard" => {"live_flags_forwarded" => false, "resources_created" => false, "resources_terminated" => false},
    "redaction" => "tokens, secrets, credentials, scope values, and provider response bodies are withheld"
  }
}
File.open(destination, "w", 0600) { |file| file.write(JSON.pretty_generate(document) + "\n") }
RUBY
    mv "$report_tmp" "$report_path"
}

tmp=$(mktemp -d "/tmp/leo-production-preflight.XXXXXX")
transcript_path="$tmp/transcript"
cloud_report_path="$tmp/cloud-report.json"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
: >"$transcript_path"

load_request
if [ "$request_status" = pass ]; then
    if "$cloud_validator" --provider "$provider" --scope-file "$scope_path" --report "$cloud_report_path" >"$transcript_path" 2>&1; then
        cloud_status=pass
        status=PASS
    else
        cloud_status=blocked
        status=BLOCKED
    fi
fi

redacted_transcript=$(redact <"$transcript_path")
printf '%s\n' "$redacted_transcript"
printf '%s\n' "$status production-preflight  read-only activation handoff" >&2
write_report || { status=BLOCKED; exit 1; }
[ "$status" = PASS ] && exit 0
exit 1
