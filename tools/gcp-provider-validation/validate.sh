#!/usr/bin/env bash
set -Eeuo pipefail

tool_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(CDPATH= cd -- "$tool_dir/../.." && pwd)
contract=${GCP_PROVIDER_CONTRACT_FILE:-$repo_root/validation/gcp-provider/provider-contract.v1.json}

usage() {
  printf '%s\n' 'Usage: validate.sh [--contract PATH]'
  printf '%s\n' 'Offline by default; never calls GCP or reads credentials.'
}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --contract) [ "$#" -ge 2 ] || { echo '--contract requires PATH' >&2; exit 2; }; contract=$2; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done
[ -f "$contract" ] || { echo "contract not found: $contract" >&2; exit 2; }
command -v ruby >/dev/null 2>&1 || { echo 'ruby is required' >&2; exit 2; }

ruby -rjson - "$contract" <<'RUBY'
path = ARGV.fetch(0)
begin
  raw = File.read(path)
  contract = JSON.parse(raw)
rescue JSON::ParserError => e
  abort "GCP provider validation failed: invalid JSON: #{e.message}"
end
fail_contract = ->(message) { abort "GCP provider validation failed: #{message}" }
fail_contract.call("unexpected top-level fields") unless (contract.keys - %w[apiVersion kind metadata spec]).empty?
fail_contract.call("apiVersion/kind") unless contract["apiVersion"] == "gcp-provider.leorunners.io/v1" && contract["kind"] == "ComputeRunnerProviderContract"
metadata = contract["metadata"] || {}
spec = contract["spec"] || {}
fail_contract.call("metadata version") unless metadata["version"].is_a?(String) && metadata["version"].match?(/\A\d+\.\d+\.\d+\z/)

scope = spec["scope"] || {}
fail_contract.call("project") unless scope["project"].is_a?(String) && scope["project"].match?(/\A[a-z][a-z0-9-]{4,28}[a-z0-9]\z/)
fail_contract.call("zone") unless scope["zone"].is_a?(String) && scope["zone"].match?(/\A[a-z]+-[a-z]+[0-9]-[a-z]\z/)
fail_contract.call("region/zone consistency") unless scope["region"].is_a?(String) && scope["zone"].start_with?("#{scope["region"]}-")

template = spec["instanceTemplate"] || {}
fail_contract.call("instance template") unless template["selfLink"].is_a?(String) && template["selfLink"].match?(%r{\Ahttps://www\.googleapis\.com/compute/v1/projects/[a-z][a-z0-9-]{4,28}[a-z0-9]/global/instanceTemplates/[a-z][a-z0-9-]{0,62}\z})
fail_contract.call("pinned instance template") unless template["versionPinned"] == true && template["fingerprint"].is_a?(String) && template["fingerprint"].match?(/\A[A-Za-z0-9_-]{8,128}\z/)

ownership = spec["ownership"] || {}
labels = ownership["requiredLabels"] || {}
required_labels = %w[leo-runners-managed-by leo-runners-runner-id leo-runners-job-id leo-runners-owner]
fail_contract.call("ephemeral ownership") unless ownership["managedBy"] == "leo-runners" && ownership["owner"] == "controller" && ownership["ephemeral"] == true && required_labels.all? { |key| labels[key].is_a?(String) && !labels[key].empty? }
fail_contract.call("label ownership mismatch") unless labels["leo-runners-managed-by"] == ownership["managedBy"] && labels["leo-runners-owner"] == ownership["owner"]

service_account = spec["serviceAccount"] || {}
fail_contract.call("service account") unless service_account["email"].is_a?(String) && service_account["email"].match?(/\A[a-z][a-z0-9-]{5,28}[a-z0-9]@[a-z][a-z0-9-]{4,28}[a-z0-9]\.iam\.gserviceaccount\.com\z/)
fail_contract.call("service account scope") unless service_account["scopes"].is_a?(Array) && service_account["scopes"].length.between?(1, 8) && service_account["scopes"].all? { |scope_name| scope_name.is_a?(String) && scope_name.start_with?("https://www.googleapis.com/auth/") }

network = spec["network"] || {}
fail_contract.call("network") unless network["network"]&.match?(/\Aprojects\/[a-z][a-z0-9-]{4,28}[a-z0-9]\/global\/networks\/[a-z][a-z0-9-]{0,62}\z/) && network["subnetwork"]&.match?(/\Aprojects\/[a-z][a-z0-9-]{4,28}[a-z0-9]\/regions\/[a-z]+-[a-z]+[0-9]\/subnetworks\/[a-z][a-z0-9-]{0,62}\z/)
fail_contract.call("network project/region consistency") unless network["network"].start_with?("projects/#{scope["project"]}/") && network["subnetwork"].include?("/regions/#{scope["region"]}/")
fail_contract.call("external IP policy") unless network["externalIp"] == false

request = spec["requestIdentity"] || {}
fail_contract.call("request ID") unless request["requestIdPresent"] == true && request["requestIdDeterministic"] == true && request["requestIdBounded"] == true
fail_contract.call("request ID format") unless request["format"] == "run-id"

readiness = spec["readiness"] || {}
fail_contract.call("readiness status") unless readiness["readyStatus"] == "RUNNING" && readiness["statusReadBeforeReady"] == true && readiness["provisioningStatuses"].is_a?(Array) && readiness["provisioningStatuses"].include?("PROVISIONING") && readiness["terminalStatuses"].is_a?(Array) && readiness["terminalStatuses"].include?("TERMINATED")

idempotency = spec["idempotency"] || {}
fail_contract.call("idempotent deletion") unless idempotency["deletionIdempotent"] == true && idempotency["notFoundDeletionSucceeds"] == true && idempotency["repeatedRequestSafe"] == true

timeouts = spec["timeouts"] || {}
limits = {"operationSeconds" => 3600, "readinessSeconds" => 3600, "deletionSeconds" => 1800}
fail_contract.call("bounded timeouts") unless timeouts["bounded"] == true && limits.all? { |key, max| timeouts[key].is_a?(Integer) && timeouts[key] > 0 && timeouts[key] <= max }

redaction = spec["redaction"] || {}
fail_contract.call("redaction") unless redaction["secretsAbsent"] == true && redaction["serviceAccountKeyAbsent"] == true && redaction["reportsRedacted"] == true
fail_contract.call("secret or wildcard content") if raw.match?(/gh[pousr]_[A-Za-z0-9_-]{8,}|AKIA[0-9A-Z]{16}|-----BEGIN|\*|Bearer\s+[A-Za-z0-9._-]{12,}|private_key|access_token/)
puts "GCP provider contract validation passed: #{metadata.fetch("name", "unnamed")}@#{metadata.fetch("version")}"
RUBY
