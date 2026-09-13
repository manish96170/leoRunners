#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
schema="$root/schema.json"
baseline="${1:-$root/baseline.json}"
candidate="${2:-$root/candidate.json}"

command -v jq >/dev/null 2>&1 || { echo "validate.sh: jq is required" >&2; exit 2; }

for file in "$schema" "$baseline" "$candidate"; do
  jq empty "$file" >/dev/null || { echo "invalid JSON: $file" >&2; exit 2; }
done

check_record() {
  file=$1
  jq -e '
    .schema_version == "phase-7.v1" and
    (.role == "baseline" or .role == "candidate") and
    (.comparison.comparison_id | type == "string") and
    (.comparison.pair_key | type == "string" and length > 0) and
    (.provenance.collected_at | type == "string") and
    (.provenance.collector | type == "string" and length > 0) and
    (.provenance.source | IN("offline-fixture", "controlled-run", "aws-run", "github-run")) and
    (.provenance.evidence_level | IN("fixture", "measured", "attested")) and
    (.identity.workload_id | type == "string" and length > 0) and
    (.identity.workload_commit | test("^[0-9a-f]{40}$")) and
    (.identity.image_digest | test("^sha256:[0-9a-f]{64}$")) and
    (.identity.region | test("^[a-z]{2}-[a-z]+-[0-9]+$")) and
    (.timing.unit == "milliseconds") and
    (([.timing.phases[].name] | sort) == ["boot", "cleanup", "job_execution", "provision", "queue_to_dispatch", "registration", "total"]) and
    (.timing.total_ms == ([.timing.phases[] | select(.name != "total") | .duration_ms] | add)) and
    (.cost.currency == "USD") and
    (.cost.total_usd == (.cost.compute_usd + .cost.storage_usd + .cost.network_usd)) and
    (.result.status | IN("passed", "failed", "cancelled", "timed_out")) and
    (.result.reproducible | type == "boolean")
  ' "$file" >/dev/null || { echo "record validation failed: $file" >&2; exit 1; }
}

check_record "$baseline"
check_record "$candidate"

jq -e -s '
  (.[0].role == "baseline") and (.[1].role == "candidate") and
  (.[0].schema_version == .[1].schema_version) and
  (.[0].comparison.comparison_id == .[1].comparison.comparison_id) and
  (.[0].comparison.pair_key == .[1].comparison.pair_key) and
  (.[0].identity.workload_id == .[1].identity.workload_id) and
  (.[0].identity.workload_version == .[1].identity.workload_version) and
  (.[0].identity.workload_commit == .[1].identity.workload_commit) and
  (.[0].identity.region == .[1].identity.region) and
  (.[0].identity.architecture == .[1].identity.architecture) and
  (.[0].timing.phases | length == 7)
' "$baseline" "$candidate" >/dev/null || { echo "baseline/candidate comparison identity mismatch" >&2; exit 1; }

echo "valid benchmark evidence: $(basename "$baseline") and $(basename "$candidate")"
