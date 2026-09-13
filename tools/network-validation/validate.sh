#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
matrix="$root/network/reachability-matrix.v1.json"
json_output=false

usage() { printf '%s\n' 'usage: validate.sh [--matrix PATH] [--json]'; }
while (($#)); do
  case "$1" in
    --matrix) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; matrix=$2; shift 2 ;;
    --json) json_output=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done
[[ -f "$matrix" ]] || { printf 'matrix not found: %s\n' "$matrix" >&2; exit 2; }

NETWORK_VALIDATION_JSON="$json_output" ruby -rjson -e '
path = ARGV.fetch(0)
begin
  doc = JSON.parse(File.read(path))
rescue JSON::ParserError => e
  warn "invalid JSON: #{e.message}"
  exit 2
end
spec = doc["spec"] || {}
egress = spec["egress"] || {}
dns = spec["dns"] || {}
errors = []
errors << "apiVersion must be network.leorunners.io/v1" unless doc["apiVersion"] == "network.leorunners.io/v1"
errors << "kind must be ReachabilityMatrix" unless doc["kind"] == "ReachabilityMatrix"
errors << "private subnet class is required" unless spec["subnetClass"] == "private"
errors << "DNS must use an approved resolver" unless %w[cloud-resolver approved-forwarder private-zone].include?(dns["mode"])
errors << "DNS resolver target is required" if dns["required"] && Array(dns["resolverTargets"]).empty?
allowed_modes = %w[nat vpc-endpoint hybrid approved-proxy]
errors << "egress mode is not approved" unless allowed_modes.include?(egress["mode"])
errors << "selected egress mode is not in allowedModes" unless Array(egress["allowedModes"]).include?(egress["mode"])
%w[maxDnsMs maxConnectMs maxRequestMs].each { |key| errors << "#{key} must be a positive bounded number" unless egress[key].is_a?(Numeric) && egress[key] > 0 && egress[key] <= 60000 }
errors << "TLS is required" unless egress["tlsRequired"] == true
destinations = Array(spec["destinations"])
errors << "at least one destination is required" if destinations.empty?
ids = destinations.map { |d| d["id"] }
errors << "destination IDs must be unique" unless ids.uniq.length == ids.length
destinations.each do |d|
  id = d["id"] || "?"
  errors << "destination #{id} has an invalid hostname" unless d["hostname"].is_a?(String) && d["hostname"] =~ /\A[a-zA-Z0-9][a-zA-Z0-9.-]+\z/ && !d["hostname"].include?("..")
  errors << "destination #{id} must use HTTPS/443" unless d["protocol"] == "https" && d["port"] == 443
  errors << "destination #{id} has an unapproved egress mode" unless Array(d["allowedEgressModes"]).include?(egress["mode"])
  errors << "destination #{id} path is invalid" unless d["path"].is_a?(String) && d["path"].start_with?("/") && !d["path"].include?("//")
end
isolation = spec["isolation"] || {}
errors << "inbound access must be deny" unless isolation["inbound"] == "deny"
errors << "public IPs must be disabled" unless isolation["publicIp"] == false
errors << "IMDSv2 is required" unless isolation["metadataImdsv2"] == true
errors << "untrusted forks need a separate trust boundary" unless isolation["untrustedForks"] == "separate-trust-boundary"
cost = spec["cost"] || {}
errors << "currency must be USD" unless cost["currency"] == "USD"
errors << "cost labels for selected egress mode are required" unless Array((cost["labels"] || {})[egress["mode"]]).any?
errors << "billing reconciliation is required" unless cost["billingReconciliationRequired"] == true
if errors.empty?
  puts(JSON.generate({status: "PASS", matrix: path})) unless ENV["NETWORK_VALIDATION_JSON"] == "true"
  exit 0
else
  warn(JSON.generate({status: "FAIL", matrix: path, errors: errors}))
  exit 1
end
' "$matrix"

if [[ "$json_output" == true ]]; then
  printf '%s\n' '{"status":"PASS","mode":"offline","mutations":false}'
else
  printf '%s\n' 'network reachability matrix passed (offline, no mutations)'
fi
