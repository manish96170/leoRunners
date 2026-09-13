#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
config_root="$root/../../extensions"
command -v ruby >/dev/null 2>&1 || { echo "validate.sh: ruby is required" >&2; exit 2; }

if [ "$#" -eq 0 ]; then
  set -- "$config_root"/runtime-policy.v1.yaml
fi
for file in "$@"; do
  [ -f "$file" ] || { echo "file not found: $file" >&2; exit 2; }
done

set +e
ruby -ryaml -rjson - "$config_root/runtime-policy.schema.v1.json" "$@" <<'RUBY'
schema_path = ARGV.shift
paths = ARGV
schema = JSON.parse(File.read(schema_path))
abort "schema must identify runtime policy v1" unless schema["$id"].to_s.end_with?("extensions-runtime-policy.v1.json")

known = %w[analytics cache cost security]
tenant_name = /\A[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}\z/
policy_name = /\A[a-z][a-z0-9.-]{2,63}\z/
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
forbidden = /\b(?:provision|terminate|cancel|retry|schedule|assign|authorize|credential|grant|release|raw|stdout|stderr|environment|logs?)\b/i
seen = {}
exit_code = 0

paths.each do |path|
  begin
    raw = File.read(path)
    raise "secret-shaped content found" if raw.match?(secret)
    data = YAML.safe_load(raw, permitted_classes: [], aliases: false)
    raise "document must be a mapping" unless data.is_a?(Hash)
    raise "invalid apiVersion or kind" unless data["apiVersion"] == "extensions.leorunners.io/v1" && data["kind"] == "ExtensionRuntimePolicy"
    metadata = data.fetch("metadata")
    name = metadata.fetch("name")
    version = metadata.fetch("version")
    raise "invalid metadata name" unless name.is_a?(String) && name.match?(policy_name)
    raise "invalid metadata version" unless version.is_a?(String) && version.match?(/\A\d+\.\d+\.\d+\z/)
    identity = [name, version]
    raise "duplicate policy identity" if seen[identity]
    seen[identity] = true

    spec = data.fetch("spec")
    global = spec.fetch("global")
    raise "global.enabled must be boolean" unless [true, false].include?(global["enabled"])
    global_tenants = global.fetch("tenant_allowlist")
    raise "global tenant allowlist must be explicit and bounded" unless global_tenants.is_a?(Array) && global_tenants.length.between?(1, 256) && global_tenants.uniq == global_tenants && global_tenants.all? { |t| t.is_a?(String) && t.match?(tenant_name) && t != "*" }

    consumers = spec.fetch("consumers")
    raise "consumers must contain 1..4 entries" unless consumers.is_a?(Array) && consumers.length.between?(1, known.length)
    ids = consumers.map { |c| c.is_a?(Hash) ? c["id"] : nil }
    raise "consumer IDs must be known and unique" unless ids.uniq == ids && ids.all? { |id| known.include?(id) }
    consumers.each do |consumer|
      raise "consumer must be a mapping" unless consumer.is_a?(Hash)
      id = consumer.fetch("id")
      raise "consumer must be enabled boolean" unless [true, false].include?(consumer["enabled"])
      tenants = consumer.fetch("tenant_allowlist")
      raise "consumer tenant allowlist must be an explicit subset" unless tenants.is_a?(Array) && tenants.length.between?(1, 256) && tenants.uniq == tenants && tenants.all? { |t| t.is_a?(String) && t.match?(tenant_name) && t != "*" } && (tenants - global_tenants).empty?
      queue = consumer.fetch("queue")
      raise "queue bounds are invalid" unless queue.is_a?(Hash) && queue["max_items"].is_a?(Integer) && queue["max_items"].between?(1, 10_000) && %w[drop reject].include?(queue["overflow"])
      raise "timeout_ms must be between 10 and 30000" unless consumer["timeout_ms"].is_a?(Integer) && consumer["timeout_ms"].between?(10, 30_000)
      raise "capabilities must be read-only and bounded" unless consumer["capabilities"] == %w[read_events emit_advisories]
      redaction = consumer.fetch("redaction")
      raise "redaction is mandatory metadata-only and bounded" unless redaction.is_a?(Hash) && redaction["required"] == true && redaction["mode"] == "metadata-only" && redaction["max_bytes"].is_a?(Integer) && redaction["max_bytes"].between?(256, 262_144)
      raise "unsafe policy content found" if JSON.generate(consumer).match?(forbidden)
      puts "PASS #{path}: #{id}"
    end
    puts "PASS #{path}: runtime policy #{name}@#{version}"
  rescue StandardError => e
    warn "FAIL #{path}: #{e.message.split(':').first}"
    exit_code = 1
  end
end
exit(exit_code || 0)
RUBY
status=$?
set -e
exit "$status"
