#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
schema="$root/../../activation/activation-request.schema.v1.json"
command -v ruby >/dev/null 2>&1 || { printf '%s\n' 'validate.sh: ruby is required' >&2; exit 2; }
[ -f "$schema" ] || { printf 'missing schema: %s\n' "$schema" >&2; exit 2; }
[ "$#" -gt 0 ] || set -- "$root/../../activation/activation-request.v1.json"
for file in "$@"; do
  [ -f "$file" ] || { printf 'file not found: %s\n' "$file" >&2; exit 2; }
done

set +e
ruby -rjson -rtime - "$schema" "$@" <<'RUBY'
schema_path = ARGV.shift
paths = ARGV
schema = JSON.parse(File.read(schema_path))
abort "schema must identify activation request v1" unless schema["$id"].to_s.end_with?("activation-request.v1.json")
now = Time.iso8601(ENV.fetch("LEO_ACTIVATION_NOW", Time.now.utc.iso8601))
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:api[_-]?)?(?:password|secret|token|access[_-]?key)\s*["']?\s*:\s*["']?\S+/i
iso = ->(value) { value.is_a?(String) && Time.iso8601(value) }
errors = 0
seen = {}

def exact_keys(hash, required, label, allowed = required)
  raise "#{label} must be an object" unless hash.is_a?(Hash)
  raise "#{label} has unsupported fields" unless (hash.keys - allowed).empty?
  raise "#{label} is missing required fields" unless (required - hash.keys).empty?
end

def owner?(value)
  value.is_a?(Hash) && value["id"].is_a?(String) && value["id"].match?(/\A[A-Za-z0-9][A-Za-z0-9_.@-]{1,63}\z/) &&
    value["contact"].is_a?(String) && value["contact"].match?(/\A[^\s@]+@[^\s@]+\.[^\s@]+\z/)
end

paths.each do |path|
  begin
    raw = File.read(path)
    raise "secret-shaped content found; activation requests must be secret-free" if raw.match?(secret)
    data = JSON.parse(raw)
    exact_keys(data, %w[apiVersion kind metadata spec], "document")
    raise "invalid apiVersion or kind" unless data["apiVersion"] == "activation.leorunners.io/v1" && data["kind"] == "ActivationRequest"
    metadata = data["metadata"]
    exact_keys(metadata, %w[name version], "metadata")
    raise "metadata identity is invalid" unless metadata["name"].is_a?(String) && metadata["name"].match?(/\A[a-z][a-z0-9.-]{2,63}\z/) && metadata["version"].is_a?(String) && metadata["version"].match?(/\A\d+\.\d+\.\d+\z/)
    spec = data["spec"]
    required = %w[request_id requested_at operator approver target scope_file scopes resource_prefix max_resources deadline budget notification rollback_owner mode]
    raise "duplicate request identity" if seen[[metadata["name"], metadata["version"]]]
    seen[[metadata["name"], metadata["version"]]] = true
    exact_keys(spec, required, "spec", required + ["confirmation"])
    raise "request_id is invalid" unless spec["request_id"].is_a?(String) && spec["request_id"].match?(/\A[A-Za-z0-9][A-Za-z0-9._:\/-]{0,127}\z/)
    requested_at = iso.call(spec["requested_at"])
    deadline = iso.call(spec["deadline"])
    raise "requested_at or deadline is invalid" unless requested_at && deadline
    raise "deadline must be after requested_at" unless deadline > requested_at
    raise "request is expired" unless deadline > now
    raise "deadline exceeds maximum activation window" if deadline > requested_at + 86_400
    raise "operator is missing or invalid" unless owner?(spec["operator"])
    raise "approver is missing or invalid" unless owner?(spec["approver"])
    raise "rollback owner is missing or invalid" unless owner?(spec["rollback_owner"])
    raise "operator and approver must be distinct" if spec["operator"]["id"] == spec["approver"]["id"]

    target = spec["target"]
    exact_keys(target, %w[provider account project region repository], "target")
    raise "target provider is invalid" unless %w[aws gcp github].include?(target["provider"])
    raise "target region is invalid" unless target["region"].is_a?(String) && target["region"].match?(/\A[a-z0-9][a-z0-9-]{1,31}\z/)
    raise "target repository is invalid" unless target["repository"].is_a?(String) && target["repository"].match?(/\A[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}\z/)
    if target["provider"] == "aws"
      raise "AWS target requires account and no project" unless target["account"].is_a?(String) && target["account"].match?(/\A\d{12}\z/) && target["project"].nil?
    elsif target["provider"] == "gcp"
      raise "GCP target requires project and no account" unless target["project"].is_a?(String) && target["project"].match?(/\A[a-z][a-z0-9-]{4,28}[a-z0-9]\z/) && target["account"].nil?
    else
      raise "GitHub target requires neither account nor project" unless target["account"].nil? && target["project"].nil?
    end

    scope_file = spec["scope_file"]
    exact_keys(scope_file, %w[path sha256], "scope_file")
    raise "scope file path is invalid" unless scope_file["path"].is_a?(String) && scope_file["path"].match?(/\A(?!\/)(?!.*\.\.)[A-Za-z0-9_.-][A-Za-z0-9_\.\/-]{0,255}\z/)
    raise "scope file hash is invalid" unless scope_file["sha256"].is_a?(String) && scope_file["sha256"].match?(/\A[a-f0-9]{64}\z/)
    scopes = spec["scopes"]
    raise "scopes are invalid or unbounded" unless scopes.is_a?(Array) && scopes.length.between?(1, 32) && scopes.uniq == scopes && scopes.all? { |scope| scope.is_a?(String) && scope.length.between?(1, 256) && !scope.match?(/[?*\[\]{}]/) }
    raise "resource prefix is invalid" unless spec["resource_prefix"].is_a?(String) && spec["resource_prefix"].match?(/\A[a-z][a-z0-9-]{2,31}\z/)
    raise "max_resources is invalid" unless spec["max_resources"].is_a?(Integer) && spec["max_resources"].between?(1, 100)
    budget = spec["budget"]
    exact_keys(budget, %w[currency max_amount], "budget")
    raise "budget is invalid" unless budget["currency"] == "USD" && budget["max_amount"].is_a?(Numeric) && budget["max_amount"] > 0 && budget["max_amount"] <= 10_000
    notification = spec["notification"]
    exact_keys(notification, %w[channel destination], "notification")
    raise "notification is invalid" unless %w[email slack pagerduty].include?(notification["channel"]) && notification["destination"].is_a?(String) && notification["destination"].length.between?(1, 256) && !notification["destination"].match?(/\s/)
    raise "mode is invalid" unless %w[read-only connectivity lifecycle].include?(spec["mode"])
    if spec["mode"] == "lifecycle"
      confirmation = spec["confirmation"]
      exact_keys(confirmation, %w[approved_at phrase change_reference], "confirmation")
      approved_at = iso.call(confirmation["approved_at"])
      raise "lifecycle confirmation is invalid" unless approved_at && approved_at <= now && confirmation["phrase"] == "I_UNDERSTAND_EPHEMERAL_RESOURCES" && confirmation["change_reference"].is_a?(String) && confirmation["change_reference"].match?(/\A[A-Za-z0-9][A-Za-z0-9._:\/-]{2,127}\z/)
    elsif spec.key?("confirmation")
      raise "confirmation metadata is only allowed for lifecycle mode"
    end
    puts "PASS #{path}: #{spec["mode"]} #{target["provider"]}/#{target["region"]} #{spec["request_id"]}"
  rescue StandardError => e
    errors += 1
    warn "FAIL #{path}: #{e.message.split(':').first}"
  end
end
if errors.positive?
  warn "activation validation failed: #{errors} file(s)"
  exit 1
end
puts "activation requests passed for #{paths.length} file(s)"
RUBY
status=$?
set -e
exit "$status"
