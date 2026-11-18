#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
validation="$root/../../validation"
schema="$validation/approved-scope.schema.v1.json"
command -v ruby >/dev/null 2>&1 || { printf '%s\n' 'validate-scope.sh: ruby is required' >&2; exit 2; }
[ -f "$schema" ] || { printf 'missing schema: %s\n' "$schema" >&2; exit 2; }
[ "$#" -gt 0 ] || set -- "$validation/approved-scope.v1.json"
for file in "$@"; do
  [ -f "$file" ] || { printf 'file not found: %s\n' "$file" >&2; exit 2; }
done

set +e
ruby -rjson -rtime - "$schema" "$@" <<'RUBY'
schema_path = ARGV.shift
paths = ARGV
schema = JSON.parse(File.read(schema_path))
abort "schema must identify approved validation scope v1" unless schema["$id"].to_s.end_with?("approved-validation-scope.v1.json")
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
name_pattern = /\A[a-z][a-z0-9.-]{2,63}\z/
region_pattern = /\A[a-z0-9][a-z0-9-]{1,30}[a-z0-9]\z/
project_pattern = /\A[A-Za-z0-9][A-Za-z0-9._:\/-]{1,127}\z/
repo_pattern = /\A[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}\z/
prefix_pattern = /\A[a-z][a-z0-9-]{2,30}\z/
owner_pattern = /\A[A-Za-z0-9][A-Za-z0-9 ._\/@:+-]{2,127}\z/
seen = {}
errors = 0

def require_keys(hash, keys, label)
  raise "#{label} must be a mapping" unless hash.is_a?(Hash)
  raise "#{label} has unsupported fields" unless (hash.keys - keys).empty?
  raise "#{label} is missing required fields" unless (keys - hash.keys).empty?
end

paths.each do |path|
  begin
    raw = File.read(path)
    raise "secret-shaped content found; scope must not contain credentials" if raw.match?(secret)
    data = JSON.parse(raw)
    require_keys(data, %w[apiVersion kind metadata spec], "document")
    raise "invalid apiVersion or kind" unless data["apiVersion"] == "validation.leorunners.io/v1" && data["kind"] == "ApprovedValidationScope"
    metadata = data["metadata"]
    require_keys(metadata, %w[name version], "metadata")
    raise "metadata.name is invalid" unless metadata["name"].is_a?(String) && metadata["name"].match?(name_pattern)
    raise "metadata.version is invalid" unless metadata["version"].is_a?(String) && metadata["version"].match?(/\A\d+\.\d+\.\d+\z/)
    identity = [metadata["name"], metadata["version"]]
    raise "duplicate scope identity" if seen[identity]
    seen[identity] = true

    spec = data["spec"]
    require_keys(spec, %w[provider region project repository resource_prefix max_resources deadline ownership], "spec")
    raise "provider is invalid" unless %w[aws gcp].include?(spec["provider"])
    raise "region is invalid" unless spec["region"].is_a?(String) && spec["region"].match?(region_pattern)
    raise "project is invalid" unless spec["project"].is_a?(String) && spec["project"].match?(project_pattern)
    raise "repository is invalid" unless spec["repository"].is_a?(String) && spec["repository"].match?(repo_pattern)
    raise "resource_prefix is invalid" unless spec["resource_prefix"].is_a?(String) && spec["resource_prefix"].match?(prefix_pattern)
    raise "max_resources must be between 1 and 100" unless spec["max_resources"].is_a?(Integer) && spec["max_resources"].between?(1, 100)
    deadline = spec["deadline"]
    raise "deadline is invalid" unless deadline.is_a?(String)
    Time.iso8601(deadline)
    ownership = spec["ownership"]
    require_keys(ownership, %w[notification rollback], "ownership")
    raise "notification owner is invalid" unless ownership["notification"].is_a?(String) && ownership["notification"].match?(owner_pattern)
    raise "rollback owner is invalid" unless ownership["rollback"].is_a?(String) && ownership["rollback"].match?(owner_pattern)
    puts "PASS #{path}: #{spec["provider"]}/#{spec["region"]} #{spec["repository"]} max=#{spec["max_resources"]} deadline=#{deadline}"
  rescue StandardError => e
    errors += 1
    warn "FAIL #{path}: #{e.message.split(":").first}"
  end
end
if errors.positive?
  warn "approved scope validation failed: #{errors} file(s)"
  exit 1
end
puts "approved validation scopes passed for #{paths.length} file(s)"
RUBY
status=$?
set -e
exit "$status"
