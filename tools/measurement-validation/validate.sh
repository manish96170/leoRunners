#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
schema="$root/measurements/schema.v1.json"
default="$root/measurements/evidence.v1.json"
file="$default"
other=""
[ "$#" -ge 1 ] && file=$1
[ "$#" -ge 2 ] && other=$2
command -v jq >/dev/null 2>&1 || { echo "validate.sh: jq is required" >&2; exit 2; }
[ -f "$schema" ] || { echo "schema not found: $schema" >&2; exit 2; }
[ -f "$file" ] || { echo "evidence not found: $file" >&2; exit 2; }
jq empty "$schema" >/dev/null || { echo "invalid schema JSON" >&2; exit 2; }
jq empty "$file" >/dev/null || { echo "invalid evidence JSON: $file" >&2; exit 2; }

check_record() {
  record=$1
  jq -e '
    .apiVersion == "measurements.leorunners.io/v1" and .kind == "MeasurementEvidence" and
    (.metadata.name | type == "string" and test("^[a-z][a-z0-9.-]{2,63}$")) and
    (.metadata.version | test("^[0-9]+\\.[0-9]+\\.[0-9]+$")) and
    (.spec.measurement_id | type == "string" and length >= 3) and
    (.spec.provenance.source | IN("offline-fixture", "controlled-run", "aws-run", "gcp-run")) and
    (.spec.provenance.evidence_level | IN("fixture", "measured", "attested")) and
    (.spec.provenance.execution_class | IN("synthetic", "real")) and
    (.spec.provenance.collector | type == "string" and length > 0) and
    (.spec.provenance.collector_version | test("^[0-9]+\\.[0-9]+\\.[0-9]+$")) and
    ((.spec.provenance.execution_class == "synthetic") == (.spec.provenance.evidence_level == "fixture")) and
    ((.spec.provenance.execution_class == "synthetic") == (.spec.provenance.source == "offline-fixture")) and
    ((.spec.provenance.execution_class == "real") == (.spec.provenance.source != "offline-fixture")) and
    ((.spec.provenance.execution_class == "synthetic") == (.spec.comparability.pricing_source == "fixture")) and
    (.spec.identity.workload_commit | test("^[0-9a-f]{40}$")) and
    (.spec.identity.provider | IN("aws", "gcp", "offline")) and
    (.spec.comparability.sample_count >= 1) and (.spec.comparability.warmup_count >= 0) and
    (.spec.comparability.unit_system == "si") and
    (.spec.cache.namespace.tenant_ref | test("^ref:[a-z0-9-]{3,64}$")) and
    (.spec.cache.namespace.namespace_ref | test("^ref:[a-z0-9-]{3,64}$")) and
    (.spec.cache.namespace.prefix_ref | test("^ref:[a-z0-9-]{3,128}$")) and
    (.spec.cache.operations.lookup_count == (.spec.cache.operations.hit_count + .spec.cache.operations.miss_count)) and
    (.spec.cache.hit_rate == (if .spec.cache.operations.lookup_count == 0 then 0 else (.spec.cache.operations.hit_count / .spec.cache.operations.lookup_count) end)) and
    (.spec.network.egress_mode | IN("nat", "vpc-endpoint", "hybrid", "approved-proxy")) and
    (.spec.network.nat_gateway.unit == "bytes" and .spec.network.vpc_endpoint.unit == "bytes" and .spec.network.egress.unit == "bytes") and
    (.spec.network.nat_gateway.bytes >= 0 and .spec.network.vpc_endpoint.bytes >= 0 and .spec.network.egress.bytes >= 0) and
    (.spec.startup.unit == "milliseconds") and
    ([.spec.startup.phases[].name] | index("total")) != null and
    (.spec.startup.total_ms == ([.spec.startup.phases[] | select(.name == "total") | .duration_ms] | first)) and
    (.spec.cost.currency == "USD") and
    (.spec.cost.total_usd == (.spec.cost.compute_usd + .spec.cost.storage_usd + .spec.cost.network_usd)) and
    (.spec.cost.pricing_as_of | type == "string" and length > 0) and
    (.spec.redaction.status == "redacted") and (.spec.redaction.secrets_scanned == true) and
    (.spec.redaction.raw_payloads_excluded == true) and (.spec.redaction.identifiers_hashed == true) and
    ((tostring | test("(?i)(AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|bearer[[:space:]]+[A-Za-z0-9._-]+|password[=:][^[:space:]]+|secret[=:][^[:space:]]+|token[=:][^[:space:]]+|aws_access_key|private_key)") | not))
  ' "$record" >/dev/null
}

if ! check_record "$file"; then
  echo "measurement evidence validation failed: $file" >&2
  exit 1
fi

if [ -n "$other" ]; then
  [ -f "$other" ] || { echo "comparison evidence not found: $other" >&2; exit 2; }
  jq empty "$other" >/dev/null || { echo "invalid comparison evidence JSON: $other" >&2; exit 2; }
  if ! check_record "$other"; then
    echo "comparison evidence validation failed: $other" >&2
    exit 1
  fi
  jq -e -s '
    .[0].spec.provenance.execution_class == .[1].spec.provenance.execution_class and
    .[0].spec.comparability.comparison_id == .[1].spec.comparability.comparison_id and
    .[0].spec.comparability.protocol_id == .[1].spec.comparability.protocol_id and
    .[0].spec.comparability.comparison_key == .[1].spec.comparability.comparison_key and
    .[0].spec.identity.workload_id == .[1].spec.identity.workload_id and
    .[0].spec.identity.workload_version == .[1].spec.identity.workload_version and
    .[0].spec.identity.workload_commit == .[1].spec.identity.workload_commit and
    .[0].spec.identity.architecture == .[1].spec.identity.architecture and
    .[0].spec.identity.provider == .[1].spec.identity.provider and
    .[0].spec.identity.region == .[1].spec.identity.region and
    .[0].spec.identity.cache_mode == .[1].spec.identity.cache_mode and
    .[0].spec.identity.network_profile == .[1].spec.identity.network_profile and
    .[0].spec.comparability.pricing_source == .[1].spec.comparability.pricing_source
  ' "$file" "$other" >/dev/null || {
    echo "measurement records are not comparable" >&2
    exit 1
  }
  echo "comparable measurement evidence: $(basename "$file") and $(basename "$other") (offline)"
else
  echo "valid measurement evidence: $(basename "$file") (offline, no cloud calls)"
fi
