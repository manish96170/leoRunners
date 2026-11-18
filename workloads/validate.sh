#!/bin/sh
set -eu

manifest=${1:-"$(dirname "$0")/workload-manifest.v1.yaml"}

if [ ! -f "$manifest" ]; then
  printf 'manifest not found: %s\n' "$manifest" >&2
  exit 1
fi

# Ruby's stdlib YAML parser keeps this usable without installing a package
# manager or coupling validation to a workload runtime.
ruby -ryaml - "$manifest" <<'RUBY'
path = ARGV.fetch(0)
data = YAML.safe_load(File.read(path), permitted_classes: [], aliases: false)
fail "manifest must be a mapping" unless data.is_a?(Hash)
fail "apiVersion must be workloads.leorunners.io/v1" unless data["apiVersion"] == "workloads.leorunners.io/v1"
fail "kind must be WorkloadManifest" unless data["kind"] == "WorkloadManifest"

metadata = data.fetch("metadata")
spec = data.fetch("spec")
fail "metadata.version must be semantic version text" unless metadata["version"].to_s.match?(/\A\d+\.\d+\.\d+\z/)
source = metadata.fetch("source")
fail "metadata.source is required" unless source.is_a?(Hash)
fail "metadata.source.repository must identify the fixed repository" unless source["repository"] == "github.com/manish96170/leoRunners"
fail "metadata.source.revision must be a full commit" unless source["revision"].to_s.match?(/\A[0-9a-f]{40}\z/)
fail "metadata.source.inspectedAt must be UTC date-time" unless source["inspectedAt"].to_s.match?(/\A\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z\z/)

workload = spec.fetch("workload")
fail "spec.workload must be a mapping" unless workload.is_a?(Hash)
fail "workload.id is required" unless workload["id"].to_s.match?(/\A[a-z0-9][a-z0-9-]{2,63}\z/)
fail "workload.workflow must be repository-relative" unless workload["workflow"].to_s.match?(/\A(?!\/|\.\.)[^\s]+\z/)
fail "workload.command must be bounded and secret-free" unless workload["command"].to_s.match?(/\A(?!.*(?:AWS_SECRET|TOKEN|PASSWORD|SECRET|curl\s+[^\s]+:\/\/))[^\r\n]{1,240}\z/i)
fail "workload.commandDigest must be sha256" unless workload["commandDigest"].to_s.match?(/\Asha256:[0-9a-f]{64}\z/)
fail "workload.provenance must be repository-defined" unless workload["provenance"] == "repository-defined"

runner = spec.fetch("runner")
%w[operatingSystem architecture cpu memoryGB diskGB imageProfile].each do |key|
  fail "spec.runner.#{key} is required" unless runner.key?(key)
end
fail "runner resources must be positive" unless %w[cpu memoryGB diskGB].all? { |key| runner[key].is_a?(Integer) && runner[key] > 0 }

expected = %w[
  node javascript react redux serverless-framework appsync-graphql lambda sqs
  dynamodb s3 terraform vtl cypress vitest biome eslint typescript docker rust
  go aws-cli
]
capabilities = spec.fetch("capabilities")
fail "capabilities must be a list" unless capabilities.is_a?(Array)
ids = capabilities.map { |item| item.fetch("id") }
fail "capability IDs must be unique" unless ids.uniq.length == ids.length
fail "missing capabilities: #{(expected - ids).join(', ')}" unless (expected - ids).empty?

valid_status = %w[absent observed inferred measured]
capabilities.each do |item|
  fail "unknown capability #{item['id']}" unless expected.include?(item["id"])
  %w[enabled required].each do |key|
    fail "#{item['id']}.#{key} must be boolean" unless [true, false].include?(item[key])
  end
  evidence = item.fetch("evidence")
  fail "#{item['id']}.evidence.status is invalid" unless valid_status.include?(evidence["status"])
  fail "#{item['id']} evidence sources must be a list" unless evidence["sources"].is_a?(Array)
end

expected_caches = %w[npm pnpm yarn cargo go-modules docker-layers terraform]
caches = spec.fetch("caches")
cache_ids = caches.map { |item| item.fetch("id") }
fail "missing caches: #{(expected_caches - cache_ids).join(', ')}" unless (expected_caches - cache_ids).empty?
fail "cache IDs must be unique" unless cache_ids.uniq.length == cache_ids.length
caches.each do |item|
  fail "unknown cache #{item['id']}" unless expected_caches.include?(item["id"])
  fail "cache #{item['id']} has invalid mode" unless %w[observe-only enabled].include?(item["mode"])
  %w[enabled required].each do |key|
    fail "cache #{item['id']}.#{key} must be boolean" unless [true, false].include?(item[key])
  end
  fail "cache #{item['id']} evidence is required" unless item["evidence"].is_a?(Hash)
  fail "cache #{item['id']} measurements are required" unless item["measurements"].is_a?(Hash)
end

validation = spec["validation"]
fail "validation section is required" unless validation.is_a?(Hash)
fail "validation.repositoryRevision must match source revision" unless validation["repositoryRevision"] == source["revision"]
puts "valid workload manifest: #{path}"
RUBY
