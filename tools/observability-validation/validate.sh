#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
config_root="$root/../.."/observability
command -v ruby >/dev/null 2>&1 || { printf '%s\n' 'validate.sh: ruby is required' >&2; exit 2; }

if [ "$#" -eq 0 ]; then
  set -- "$config_root"/alert-config.v1.yaml "$config_root"/alert-config-warning.v1.yaml
fi
schema="$config_root/schema.v1.json"
[ -f "$schema" ] || { printf 'missing schema: %s\n' "$schema" >&2; exit 2; }
for file in "$@"; do
  [ -f "$file" ] || { printf 'file not found: %s\n' "$file" >&2; exit 2; }
done

set +e
ruby -ryaml -rjson - "$schema" "$@" <<'RUBY'
schema_path = ARGV.shift
paths = ARGV
schema = JSON.parse(File.read(schema_path))
abort "schema must identify observability v1" unless schema["$id"].to_s.end_with?("observability.v1.json")

allowed_dimensions = %w[provider region state outcome capacity_owner error_class extension]
extension_metrics = %w[extension_failures_total extension_timeouts_total extension_panics_total extension_events_dropped_total extension_reports_dropped_total]
bounded_name = /\A[a-z][a-z0-9_.-]{2,63}\z/
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
control_action = /\A(?:provision|terminate|cancel|retry|schedule|assign|release|change[_-]?security|credential)/i
errors = 0
warnings = 0
seen_configs = {}

paths.each do |path|
  begin
    raw = File.read(path)
    raise "secret-shaped content found; remove it before storing this config" if raw.match?(secret)
    data = YAML.safe_load(raw, permitted_classes: [], aliases: false)
    raise "document must be a mapping" unless data.is_a?(Hash)
    raise "invalid apiVersion" unless data["apiVersion"] == "observability.leorunners.io/v1"
    raise "invalid kind" unless data["kind"] == "AlertConfig"
    metadata = data.fetch("metadata")
    spec = data.fetch("spec")
    name = metadata.fetch("name")
    raise "metadata.name is invalid" unless name.is_a?(String) && name.match?(bounded_name)
    version = metadata.fetch("version")
    raise "metadata.version must be semantic version" unless version.is_a?(String) && version.match?(/\A\d+\.\d+\.\d+\z/)
    key = [name, version]
    raise "duplicate config identity" if seen_configs.key?(key)
    seen_configs[key] = true
    raise "enabled must be boolean" unless [true, false].include?(spec["enabled"])
    dimensions = spec.fetch("dimensions")
    raise "dimensions must be a unique array of allowlisted values" unless dimensions.is_a?(Array) && dimensions.uniq == dimensions && dimensions.length <= 7 && dimensions.all? { |d| allowed_dimensions.include?(d) }
    alerts = spec.fetch("alerts")
    raise "alerts must contain 1..64 entries" unless alerts.is_a?(Array) && alerts.length.between?(1, 64)
    alerts.each do |alert|
      raise "alert must be a mapping" unless alert.is_a?(Hash)
      raise "alert name is invalid" unless alert["name"].is_a?(String) && alert["name"].match?(bounded_name)
      raise "alert enabled must be boolean" unless [true, false].include?(alert["enabled"])
      raise "metric name is invalid" unless alert["metric"].is_a?(String) && alert["metric"].match?(/\A[a-z][a-z0-9_.-]{2,127}\z/)
      if alert["metric"].start_with?("extension_")
        raise "unsupported extension signal" unless extension_metrics.include?(alert["metric"])
        raise "extension signals must use Sum" unless alert["statistic"] == "Sum"
      end
      raise "unsupported statistic or comparison" unless %w[Sum Average Minimum Maximum SampleCount].include?(alert["statistic"]) && %w[GreaterThanOrEqualToThreshold GreaterThanThreshold LessThanOrEqualToThreshold LessThanThreshold].include?(alert["comparison"])
      raise "threshold must be numeric" unless alert["threshold"].is_a?(Numeric)
      raise "period_seconds must be at least 10" unless alert["period_seconds"].is_a?(Integer) && alert["period_seconds"] >= 10
      raise "evaluation_periods must be 1..100" unless alert["evaluation_periods"].is_a?(Integer) && alert["evaluation_periods"].between?(1, 100)
      raise "invalid severity" unless %w[info warning critical].include?(alert["severity"])
      alert_dimensions = alert.fetch("dimensions")
      raise "alert dimensions must be a subset of the global allowlist" unless alert_dimensions.is_a?(Array) && alert_dimensions.uniq == alert_dimensions && alert_dimensions.length <= 7 && (alert_dimensions - dimensions).empty? && alert_dimensions.all? { |d| allowed_dimensions.include?(d) }
      if extension_metrics.include?(alert["metric"])
        raise "extension signal must use notification-only actions" unless alert["actions"].is_a?(Array)
      end
      actions = alert.fetch("actions")
      raise "unsupported or unsafe alert action" unless actions.is_a?(Array) && actions.uniq == actions && actions.all? { |a| %w[notify_oncall notify_team none].include?(a) && !a.match?(control_action) }
      warnings += 1 unless alert["enabled"]
    end
    puts "PASS #{path}"
    puts "WARN #{path}: disabled alert is not evaluated" if alerts.any? { |a| !a["enabled"] }
  rescue StandardError => e
    errors += 1
    warn "FAIL #{path}: #{e.message.split(':').first}"
  end
end
if errors.positive?
  warn "observability validation failed: #{errors} file(s)"
  exit 1
end
puts "observability validation passed with #{warnings} warning(s)"
RUBY
status=$?
set -e
exit "$status"
