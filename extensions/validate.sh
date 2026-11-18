#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
schema="$root/schema.v1.json"
command -v ruby >/dev/null 2>&1 || { echo "validate.sh: ruby is required" >&2; exit 2; }
[ -f "$schema" ] || { echo "missing schema: $schema" >&2; exit 2; }

if [ "$#" -eq 0 ]; then
  set -- "$root"/analytics-*.v1.json "$root"/cache-*.v1.json "$root"/security-*.v1.json "$root"/intelligence-*.v1.json
fi

for file in "$schema" "$@"; do
  [ -f "$file" ] || { echo "file not found: $file" >&2; exit 2; }
done

ruby -rjson -rdate - "$schema" "$@" <<'RUBY'
schema_path = ARGV.shift
paths = ARGV
schema = JSON.parse(File.read(schema_path))
raise "schema must be draft 2020-12" unless schema["$schema"].to_s.include?("draft/2020-12")
raise "schema must identify extensions v1" unless schema["$id"].to_s.end_with?("extensions.v1.json")
types = %w[analytics cache security intelligence]
seen = {}
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
control = /\b(?:provision|terminate|cancel|retry|schedule|assign|authorize|credential|grant|release)\b/i

paths.each do |path|
  record = JSON.parse(File.read(path))
  raise "#{path}: record must be an object" unless record.is_a?(Hash)
  raise "#{path}: invalid apiVersion or kind" unless record["apiVersion"] == "extensions.leorunners.io/v1" && record["kind"] == "ExtensionRecord"
  id = record["extensionId"]
  raise "#{path}: invalid or duplicate extensionId" unless id.is_a?(String) && id.match?(/\A[a-z][a-z0-9_.-]{2,63}\z/) && !seen.key?(id)
  seen[id] = true
  type = record["extensionType"]
  raise "#{path}: unsupported extensionType" unless types.include?(type)
  emitted = DateTime.iso8601(record.fetch("emittedAt")) rescue (raise "#{path}: emittedAt must be ISO-8601")
  subject = record["subject"]
  raise "#{path}: tenantId and jobId are required" unless subject.is_a?(Hash) && subject["tenantId"].is_a?(String) && subject["jobId"].is_a?(String)
  retention = record["retention"]
  raise "#{path}: retention must be redacted with evidenceClass" unless retention.is_a?(Hash) && retention["redacted"] == true && retention["evidenceClass"].is_a?(String)
  expires = DateTime.iso8601(retention.fetch("expiresAt")) rescue (raise "#{path}: expiresAt must be ISO-8601")
  raise "#{path}: retention must expire after emittedAt" unless expires > emitted
  metadata = record["metadata"]
  raise "#{path}: invalid metadata" unless metadata.is_a?(Hash) && metadata["schemaVersion"] == "1" && metadata["source"].is_a?(String) && metadata["evidence"].is_a?(String) && metadata["values"].is_a?(Hash)
  raise "#{path}: metadata values are too large" if metadata["values"].length > 24
  raise "#{path}: metadata values must be scalar" unless metadata["values"].all? { |key, value| key.match?(/\A[a-z][a-z0-9_.-]{0,63}\z/) && [String, Integer, Float, TrueClass, FalseClass, NilClass].include?(value.class) }
  advisory = record["advisory"]
  raise "#{path}: invalid advisory" unless advisory.is_a?(Hash) && %w[none informational review blocked].include?(advisory["status"]) && %w[none low medium high].include?(advisory["severity"])
  raise "#{path}: advisory summary must be bounded" unless advisory["summary"].is_a?(String) && advisory["summary"].length.between?(1, 512)
  raise "#{path}: confidence must be between 0 and 1" unless advisory["confidence"].is_a?(Numeric) && advisory["confidence"].between?(0, 1)
  raise "#{path}: too many recommendations" unless advisory["recommendations"].is_a?(Array) && advisory["recommendations"].length <= 8 && advisory["recommendations"].all? { |item| item.is_a?(String) && item.length <= 256 }
  raise "#{path}: annotations must be bounded scalars" unless advisory["annotations"].is_a?(Hash) && advisory["annotations"].length <= 16 && advisory["annotations"].all? { |key, value| key.match?(/\A[a-z][a-z0-9_.-]{0,63}\z/) && [String, Integer, Float, TrueClass, FalseClass, NilClass].include?(value.class) }
  serialized = JSON.generate(record)
  raise "#{path}: secret-shaped content found" if serialized.match?(secret)
  raise "#{path}: lifecycle or authorization command found" if serialized.match?(control)
  raise "#{path}: raw logs or environment are forbidden" if serialized.match?(/"(?:logs?|stdout|stderr|environment|env|prompt)"\s*:/i)
  puts "valid extension record: #{path} (#{type})"
end
puts "all extension fixtures are valid"
RUBY
