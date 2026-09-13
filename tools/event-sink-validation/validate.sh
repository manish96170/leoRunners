#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
schema="$root/../../events/sink-contract.v1.json"
default_contract="$root/../../events/sink-contract.safe.v1.json"
default_behavior="$root/../../events/sink-behavior.v1.json"
command -v ruby >/dev/null 2>&1 || { printf '%s\n' 'validate.sh: ruby is required' >&2; exit 2; }

contract=${1:-$default_contract}
behavior=${2:-$default_behavior}
for file in "$schema" "$contract" "$behavior"; do
  [ -f "$file" ] || { printf 'file not found: %s\n' "$file" >&2; exit 2; }
done

set +e
ruby -rjson - "$schema" "$contract" "$behavior" <<'RUBY'
schema_path, contract_path, behavior_path = ARGV
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
failures = []
begin
  schema = JSON.parse(File.read(schema_path))
  failures << "schema must be the v1 object contract" unless schema["$id"].to_s.end_with?("event-sink-contract.v1.json") && schema["type"] == "object"
rescue StandardError
  failures << "schema could not be parsed"
end

begin
  contract_raw = File.read(contract_path)
  failures << "contract contains secret-shaped content" if contract_raw.match?(secret)
  contract = JSON.parse(contract_raw)
  failures << "invalid contract identity" unless contract.is_a?(Hash) && contract["apiVersion"] == "events.leorunners.io/v1" && contract["kind"] == "EventSinkContract"
  metadata = contract.fetch("metadata")
  failures << "invalid metadata" unless metadata.is_a?(Hash) && metadata.keys.sort == %w[name version] && metadata["name"].to_s.match?(/\A[a-z][a-z0-9.-]{2,63}\z/) && metadata["version"].to_s.match?(/\A\d+\.\d+\.\d+\z/)
  spec = contract.fetch("spec")
  failures << "transport must be local-jsonl" unless spec["transport"] == "local-jsonl"
  delivery = spec.fetch("delivery")
  failures << "delivery must persist before ack with deduplication key" unless delivery["ack_mode"] == "persist_before_ack" && delivery["idempotency_key"] == "delivery.deduplicationKey"
  failures << "delivery bounds are invalid" unless delivery["max_attempts"].is_a?(Integer) && delivery["max_attempts"].between?(1, 10) && delivery["retry_backoff_seconds"].is_a?(Integer) && delivery["retry_backoff_seconds"].between?(0, 3600) && delivery["queue_capacity"].is_a?(Integer) && delivery["queue_capacity"].between?(1, 100_000) && %w[drop reject].include?(delivery["overflow"])
  dlq = delivery.fetch("dead_letter")
  failures << "dead-letter bounds are invalid" unless dlq["enabled"] == true && dlq["max_items"].is_a?(Integer) && dlq["max_items"].between?(1, 100_000)
  archive = spec.fetch("archive")
  failures << "archive must be absolute and bounded" unless archive["path"].is_a?(String) && archive["path"].start_with?("/") && !archive["path"].include?("..") && archive["path"].length <= 512
  failures << "archive must be owner-only 0600 and append-safe" unless archive["file_mode"] == "0600" && archive["owner_only"] == true && archive["restart_behavior"] == "reopen_append"
  failures << "archive retention bounds are invalid" unless archive["max_line_bytes"].is_a?(Integer) && archive["max_line_bytes"].between?(1024, 16 * 1024 * 1024) && archive["max_bytes"].is_a?(Integer) && archive["max_bytes"].between?(0, 1 << 30) && archive["max_files"].is_a?(Integer) && archive["max_files"].between?(0, 64) && archive["max_age_seconds"].is_a?(Integer) && archive["max_age_seconds"].between?(0, 30 * 24 * 60 * 60)
  redaction = spec.fetch("redaction")
  required_fields = %w[token secret password credentials logs stdout stderr environment]
  failures << "redaction must be mandatory metadata-only and bounded" unless redaction["required"] == true && redaction["mode"] == "metadata-only" && redaction["max_bytes"].is_a?(Integer) && redaction["max_bytes"].between?(256, 262_144) && redaction["forbidden_fields"].is_a?(Array) && required_fields.all? { |field| redaction["forbidden_fields"].include?(field) }
  replay = spec.fetch("replay")
  failures << "replay must be bounded read-only and preserve IDs" unless replay["enabled"] == true && replay["max_events"].is_a?(Integer) && replay["max_events"].between?(1, 100_000) && replay["max_window_seconds"].is_a?(Integer) && replay["max_window_seconds"].between?(1, 30 * 24 * 60 * 60) && replay["preserve_event_id"] == true && replay["side_effects"] == "forbidden" && replay["mode"] == "read_only"
rescue StandardError => e
  failures << "contract structure is invalid: #{e.message}"
end

begin
  behavior_raw = File.read(behavior_path)
  failures << "behavior evidence contains secret-shaped content" if behavior_raw.match?(secret)
  evidence = JSON.parse(behavior_raw)
  failures << "invalid behavior evidence identity" unless evidence.is_a?(Hash) && evidence["apiVersion"] == "events.leorunners.io/v1" && evidence["kind"] == "EventSinkBehaviorEvidence"
  failures << "invalid behavior metadata" unless evidence.dig("metadata", "version").to_s.match?(/\A\d+\.\d+\.\d+\z/)
  scenarios = evidence.fetch("scenarios")
  failures << "behavior evidence must contain exactly the required scenarios" unless scenarios.is_a?(Array) && scenarios.map { |item| item["name"] }.sort == %w[duplicate_delivery queue_overflow read_only_replay]
  duplicate = scenarios.find { |item| item["name"] == "duplicate_delivery" }
  delivery = duplicate.fetch("delivery")
  expected = duplicate.fetch("expected")
  failures << "duplicate delivery is not idempotent" unless delivery["attempts"] == 2 && delivery["persisted_records"] == 1 && delivery["processed_records"] == 1 && delivery["acknowledgements"] == 2 && expected["idempotent"] == true && expected["duplicate_suppressed"] == true && expected["ack_after_persist"] == true
  replay = scenarios.find { |item| item["name"] == "read_only_replay" }
  replay_data = replay.fetch("replay")
  replay_expected = replay.fetch("expected")
  failures << "replay evidence is not bounded read-only" unless replay_data["selected_events"] == replay_data["emitted_events"] && replay_data["selected_events"].is_a?(Integer) && replay_data["selected_events"] > 0 && replay_data["preserved_event_ids"] == true && replay_data["lifecycle_side_effects"] == 0 && replay_data["bounded"] == true && replay_expected["read_only"] == true && replay_expected["side_effects_forbidden"] == true
  queue = scenarios.find { |item| item["name"] == "queue_overflow" }
  queue_data = queue.fetch("queue")
  queue_expected = queue.fetch("expected")
  failures << "queue overflow evidence is inconsistent or blocks cleanup" unless queue_data["capacity"].is_a?(Integer) && queue_data["capacity"] > 0 && queue_data["submitted"] == queue_data["persisted"] + queue_data["dropped"] && queue_data["dropped"] > 0 && queue_data["drop_reason"] == "queue_full" && queue_data["cleanup_blocked"] == false && queue_expected["drop_measured"] == true && queue_expected["bounded"] == true && queue_expected["cleanup_unblocked"] == true
rescue StandardError => e
  failures << "behavior structure is invalid: #{e.message}"
end

if failures.empty?
  puts "PASS event sink contract: #{contract_path}"
  puts "PASS behavior evidence: #{behavior_path}"
  puts "event sink validation passed"
  exit 0
end
failures.each { |failure| warn "FAIL: #{failure}" }
exit 1
RUBY
status=$?
set -e
exit "$status"
