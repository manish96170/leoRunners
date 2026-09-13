#!/usr/bin/env bash
set -Eeuo pipefail

tool_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(CDPATH= cd -- "$tool_dir/../.." && pwd)
contract=${AWS_PROVIDER_CONTRACT_FILE:-$repo_root/validation/aws-provider/provider-contract.v1.json}

usage() {
  printf '%s\n' 'Usage: validate.sh [--contract PATH]'
  printf '%s\n' 'Offline by default; never calls AWS or reads credentials.'
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
  abort "AWS provider validation failed: invalid JSON: #{e.message}"
end
fail_contract = ->(message) { abort "AWS provider validation failed: #{message}" }
fail_contract.call("unexpected top-level fields") unless (contract.keys - %w[apiVersion kind metadata spec]).empty?
fail_contract.call("apiVersion/kind") unless contract["apiVersion"] == "aws-provider.leorunners.io/v1" && contract["kind"] == "EC2RunnerProviderContract"
metadata = contract["metadata"] || {}
spec = contract["spec"] || {}
fail_contract.call("metadata version") unless metadata["version"].is_a?(String) && metadata["version"].match?(/\A\d+\.\d+\.\d+\z/)
lt = spec["launchTemplate"] || {}
fail_contract.call("Launch Template ID") unless lt["id"].is_a?(String) && lt["id"].match?(/\Alt-[0-9a-f]{8,}\z/)
fail_contract.call("Launch Template version must be pinned") unless lt["versionPinned"] == true && lt["version"].is_a?(String) && lt["version"].match?(/\A[1-9][0-9]*\z/)
imds = spec["metadataOptions"] || {}
fail_contract.call("IMDSv2 required") unless imds["httpEndpoint"] == "enabled" && imds["httpTokens"] == "required"
ownership = spec["ownership"] || {}
tags = ownership["requiredTags"] || {}
required_tags = %w[leo-runners:managed-by leo-runners:runner-id leo-runners:job-id leo-runners:owner]
fail_contract.call("ephemeral ownership") unless ownership["managedBy"] == "leo-runners" && ownership["owner"] == "controller" && ownership["ephemeral"] == true && required_tags.all? { |key| tags[key].is_a?(String) && !tags[key].empty? }
fail_contract.call("tag ownership mismatch") unless tags["leo-runners:managed-by"] == ownership["managedBy"] && tags["leo-runners:owner"] == ownership["owner"]
profile = spec["instanceProfile"] || {}
fail_contract.call("exactly one instance profile") unless (profile["arn"].to_s.empty?) ^ (profile["name"].to_s.empty?)
fail_contract.call("instance profile ARN") if !profile["arn"].to_s.empty? && !profile["arn"].match?(/\Aarn:aws:iam::[0-9]{12}:instance-profile\/[A-Za-z0-9+=,.@_-]+\z/)
network = spec["network"] || {}
fail_contract.call("subnet") unless network["subnetId"].is_a?(String) && network["subnetId"].match?(/\Asubnet-[0-9a-f]{8,}\z/)
groups = network["securityGroupIds"]
fail_contract.call("security groups") unless groups.is_a?(Array) && groups.length.between?(1, 16) && groups.all? { |id| id.is_a?(String) && id.match?(/\Asg-[0-9a-f]{8,}\z/) }
idempotency = spec["idempotency"] || {}
fail_contract.call("idempotent ClientToken") unless idempotency["clientTokenPresent"] == true && idempotency["clientTokenDeterministic"] == true
fail_contract.call("idempotent termination") unless idempotency["terminationIdempotent"] == true && idempotency["notFoundTerminationSucceeds"] == true
readiness = spec["readiness"] || {}
fail_contract.call("readiness status") unless readiness["readyStatus"] == "running" && readiness["statusReadBeforeReady"] == true && readiness["provisioningStatuses"].is_a?(Array) && readiness["provisioningStatuses"].include?("pending") && readiness["terminalStatuses"].is_a?(Array) && readiness["terminalStatuses"].include?("terminated")
timeouts = spec["timeouts"] || {}
limits = {"operationSeconds" => 3600, "readinessSeconds" => 3600, "terminationSeconds" => 1800}
fail_contract.call("bounded timeouts") unless timeouts["bounded"] == true && limits.all? { |key, max| timeouts[key].is_a?(Integer) && timeouts[key] > 0 && timeouts[key] <= max }
redaction = spec["redaction"] || {}
fail_contract.call("redaction") unless redaction["secretsAbsent"] == true && redaction["jitConfigAbsent"] == true && redaction["reportsRedacted"] == true
fail_contract.call("secret or wildcard content") if raw.match?(/gh[pousr]_[A-Za-z0-9_-]{8,}|AKIA[0-9A-Z]{16}|-----BEGIN|\*|\$Latest|{{GITHUB_JIT_CONFIG_B64}}/)
puts "AWS provider contract validation passed: #{metadata.fetch("name", "unnamed")}@#{metadata.fetch("version")}"
RUBY
