#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
schema="$root/schema.v1.json"

command -v ruby >/dev/null 2>&1 || {
  echo "validate.sh: ruby is required" >&2
  exit 2
}

[ -f "$schema" ] || { echo "missing schema: $schema" >&2; exit 2; }

if [ "$#" -eq 0 ]; then
  set -- "$root"/lifecycle-*.v1.json
fi

for file in "$schema" "$@"; do
  [ -f "$file" ] || { echo "file not found: $file" >&2; exit 2; }
done

ruby -rjson - "$schema" "$@" <<'RUBY'
require "date"

schema_path = ARGV.shift
paths = ARGV
schema = JSON.parse(File.read(schema_path))
raise "schema must be a draft 2020-12 object" unless schema["$schema"].to_s.include?("draft/2020-12") && schema["type"] == "object"
raise "schema must identify v1" unless schema["$id"].to_s.end_with?("events.v1.json")

allowed_types = {
  "job.queued" => "queued", "job.started" => "running", "job.completed" => "completed",
  "job.failed" => "failed", "job.cancelled" => "cancelled", "runner.provisioning" => "provisioning",
  "runner.ready" => "ready", "runner.busy" => "busy", "runner.terminating" => "terminating",
  "runner.terminated" => "terminated", "runner.failed" => "failed"
}
secret_pattern = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
seen_ids = {}
seen_dedup = {}

paths.each do |path|
  event = JSON.parse(File.read(path))
  raise "#{path}: event must be an object" unless event.is_a?(Hash)
  raise "#{path}: invalid apiVersion" unless event["apiVersion"] == "events.leorunners.io/v1"
  raise "#{path}: invalid kind" unless event["kind"] == "LifecycleEvent"
  raise "#{path}: required envelope field missing" unless %w[eventId eventType occurredAt sequence source subject delivery retention data].all? { |key| event.key?(key) }
  id = event.fetch("eventId")
  raise "#{path}: invalid eventId" unless id.is_a?(String) && id.match?(/\A[a-zA-Z0-9][a-zA-Z0-9._:-]{2,127}\z/)
  raise "#{path}: duplicate eventId" if seen_ids.key?(id)
  seen_ids[id] = path
  type = event.fetch("eventType")
  raise "#{path}: unknown eventType" unless allowed_types.key?(type)
  status = event.dig("data", "status")
  raise "#{path}: event status does not match eventType" unless status == allowed_types[type]
  begin
    occurred_at = DateTime.iso8601(event.fetch("occurredAt"))
  rescue ArgumentError, TypeError
    raise "#{path}: occurredAt must be ISO-8601"
  end
  raise "#{path}: sequence must be a non-negative integer" unless event["sequence"].is_a?(Integer) && event["sequence"] >= 0
  source = event.fetch("source")
  raise "#{path}: invalid source" unless source.is_a?(Hash) && %w[component version].all? { |key| source[key].is_a?(String) && !source[key].empty? }
  subject = event.fetch("subject")
  raise "#{path}: tenantId and jobId are required" unless subject.is_a?(Hash) && %w[tenantId jobId].all? { |key| subject[key].is_a?(String) && !subject[key].empty? }
  delivery = event.fetch("delivery")
  raise "#{path}: invalid delivery" unless delivery.is_a?(Hash) && delivery["deduplicationKey"].is_a?(String) && delivery["replayKey"].is_a?(String) && delivery["attempt"].is_a?(Integer) && delivery["attempt"] >= 1
  dedup = delivery.fetch("deduplicationKey")
  raise "#{path}: duplicate deduplicationKey" if seen_dedup.key?(dedup)
  seen_dedup[dedup] = path
  raise "#{path}: replayKey must include job identity" unless delivery.fetch("replayKey").include?(subject.fetch("jobId"))
  retention = event.fetch("retention")
  raise "#{path}: retention must be redacted with expiry" unless retention.is_a?(Hash) && retention["redacted"] == true && retention["expiresAt"].is_a?(String) && !retention["expiresAt"].empty?
  begin
    expires_at = DateTime.iso8601(retention.fetch("expiresAt"))
  rescue ArgumentError, TypeError
    raise "#{path}: retention expiresAt must be ISO-8601"
  end
  raise "#{path}: retention must expire after occurrence" unless expires_at > occurred_at
  data = event.fetch("data")
  raise "#{path}: data must be bounded" unless data.is_a?(Hash) && data["status"].is_a?(String)
  extensions = data["extensions"]
  raise "#{path}: extensions must be a bounded scalar map" unless extensions.nil? || (extensions.is_a?(Hash) && extensions.length <= 16 && extensions.all? { |key, value| key.match?(/\A[a-z][a-z0-9_.-]{0,63}\z/) && [String, Integer, Float, TrueClass, FalseClass, NilClass].include?(value.class) })
  forbidden_extension = /\A(?:assign|cancel|change[_-]?security|provision|release|retry|schedule|terminate|credential|secret)(?:[_.-]|\z)/
  raise "#{path}: extensions cannot request lifecycle control" if extensions&.keys&.any? { |key| key.match?(forbidden_extension) }
  serialized = JSON.generate(event)
  raise "#{path}: secret-shaped content found" if serialized.match?(secret_pattern)
  raise "#{path}: raw log field is forbidden" if serialized.match?(/"(?:logs?|stdout|stderr|environment|env)"\s*:/i)
  puts "valid lifecycle event: #{path}"
end
puts "all lifecycle event fixtures are valid"
RUBY
