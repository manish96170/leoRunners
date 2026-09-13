#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
if [ "$#" -gt 0 ]; then
  files="$*"
else
  files="$root/intelligence/evaluation-synthetic.v1.json $root/intelligence/evaluation-real.v1.json"
fi

command -v ruby >/dev/null 2>&1 || { echo "ruby is required" >&2; exit 2; }

for file in $files; do
  [ -f "$file" ] || { echo "file not found: $file" >&2; exit 2; }
  ruby -rjson - "$file" <<'RUBY'
path = ARGV.fetch(0)
data = JSON.parse(File.read(path))
fail "fixture must be an object" unless data.is_a?(Hash)
fail "invalid version" unless data["version"] == "intelligence-evaluation.v1"
fail "dataset_id is invalid" unless data["dataset_id"].is_a?(String) && data["dataset_id"].length.between?(1, 128)
fail "invalid provenance" unless %w[synthetic real].include?(data["provenance"])
fail "AI must be disabled" unless data["ai_enabled"] == false
fail "policy version is required" unless data["policy_version"].is_a?(String) && data["policy_version"].length.between?(1, 64)
telemetry = data["telemetry"]
cases = data["cases"]
fail "telemetry is unbounded or empty" unless telemetry.is_a?(Array) && telemetry.length.between?(1, 10_000)
fail "cases are unbounded or empty" unless cases.is_a?(Array) && cases.length.between?(1, 1_000)
ids = telemetry.map { |sample| sample["id"] }
fail "telemetry ids must be unique and bounded" unless ids.all? { |id| id.is_a?(String) && id.length.between?(1, 128) } && ids.uniq.length == ids.length
cases.each do |entry|
  fail "case id is invalid" unless entry["id"].is_a?(String) && entry["id"].length.between?(1, 128)
  fail "duplicate case id" if cases.count { |other| other["id"] == entry["id"] } > 1
  fail "log is missing or unbounded" unless entry["log"].is_a?(String) && entry["log"].bytesize <= 1_048_576
  fail "max_log_bytes is invalid" unless entry["max_log_bytes"].is_a?(Integer) && entry["max_log_bytes"].between?(1, 262_144)
  fail "unsafe expected action" unless %w[none annotate retry rerun].include?(entry["expected_action"])
  response = entry["response"] || {}
  recommendations = response["recommendations"] || []
  fail "recommendations must be bounded" unless recommendations.is_a?(Array) && recommendations.length <= 8
  recommendations.each do |recommendation|
    fail "unsafe advisory action" unless %w[none annotate retry rerun].include?(recommendation["action"])
    fail "rationale is unbounded" unless recommendation["rationale"].to_s.bytesize <= 1024
  end
end
serialized = JSON.generate(data)
credential_pattern = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|(?:password|secret|access[_-]?key)\s*[:=]\s*\S+/i
fail "credential-shaped content found" if serialized.match?(credential_pattern)
puts "valid intelligence evaluation: #{path}"
RUBY
done

echo "all intelligence evaluation fixtures are valid"
