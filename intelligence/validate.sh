#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

if [ "$#" -eq 0 ]; then
  set -- "$root"/config-*.v1.yaml "$root"/failure-event-*.v1.json
fi

command -v ruby >/dev/null 2>&1 || {
  echo "validate.sh: ruby is required" >&2
  exit 2
}

for file in "$@"; do
  [ -f "$file" ] || { echo "file not found: $file" >&2; exit 2; }
  case "$file" in
    *.yaml)
      ruby -ryaml - "$file" <<'RUBY'
path = ARGV.fetch(0)
data = YAML.safe_load(File.read(path), permitted_classes: [], aliases: false)
fail "document must be a mapping" unless data.is_a?(Hash)
fail "invalid apiVersion" unless data["apiVersion"] == "intelligence.leorunners.io/v1"
fail "invalid kind" unless data["kind"] == "IntelligenceConfig"
metadata = data.fetch("metadata")
spec = data.fetch("spec")
fail "metadata.version must be semantic version" unless metadata["version"].to_s.match?(/\A\d+\.\d+\.\d+\z/)
fail "metadata.name is required" unless metadata["name"].is_a?(String) && !metadata["name"].empty?
fail "enabled must be boolean" unless [true, false].include?(spec["enabled"])
provider = spec.fetch("provider")
fail "invalid provider mode" unless %w[disabled local hosted].include?(provider["mode"])
if spec["enabled"]
  fail "enabled config needs consent" unless spec.dig("consent", "approved") == true
  fail "enabled config needs triggers" unless spec["triggers"].is_a?(Array) && !spec["triggers"].empty?
  fail "enabled config needs endpoint and model" unless provider["endpoint"].to_s != "" && provider["model"].to_s != ""
else
  fail "disabled config must use disabled provider" unless provider["mode"] == "disabled" && provider["id"] == "none"
  fail "disabled config must have no triggers" unless spec["triggers"] == []
end
fail "local mode must use customer-local provider" if provider["mode"] == "local" && provider["id"] != "customer-local"
fail "hosted mode must declare region" if provider["mode"] == "hosted" && provider["region"].to_s == ""
allowed_triggers = %w[job_failed job_timed_out repeated_failure]
fail "unknown trigger" unless spec.fetch("triggers").is_a?(Array) && spec["triggers"].all? { |trigger| allowed_triggers.include?(trigger) }
input = spec.fetch("input")
fail "redaction is required" unless input["redact_before_persist"] == true
fail "excerpt limit is invalid" unless input["max_excerpt_bytes"].is_a?(Integer) && input["max_excerpt_bytes"].between?(0, 65536)
retention = spec.fetch("retention")
fail "retention must be finite non-negative integers" unless %w[event_hours evidence_hours].all? { |key| retention[key].is_a?(Integer) && retention[key] >= 0 }
policy = spec.fetch("action_policy")
fail "autonomous actions are forbidden" unless policy["autonomous_actions"] == false
confidence = policy["minimum_confidence"]
fail "invalid confidence" unless confidence.is_a?(Numeric) && confidence.between?(0.0, 1.0)
actions = %w[retry change_capacity inspect_image open_incident none]
fail "unknown action" unless policy.fetch("allowed_actions").is_a?(Array) && policy["allowed_actions"].all? { |action| actions.include?(action) }
puts "valid intelligence config: #{path}"
RUBY
      ;;
    *.json)
      ruby -rjson - "$file" <<'RUBY'
path = ARGV.fetch(0)
data = JSON.parse(File.read(path))
fail "document must be a mapping" unless data.is_a?(Hash)
fail "invalid apiVersion" unless data["apiVersion"] == "intelligence.leorunners.io/v1"
fail "invalid kind" unless data["kind"] == "FailureAnalysisEvent"
metadata = data.fetch("metadata")
spec = data.fetch("spec")
required = %w[event_id created_at tenant_id job_id policy_version]
fail "metadata fields are required" unless required.all? { |key| metadata[key].is_a?(String) && !metadata[key].empty? }
fail "invalid trigger" unless %w[job_failed job_timed_out repeated_failure].include?(spec["trigger"])
fail "failure metadata is incomplete" unless %w[failure_class lifecycle_phase runner_profile].all? { |key| spec[key].is_a?(String) && !spec[key].empty? }
redaction = spec.fetch("redaction")
fail "evidence must be redacted" unless redaction["status"] == "complete" && redaction["ruleset"].is_a?(String)
evidence = spec.fetch("evidence")
fail "evidence fields are invalid" unless evidence["source"].is_a?(String) && evidence["excerpt"].is_a?(String) && evidence["excerpt"].bytesize <= 65536 && evidence["sha256"].to_s.match?(/\A[0-9a-f]{64}\z/) && evidence["bytes"].is_a?(Integer) && evidence["bytes"] >= 0
serialized = JSON.generate(data)
credential_pattern = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|(?:password|secret|access[_-]?key)\s*[:=]\s*\S+/i
fail "recognizable credential pattern found" if serialized.match?(credential_pattern)
action = spec.fetch("proposed_action")
fail "invalid action" unless %w[retry change_capacity inspect_image open_incident none].include?(action["action"])
fail "invalid confidence" unless action["confidence"].is_a?(Numeric) && action["confidence"].between?(0.0, 1.0)
fail "autonomous action is forbidden" unless action["autonomous"] == false
puts "valid intelligence event: #{path}"
RUBY
      ;;
    *)
      echo "unsupported file type: $file" >&2
      exit 2
      ;;
  esac
done

echo "all intelligence fixtures are valid"
