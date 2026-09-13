#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
schema="$root/schema.json"
baseline="${1:-$root/baseline.json}"
candidate="${2:-$root/candidate.json}"
manifest="$root/../workloads/workload-manifest.v1.yaml"

command -v jq >/dev/null 2>&1 || { echo "validate.sh: jq is required" >&2; exit 2; }
[ -f "$manifest" ] || { echo "workload manifest not found: $manifest" >&2; exit 2; }
manifest_digest="sha256:$(shasum -a 256 "$manifest" | awk '{print $1}')"

for file in "$schema" "$baseline" "$candidate"; do
  jq empty "$file" >/dev/null || { echo "invalid JSON: $file" >&2; exit 2; }
done

check_record() {
  file=$1
  jq -e --arg manifest_digest "$manifest_digest" '
    .schema_version == "phase-38.v1" and
    (.role == "baseline" or .role == "candidate") and
    (.comparison.comparison_id | type == "string" and length > 0) and
    (.comparison.pair_key | type == "string" and length > 0) and
    (.comparison.max_duration_regression_percent >= 0) and
    (.comparison.max_cost_regression_percent >= 0) and
    (.provenance.collected_at | type == "string") and
    (.provenance.collector | type == "string" and length > 0) and
    (.provenance.source | IN("offline-fixture", "controlled-run", "aws-run", "github-run")) and
    (.provenance.evidence_level | IN("fixture", "measured", "attested")) and
    (.provenance.execution_class | IN("synthetic", "real")) and
    (.workload.id | type == "string" and length > 0) and
    (.workload.manifest_version | test("^[0-9]+\\.[0-9]+\\.[0-9]+$")) and
    (.workload.manifest_digest | test("^sha256:[0-9a-f]{64}$")) and
    (.workload.manifest_digest == $manifest_digest) and
    (.workload.repository == "github.com/manish96170/leoRunners") and
    (.workload.commit | test("^[0-9a-f]{40}$")) and
    (.workload.workflow | test("^(?!/|.*\\.\\.).+$")) and
    (.workload.command_digest | test("^sha256:[0-9a-f]{64}$")) and
    (.identity.workload_id == .workload.id) and
    (.identity.workload_version == .workload.manifest_version) and
    (.identity.workload_commit == .workload.commit) and
    (.identity.image_digest | test("^sha256:[0-9a-f]{64}$")) and
    (.identity.region | test("^[a-z]{2}-[a-z]+-[0-9]+$")) and
    (.identity.provider | IN("aws", "gcp", "offline")) and
    (.comparability.repetitions >= 1) and
    (.comparability.warmup_runs >= 0) and
    (.comparability.toolchain_digest | test("^sha256:[0-9a-f]{64}$")) and
    (.redaction.status == "redacted") and
    (.redaction.secrets_scanned == true) and
    (.redaction.raw_payloads_excluded == true) and
    (.timing.unit == "milliseconds") and
    (([.timing.phases[].name] | sort) == ["boot", "cleanup", "job_execution", "provision", "queue_to_dispatch", "registration", "total"]) and
    (.timing.total_ms == ([.timing.phases[] | select(.name != "total") | .duration_ms] | add)) and
    (.cost.currency == "USD") and
    (.cost.total_usd == (.cost.compute_usd + .cost.storage_usd + .cost.network_usd)) and
    (.cost.pricing_source | IN("fixture", "aws-pricing-api", "invoice")) and
    (.result.status | IN("passed", "failed", "cancelled", "timed_out")) and
    (.result.reproducible | type == "boolean") and
    ((tostring | test("(?i)(AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|bearer[[:space:]]+[A-Za-z0-9._-]+|password[=:][^[:space:]]+|secret[=:][^[:space:]]+)") | not))
  ' "$file" >/dev/null || { echo "record validation failed: $file" >&2; exit 1; }
}

check_record "$baseline"
check_record "$candidate"

jq -e -s '
  (.[0].role == "baseline") and (.[1].role == "candidate") and
  (.[0].schema_version == .[1].schema_version) and
  (.[0].comparison.comparison_id == .[1].comparison.comparison_id) and
  (.[0].comparison.pair_key == .[1].comparison.pair_key) and
  (.[0].workload == .[1].workload) and
  (.[0].provenance.execution_class == .[1].provenance.execution_class) and
  (.[0].comparability == .[1].comparability) and
  (.[0].identity.region == .[1].identity.region) and
  (.[0].identity.architecture == .[1].identity.architecture) and
  (.[0].identity.provider == .[1].identity.provider) and
  (.[0].cost.currency == .[1].cost.currency) and
  (.[0].cost.pricing_source == .[1].cost.pricing_source) and
  (.[0].timing.phases | length == 7) and
  ((.[1].timing.total_ms - .[0].timing.total_ms) / ([.[0].timing.total_ms, 1] | max) * 100 <= .[1].comparison.max_duration_regression_percent) and
  ((.[1].cost.total_usd - .[0].cost.total_usd) / ([.[0].cost.total_usd, 0.000001] | max) * 100 <= .[1].comparison.max_cost_regression_percent)
' "$baseline" "$candidate" >/dev/null || { echo "benchmark comparison is not comparable or exceeds regression/cost limits" >&2; exit 1; }

echo "valid benchmark evidence: $(basename "$baseline") and $(basename "$candidate")"
