#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
report_path=${RELEASE_VALIDATION_REPORT:-}
format=text

usage() {
    cat <<'USAGE'
Usage: validate.sh [--report PATH] [--format text|json]
Runs offline release checks only; no live provisioning action is permitted.
Exit status: 0 all pass, 1 failure, 2 warnings only.
USAGE
}
while [ "$#" -gt 0 ]; do
    case "$1" in
        --report) [ "$#" -ge 2 ] || { echo '--report requires a path' >&2; exit 2; }; report_path=$2; shift ;;
        --format) [ "$#" -ge 2 ] || { echo '--format requires a value' >&2; exit 2; }; format=$2; shift ;;
        --help|-h) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
    shift
done
case "$format" in text|json) ;; *) echo "unsupported format: $format" >&2; exit 2 ;; esac

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/leo-release-validation.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
results_file="$tmp_dir/results.tsv"
: >"$results_file"
overall=0
warning_count=0
failure_count=0

redact() {
    sed -E \
      -e 's/(Bearer[[:space:]]+)[^[:space:]]+/\1[REDACTED]/Ig' \
      -e 's/\bgh[pousr]_[A-Za-z0-9_]+\b/[REDACTED]/g' \
      -e 's/\bAKIA[0-9A-Z]{16}\b/[REDACTED]/g' \
      -e 's/(BEGIN[[:space:]][A-Z ]*PRIVATE KEY)/[REDACTED PRIVATE KEY]/g' \
      -e 's/((password|secret|token|access[_-]?key)[[:space:]]*[:=][[:space:]]*)[^[:space:]]+/\1[REDACTED]/Ig' \
      -e 's/[A-Za-z0-9+\/_=-]{32,}/[REDACTED]/g'
}

record() {
    name=$1; status=$2; code=$3; message=$4
    printf '%s\t%s\t%s\t%s\n' "$name" "$status" "$code" "$message" >>"$results_file"
    case "$status" in
      PASS) printf 'PASS %-18s %s\n' "$name" "$message" ;;
      WARN) printf 'WARN %-18s %s\n' "$name" "$message" >&2; warning_count=$((warning_count + 1)); [ "$overall" -eq 0 ] && overall=2 ;;
      FAIL) printf 'FAIL %-18s %s\n' "$name" "$message" >&2; failure_count=$((failure_count + 1)); overall=1 ;;
    esac
}

run_suite() {
    name=$1; shift; log="$tmp_dir/$name.log"
    if "$@" >"$log" 2>&1; then
        message=$(tail -n 1 "$log" 2>/dev/null | redact | tr '\t\r\n' '   ')
        record "$name" PASS 0 "${message:-offline suite passed}"; return
    fi
    code=$?
    output=$(cat "$log" | redact | tr '\t\r\n' '   ' | cut -c1-240)
    if grep -Eiq 'unavailable|not installed|not initialized|daemon is unavailable|skipped|blocked|require.*(go|docker|kubectl|terraform|ruby)' "$log"; then
        record "$name" WARN "$code" "${output:-offline prerequisite unavailable}"
    else
        record "$name" FAIL "$code" "${output:-offline suite failed}"
    fi
}

run_controller() {
    command -v go >/dev/null 2>&1 || { echo 'Go is not installed'; return 2; }
    (cd "$repo_root/controller" && go test ./... && go vet ./... && go build ./...)
}
run_terraform() {
    "$repo_root/terraform/environments/dev/validate.sh"
    "$repo_root/terraform/modules/controller-iam/validate.sh"
    "$repo_root/terraform/modules/observability/validate.sh"
    "$repo_root/terraform/modules/runner-network/validate.sh"
}

run_suite controller run_controller
run_suite extensions "$repo_root/tools/extension-validation/test.sh"
run_suite observability "$repo_root/tools/observability-validation/test.sh"
run_suite evidence "$repo_root/tools/evidence-validation/test.sh"
run_suite cloud-fixtures "$repo_root/tools/cloud-validation/test.sh"
run_suite docker "$repo_root/tools/docker-validation/test.sh"
run_suite kubernetes "$repo_root/deploy/kubernetes/test.sh"
run_suite iam "$repo_root/tools/iam-validation/test.sh"
run_suite terraform-static run_terraform

if grep -Eiq '(run-instances|instances create|generate-jitconfig|terraform apply|kubectl apply)' "$tmp_dir"/*.log 2>/dev/null; then
    record live-provisioning FAIL 1 'live provisioning command appeared in offline suite output'
else
    record live-provisioning PASS 0 'no live provisioning command was invoked by the release gate'
fi

status_name() { [ "$overall" -eq 1 ] && printf fail || [ "$overall" -eq 2 ] && printf warn || printf pass; }

if [ -n "$report_path" ]; then
    mkdir -p "$(dirname -- "$report_path")"
    ruby -rjson - "$results_file" "$report_path" "$overall" "$warning_count" "$failure_count" <<'RUBY'
results_path, destination, overall, warnings, failures = ARGV
checks = File.readlines(results_path, chomp: true).map do |line|
  name, status, exit_code, message = line.split("\t", 4)
  next if name.nil? || name.empty?
  {"name" => name, "status" => status, "exit_code" => Integer(exit_code), "message" => message.to_s}
end.compact
status = Integer(overall) == 1 ? "fail" : (Integer(overall) == 2 ? "warn" : "pass")
document = {"apiVersion" => "release-validation.leorunners.io/v1", "kind" => "ReleaseValidationReport", "metadata" => {"name" => "leo-runners-release", "version" => "1.0.0"}, "spec" => {"status" => status, "read_only" => true, "live_provisioning_invoked" => false, "checks" => checks, "summary" => {"warnings" => Integer(warnings), "failures" => Integer(failures)}}}
tmp = "#{destination}.tmp.#{$$}"
File.open(tmp, "w", 0o600) { |file| file.write(JSON.pretty_generate(document) + "\n") }
File.rename(tmp, destination)
RUBY
    test -s "$report_path"
fi

if [ "$format" = json ]; then
    ruby -rjson - "$results_file" "$overall" "$warning_count" "$failure_count" <<'RUBY'
results_path, overall, warnings, failures = ARGV
checks = File.readlines(results_path, chomp: true).map { |line| n, s, e, m = line.split("\t", 4); {"name" => n, "status" => s, "exit_code" => Integer(e), "message" => m.to_s} }
status = Integer(overall) == 1 ? "fail" : (Integer(overall) == 2 ? "warn" : "pass")
puts JSON.pretty_generate({"status" => status, "read_only" => true, "live_provisioning_invoked" => false, "checks" => checks, "warnings" => Integer(warnings), "failures" => Integer(failures)})
RUBY
fi
printf 'release validation summary: status=%s warnings=%s failures=%s\n' "$(status_name)" "$warning_count" "$failure_count"
exit "$overall"
