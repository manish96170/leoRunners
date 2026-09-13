#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
command -v ruby >/dev/null 2>&1 || { printf '%s\n' 'validate.sh: ruby is required' >&2; exit 2; }
[ "$#" -gt 0 ] || set -- "$root/config.v1.json"

set +e
ruby -rjson - "$@" <<'RUBY'
paths = ARGV
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
errors = 0
paths.each do |path|
  begin
    raw = File.read(path)
    raise "secret-shaped content found" if raw.match?(secret)
    data = JSON.parse(raw)
    raise "unsupported document" unless data.is_a?(Hash) && data["apiVersion"] == "events.leorunners.io/v1" && data["kind"] == "EventArchiveRetention"
    metadata = data.fetch("metadata")
    raise "invalid metadata" unless metadata.is_a?(Hash) && metadata.keys.sort == %w[name version] && metadata["name"].match?(/\A[a-z][a-z0-9.-]{2,63}\z/) && metadata["version"].match?(/\A\d+\.\d+\.\d+\z/)
    spec = data.fetch("spec")
    required = %w[enabled path file_mode owner_only restart_behavior max_line_bytes max_bytes max_files max_age_seconds]
    raise "invalid spec fields" unless spec.is_a?(Hash) && (spec.keys - required).empty? && (required - spec.keys).empty?
    raise "enabled must be boolean" unless [true, false].include?(spec["enabled"])
    raise "path must be absolute" unless spec["path"].is_a?(String) && spec["path"].start_with?("/") && spec["path"].length <= 512 && !spec["path"].include?("..")
    raise "owner-only JSONL permissions are required" unless spec["file_mode"] == "0600" && spec["owner_only"] == true && spec["restart_behavior"] == "reopen_append"
    raise "max_line_bytes is out of bounds" unless spec["max_line_bytes"].is_a?(Integer) && spec["max_line_bytes"].between?(1024, 16 * 1024 * 1024)
    raise "max_bytes is out of bounds" unless spec["max_bytes"].is_a?(Integer) && spec["max_bytes"].between?(0, 1 << 30)
    raise "max_files is out of bounds" unless spec["max_files"].is_a?(Integer) && spec["max_files"].between?(0, 64)
    raise "max_age_seconds is out of bounds" unless spec["max_age_seconds"].is_a?(Integer) && spec["max_age_seconds"].between?(0, 30 * 24 * 60 * 60)
    if spec["enabled"] && (spec["max_bytes"] > 0 || spec["max_age_seconds"] > 0)
      raise "rotation requires max_files between 2 and 64" unless spec["max_files"].between?(2, 64)
    end
    puts "PASS #{path}"
  rescue StandardError => e
    errors += 1
    warn "FAIL #{path}: #{e.message}"
  end
end
exit 1 if errors.positive?
puts "event retention validation passed for #{paths.length} file(s)"
RUBY
status=$?
set -e
exit "$status"
