#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
config_root="$root/../../validation"
schema="$config_root/evidence.schema.v1.json"
command -v ruby >/dev/null 2>&1 || { printf '%s\n' 'validate.sh: ruby is required' >&2; exit 2; }
[ -f "$schema" ] || { printf 'missing schema: %s\n' "$schema" >&2; exit 2; }
[ "$#" -gt 0 ] || set -- "$config_root/evidence.v1.json"
for file in "$@"; do
  [ -f "$file" ] || { printf 'file not found: %s\n' "$file" >&2; exit 2; }
done

set +e
ruby -rjson -rtime - "$schema" "$@" <<'RUBY'
schema_path = ARGV.shift
paths = ARGV
schema = JSON.parse(File.read(schema_path))
abort "schema must identify validation evidence v1" unless schema["$id"].to_s.end_with?("validation-evidence.v1.json")
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
name_pattern = /\A[a-z][a-z0-9.-]{2,63}\z/
run_id_pattern = /\A[A-Za-z0-9][A-Za-z0-9._:\/-]{0,127}\z/
repo_pattern = /\A[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}\z/
region_pattern = /\A[a-z0-9-]{2,32}\z/
ref_pattern = /\A(?:https:\/\/|arn:|[a-z][a-z0-9+.-]*:\/\/|[A-Za-z0-9_.:-])[\S]{0,511}\z/
iso = ->(value) { value.is_a?(String) && Time.iso8601(value) }
seen = {}
errors = 0

def require_keys(hash, keys, label)
  raise "#{label} must be a mapping" unless hash.is_a?(Hash)
  raise "#{label} has unsupported fields" unless (hash.keys - keys).empty?
  raise "#{label} is missing required fields" unless (keys - hash.keys).empty?
end

def bounded_integer?(value, min, max)
  value.is_a?(Integer) && value.between?(min, max)
end

paths.each do |path|
  begin
    raw = File.read(path)
    raise "secret-shaped content found; evidence must be redacted" if raw.match?(secret)
    data = JSON.parse(raw)
    require_keys(data, %w[apiVersion kind metadata spec], "document")
    raise "invalid apiVersion or kind" unless data["apiVersion"] == "validation.leorunners.io/v1" && data["kind"] == "ControlledValidationEvidence"
    metadata = data["metadata"]
    require_keys(metadata, %w[name version], "metadata")
    raise "metadata.name is invalid" unless metadata["name"].is_a?(String) && metadata["name"].match?(name_pattern)
    raise "metadata.version is invalid" unless metadata["version"].is_a?(String) && metadata["version"].match?(/\A\d+\.\d+\.\d+\z/)
    identity = [metadata["name"], metadata["version"]]
    raise "duplicate evidence identity" if seen[identity]
    seen[identity] = true

    spec = data["spec"]
    require_keys(spec, %w[run lifecycle cleanup evidence], "spec")
    run = spec["run"]
    require_keys(run, %w[run_id repository workflow provider region started_at finished_at duration_ms], "run")
    raise "run_id is invalid" unless run["run_id"].is_a?(String) && run["run_id"].match?(run_id_pattern)
    raise "repository is invalid" unless run["repository"].is_a?(String) && run["repository"].match?(repo_pattern)
    raise "workflow is invalid" unless run["workflow"].is_a?(String) && run["workflow"].length.between?(1, 128)
    raise "provider or region is invalid" unless %w[aws gcp].include?(run["provider"]) && run["region"].is_a?(String) && run["region"].match?(region_pattern)
    raise "run timestamps are invalid" unless iso.call(run["started_at"]) && iso.call(run["finished_at"])
    start_time = Time.iso8601(run["started_at"])
    finish_time = Time.iso8601(run["finished_at"])
    raise "run finishes before it starts" if finish_time < start_time
    raise "duration_ms is invalid" unless bounded_integer?(run["duration_ms"], 0, 86_400_000)
    raise "duration_ms does not match timestamps" unless ((finish_time - start_time) * 1000 - run["duration_ms"]).abs <= 1000

    lifecycle = spec["lifecycle"]
    raise "lifecycle must contain 4..16 checkpoints" unless lifecycle.is_a?(Array) && lifecycle.length.between?(4, 16)
    allowed = %w[queued provisioning ready assigned running completed cleanup_started cleanup_completed]
    names = lifecycle.map { |checkpoint| checkpoint.is_a?(Hash) ? checkpoint["name"] : nil }
    raise "lifecycle checkpoint names are invalid or duplicated" unless names.uniq == names && names.all? { |name| allowed.include?(name) }
    raise "lifecycle must prove completion and cleanup" unless %w[completed cleanup_started cleanup_completed].all? { |name| names.include?(name) }
    previous = nil
    lifecycle.each do |checkpoint|
      require_keys(checkpoint, %w[name status observed_at duration_ms], "lifecycle checkpoint")
      raise "lifecycle status is invalid" unless %w[observed skipped].include?(checkpoint["status"])
      raise "lifecycle timestamp is invalid" unless iso.call(checkpoint["observed_at"])
      observed = Time.iso8601(checkpoint["observed_at"])
      raise "lifecycle timestamps are out of order" if previous && observed < previous
      previous = observed
      raise "lifecycle duration_ms is invalid" unless bounded_integer?(checkpoint["duration_ms"], 0, 86_400_000)
    end

    cleanup = spec["cleanup"]
    require_keys(cleanup, %w[attempted completed resource_refs verified_at], "cleanup")
    raise "cleanup proof must confirm attempted and completed" unless cleanup["attempted"] == true && cleanup["completed"] == true
    refs = cleanup["resource_refs"]
    raise "cleanup resource references are invalid or unbounded" unless refs.is_a?(Array) && refs.length.between?(1, 32) && refs.uniq == refs && refs.all? { |ref| ref.is_a?(String) && ref.length <= 256 && ref.match?(ref_pattern) }
    raise "cleanup verified_at is invalid" unless iso.call(cleanup["verified_at"])

    evidence = spec["evidence"]
    require_keys(evidence, %w[metrics logs alarms dashboards], "evidence")
    evidence.each do |category, entries|
      raise "#{category} references are invalid or unbounded" unless entries.is_a?(Array) && entries.length <= 32
      entries.each do |entry|
        require_keys(entry, %w[ref captured_at], "#{category} reference")
        raise "#{category} reference is invalid or unbounded" unless entry["ref"].is_a?(String) && entry["ref"].length.between?(1, 512) && entry["ref"].match?(ref_pattern)
        raise "#{category} captured_at is invalid" unless iso.call(entry["captured_at"])
      end
    end
    puts "PASS #{path}: #{run["provider"]}/#{run["region"]} #{run["run_id"]}"
  rescue StandardError => e
    errors += 1
    warn "FAIL #{path}: #{e.message.split(':').first}"
  end
end
if errors.positive?
  warn "evidence validation failed: #{errors} file(s)"
  exit 1
end
puts "controlled validation evidence passed for #{paths.length} file(s)"
RUBY
status=$?
set -e
exit "$status"
