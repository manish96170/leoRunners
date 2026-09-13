#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
input=${1:-$root/../../measurements/phase-51-comparison.v1.json}
output=${2:-}
command -v ruby >/dev/null 2>&1 || { echo "validate.sh: ruby is required" >&2; exit 2; }
[ -f "$input" ] || { echo "comparison file not found: $input" >&2; exit 2; }

set +e
ruby -rjson - "$input" "$output" <<'RUBY'
input_path, output_path = ARGV
begin
raw = File.read(input_path)
raise "credential-shaped content found" if raw.match?(/Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i)
data = JSON.parse(raw)
fail "document must be an object" unless data.is_a?(Hash)
fail "unsupported schema version" unless data["schema_version"] == "phase-51.v1"
fail "comparison_id is invalid" unless data["comparison_id"].is_a?(String) && data["comparison_id"].match?(/\A[a-z0-9][a-z0-9._-]{2,127}\z/)
fail "scope must be bounded-evidence-only" unless data["claim_policy"] == "bounded-evidence-only"
fail "real-world claims must be disabled" unless data["real_world_claim"] == false

providers = data["expected_providers"]
fail "expected_providers must list aws and gcp exactly once" unless providers.is_a?(Array) && providers.sort == %w[aws gcp]
reference = data["reference_provider"]
fail "reference_provider must be aws or gcp" unless providers.include?(reference)
metric_names = %w[startup_ms lifecycle_ms cache_hit_rate_percent network_ms cost_usd]
fail "metrics must define the five comparison metrics" unless data["metrics"].is_a?(Array) && data["metrics"].sort == metric_names.sort
thresholds = data["regression_thresholds_percent"]
fail "regression thresholds are invalid" unless thresholds.is_a?(Hash) && metric_names.all? { |name| thresholds[name].is_a?(Numeric) && thresholds[name] >= 0 && thresholds[name] <= 1000 }

provenance = data["provenance"]
fail "provenance is incomplete" unless provenance.is_a?(Hash) && %w[source evidence_level execution_class].all? { |key| provenance[key].is_a?(String) }
fail "unsupported execution class" unless %w[synthetic real].include?(provenance["execution_class"])
fail "unsupported evidence level" unless %w[fixture measured attested].include?(provenance["evidence_level"])
if provenance["execution_class"] == "synthetic"
  fail "synthetic comparison must use fixture evidence" unless provenance.values_at("source", "evidence_level") == %w[offline-fixture fixture]
else
  fail "real comparison requires measured or attested evidence" unless %w[measured attested].include?(provenance["evidence_level"]) && %w[controlled-run aws-run gcp-run].include?(provenance["source"])
end

samples = data["samples"]
fail "samples must be a bounded non-empty array" unless samples.is_a?(Array) && samples.length.between?(1, 200)
seen = {}
samples.each do |sample|
  fail "sample must be an object" unless sample.is_a?(Hash)
  fail "sample provider is invalid" unless providers.include?(sample["provider"])
  id = sample["sample_id"]
  fail "sample_id must be unique and bounded" unless id.is_a?(String) && id.match?(/\A[a-z0-9][a-z0-9._-]{2,127}\z/) && !seen.key?(id)
  seen[id] = true
  fail "sample provenance must match comparison provenance" unless sample["provenance"] == provenance
  values = sample["values"]
  fail "sample values are incomplete" unless values.is_a?(Hash) && metric_names.all? { |name| values[name].is_a?(Numeric) && values[name] >= 0 }
  fail "cache hit rate is out of bounds" unless values["cache_hit_rate_percent"] <= 100
end

def percentile(values, percentile)
  sorted = values.sort
  sorted[[((percentile * sorted.length).ceil - 1), 0].max]
end

def summary(values)
  { "count" => values.length, "min" => values.min, "median" => percentile(values, 0.5), "p95" => percentile(values, 0.95) }
end

by_provider = providers.to_h do |provider|
  rows = samples.select { |sample| sample["provider"] == provider }
  if rows.empty?
    [provider, { "status" => "unavailable", "sample_count" => 0, "confidence" => "none", "metrics" => {} }]
  else
    confidence = provenance["execution_class"] == "real" && rows.length >= 10 ? "high" : (rows.length >= 3 ? "medium" : "low")
    [provider, { "status" => "available", "sample_count" => rows.length, "confidence" => confidence, "metrics" => metric_names.to_h { |name| [name, summary(rows.map { |row| row["values"][name] })] } }]
  end
end

comparisons = {}
overall = "PASS"
if by_provider[reference]["status"] == "unavailable"
  overall = "WARN"
else
  metric_names.each do |name|
    comparisons[name] = {}
    reference_p95 = by_provider[reference]["metrics"][name]["p95"]
    providers.each do |provider|
      next if provider == reference
      result = by_provider[provider]
      if result["status"] == "unavailable"
        comparisons[name][provider] = { "status" => "unavailable" }
        next
      end
      candidate_p95 = result["metrics"][name]["p95"]
      change = if name == "cache_hit_rate_percent"
                 reference_p95.zero? ? 0.0 : ((reference_p95 - candidate_p95) / reference_p95.to_f) * 100
               else
                 reference_p95.zero? ? (candidate_p95.zero? ? 0.0 : 100.0) : ((candidate_p95 - reference_p95) / reference_p95.to_f) * 100
               end
      regression = change > thresholds[name]
      comparisons[name][provider] = { "status" => regression ? "FAIL" : "PASS", "reference_p95" => reference_p95, "candidate_p95" => candidate_p95, "regression_percent" => change.round(6), "threshold_percent" => thresholds[name] }
      overall = "FAIL" if regression
    end
  end
end

if overall == "PASS" && by_provider.values.any? { |result| result["status"] == "unavailable" }
  overall = "WARN"
end

report = {
  "schema_version" => "phase-51.report.v1", "comparison_id" => data["comparison_id"], "status" => overall,
  "scope" => "bounded-multi-cloud-evidence", "claim_policy" => "bounded-evidence-only", "real_world_claim" => false,
  "provenance" => provenance, "reference_provider" => reference, "providers" => by_provider, "comparisons" => comparisons,
  "notes" => ["Statistics use nearest-rank min, median, and p95 over supplied samples.", "Unavailable providers are not treated as zero and cannot support a provider comparison.", "This report is not a general real-world performance, reliability, or pricing claim."]
}
if output_path && !output_path.empty?
  tmp = "#{output_path}.tmp.#{$$}"
  File.open(tmp, "w", 0o600) { |file| file.write(JSON.pretty_generate(report) + "\n") }
  File.rename(tmp, output_path)
end
puts JSON.pretty_generate(report)
exit 1 if overall == "FAIL"
rescue StandardError => e
  warn "FAIL #{input_path}: #{e.message}"
  exit 1
end
RUBY
status=$?
set -e
exit "$status"
