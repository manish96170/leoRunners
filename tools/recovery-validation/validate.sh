#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
schema="$root/../../validation/recovery/recovery-evidence.schema.v1.json"
default="$root/../../validation/recovery/recovery-evidence.v1.json"
fixture=${1:-$default}
command -v ruby >/dev/null 2>&1 || { printf '%s\n' 'validate.sh: ruby is required' >&2; exit 2; }
[ -f "$schema" ] || { printf 'file not found: %s\n' "$schema" >&2; exit 2; }
[ -f "$fixture" ] || { printf 'file not found: %s\n' "$fixture" >&2; exit 2; }

set +e
ruby -rjson -rtime - "$schema" "$fixture" <<'RUBY'
schema_path, fixture_path = ARGV
secret = /Bearer\s+\S+|\bgh[pousr]_[A-Za-z0-9_]+\b|\bAKIA[0-9A-Z]{16}\b|BEGIN [A-Z ]*PRIVATE KEY|(?:password|secret|token|access[_-]?key)\s*[:=]\s*\S+/i
failures = []

begin
  schema = JSON.parse(File.read(schema_path))
  failures << "schema identity is invalid" unless schema["$id"].to_s.end_with?("recovery-evidence.v1.json") && schema["type"] == "object"
rescue StandardError
  failures << "schema could not be parsed"
end

raw = File.read(fixture_path)
failures << "evidence contains secret-shaped content" if raw.match?(secret)
begin
  evidence = JSON.parse(raw)
  failures << "invalid evidence identity" unless evidence.is_a?(Hash) && evidence["apiVersion"] == "recovery.leorunners.io/v1" && evidence["kind"] == "RecoveryEvidence"
  metadata = evidence.fetch("metadata")
  failures << "invalid run identity" unless metadata["run_id"].to_s.match?(/\Arecovery-[a-z0-9-]{3,64}\z/)
  started = Time.iso8601(metadata.fetch("started_at"))
  finished = Time.iso8601(metadata.fetch("finished_at"))
  failures << "timestamps are not ordered" unless finished >= started
  spec = evidence.fetch("spec")
  window = spec.fetch("recovery_window_seconds")
  failures << "recovery window is outside 1..300 seconds" unless window.is_a?(Integer) && window.between?(1, 300)

  file = spec.fetch("state_recovery").fetch("file")
  failures << "file backup recovery failed" unless file["primary_snapshot"] == "corrupt" && file["backup_snapshot"] == "valid" && file["backup_restored"] == true && file["records_before"] == file["records_after"] && file["revision_preserved"] == true
  dynamo = spec.fetch("state_recovery").fetch("dynamodb")
  failures << "DynamoDB recovery did not reject or preserve revisions" unless dynamo["records_before"] == dynamo["records_after"] && dynamo["conditional_revision_conflicts"] == dynamo["conflicts_rejected"] && dynamo["recovered_without_overwrite"] == true && dynamo["provider_calls"] == 0

  replay = spec.fetch("event_replay")
  failures << "event replay counts are inconsistent" unless replay["events_read"] == replay["unique_events"] + replay["duplicate_events"] && replay["events_applied"] == replay["unique_events"] && replay["side_effects_after_replay"] == replay["side_effects_before_replay"] && replay["idempotent"] == true && replay["event_ids_preserved"] == true && replay["provider_calls"] == 0

  fencing = spec.fetch("lease_fencing")
  failures << "lease fencing is not monotonic and fail-closed" unless fencing["new_token"] > fencing["old_token"] && fencing["stale_writes_attempted"] == fencing["stale_writes_rejected"] && fencing["fencing_enforced"] == true

  reaping = spec.fetch("stale_runner_reaping")
  failures << "stale runner reaping is incomplete or unsafe" unless reaping["scanned"] == reaping["stale"] + reaping["active_protected"] && reaping["reaped"] == reaping["stale"] && reaping["reap_idempotent"] == true && reaping["provider_calls"] == 0

  takeover = spec.fetch("takeover")
  failures << "two-replica takeover is not fenced or capacity-conserving" unless takeover["replicas"] == 2 && takeover["lease_expired"] == true && takeover["takeover_acquired"] == true && takeover["old_owner_actions_after_fence"] == 0 && takeover["duplicate_assignments"] == 0 && takeover["capacity_before"] == takeover["capacity_after"] && takeover["capacity_conserved"] == true

  evidence_block = spec.fetch("evidence")
  failures << "evidence is not redacted, bounded, or owner-only" unless evidence_block["status"] == "PASS" && evidence_block["redacted"] == true && evidence_block["secret_fields"] == 0 && evidence_block["raw_payloads"] == 0 && evidence_block["permissions"] == "0600" && evidence_block["atomic_write"] == true && evidence_block["bounded"] == true && evidence_block["destructive_operations"] == 0 && evidence_block["cloud_calls"] == 0
rescue StandardError => e
  failures << "evidence structure is invalid: #{e.message}"
end

if failures.empty?
  puts "PASS recovery evidence: #{fixture_path}"
  puts "recovery validation passed"
  exit 0
end
failures.each { |failure| warn "FAIL: #{failure}" }
exit 1
RUBY
status=$?
set -e
exit "$status"
