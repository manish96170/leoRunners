#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
report=
format=text
while [ "$#" -gt 0 ]; do
  case "$1" in
    --report) [ "$#" -ge 2 ] || { echo 'error: --report needs a path' >&2; exit 2; }; report=$2; shift 2 ;;
    --format) [ "$#" -ge 2 ] || { echo 'error: --format needs a value' >&2; exit 2; }; format=$2; shift 2 ;;
    --root) [ "$#" -ge 2 ] || { echo 'error: --root needs a path' >&2; exit 2; }; root=$(CDPATH= cd -- "$2" && pwd); shift 2 ;;
    --help|-h) printf '%s\n' 'usage: validate.sh [--root PATH] [--report PATH] [--format text|json]'; exit 0 ;;
    *) echo "error: unknown argument: $1" >&2; exit 2 ;;
  esac
done
case "$format" in text|json) ;; *) echo 'error: --format must be text or json' >&2; exit 2 ;; esac

command -v ruby >/dev/null 2>&1 || { echo 'error: Ruby is required' >&2; exit 2; }
exec ruby - "$root" "$report" "$format" <<'RUBY'
require "json"
require "fileutils"

root, report, format = ARGV
results = []
gaps = []

def text(path)
  File.read(path, mode: "rb", encoding: "UTF-8")
rescue StandardError
  ""
end

def present(root, relative)
  File.file?(File.join(root, relative)) || File.directory?(File.join(root, relative))
end

def check(results, name, status, message, environment_gated = false)
  code = status == "PASS" ? 0 : (status == "WARN" ? 2 : 1)
  results << {"name" => name, "status" => status, "exit_code" => code, "message" => message.to_s.gsub(/\s+/, " ").strip[0, 512], "environment_gated" => environment_gated}
end

def secret?(value)
  value.match?(/AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{12,}|BEGIN [A-Z ]*PRIVATE KEY|xox[baprs]-[A-Za-z0-9-]+|fixture-secret-token/i)
end

required_docs = [
  "Master Codex Prompt — Intelligent Multi-Cloud Ephemeral CI Platform.md",
  "docs/architecture.md", "docs/production-readiness.md", "docs/security-hardening.md",
  "docs/final-readiness.md", "docs/cloud-validation.md",
  "docs/controlled-validation-evidence.md", "docs/live-validation-operations.md",
  "docs/preflight-scope-operations.md", "docs/observability-operations.md"
]
check(results, "required-docs", "PASS", "required architecture, security, validation, and operations documents are present") if required_docs.all? { |path| present(root, path) }
check(results, "required-docs", "BLOCKED", "required documentation contracts are missing") unless required_docs.all? { |path| present(root, path) }

plan = text(File.join(root, "PLAN.md"))
manifest = JSON.parse(text(File.join(root, "validation/final-audit/phase-manifest.v1.json"))) rescue {}
target_phase = manifest.fetch("target_completed_phase", 53)
current_match = plan.match(/Current milestone:\s*Phase\s+(\d+)/i)
tracked_max = plan.scan(/Phase\s+(\d+)/i).flatten.map(&:to_i).max || 0
marker_gaps = manifest.fetch("phase_markers", {}).any? { |_phase, paths| paths.any? { |path| !present(root, path) } }
if current_match && tracked_max >= target_phase && !marker_gaps
  check(results, "phase-tracking", "PASS", "tracking files cover the audited phase range and all phase markers are present")
else
  check(results, "phase-tracking", "BLOCKED", "tracking files do not identify all audited completed phases")
end

security = text(File.join(root, "security/policy.v1.yaml"))
kube = text(File.join(root, "deploy/kubernetes/deployment.yaml"))
config = text(File.join(root, "controller/internal/config/config.go"))
required_security = [
  /defaultDecision:\s*deny/, /mutateSecurityControlsAutomatically:\s*false/,
  /allowControllerCredentials:\s*false/, /allowCloudCredentials:\s*false/,
  /imdsv2-required/, /privileged:\s*false/, /hostNetwork:\s*false/,
  /hostPathMounts:\s*false/, /enabledByDefault:\s*false/
]
unsafe_security = /privileged:\s*true|allowAll(?:IPv4|IPv6):\s*true|0\.0\.0\.0\/0|BEGIN [A-Z ]*PRIVATE KEY/i
secure_runtime = kube.include?("runAsNonRoot: true") && kube.include?("readOnlyRootFilesystem: true") && kube.include?("allowPrivilegeEscalation: false") && kube.include?("- ALL")
secure = required_security.all? { |pattern| security.match?(pattern) } && !security.match?(unsafe_security) && secure_runtime && config.include?('AIConfig{Provider: "disabled"')
check(results, "security-boundaries", secure ? "PASS" : "BLOCKED", secure ? "security policy, non-root runtime controls, and advisory AI defaults are present" : "required fail-closed security boundary markers are incomplete")

cloud = text(File.join(root, "tools/cloud-validation/validate.sh"))
configmap = text(File.join(root, "deploy/kubernetes/configmap.yaml"))
tfvars = text(File.join(root, "terraform/environments/dev/variables.tf"))
no_live = cloud.include?("allow_live=0") && cloud.include?("live_action=none") && configmap.include?("CAPACITY_PROVIDER: fake") && configmap.include?("GITHUB_JIT_ENABLED: \"false\"") && configmap.include?("AI_ENABLED: \"false\"") && configmap.include?("EXTENSIONS_ENABLED: \"false\"") && tfvars.match?(/variable\s+"enable_observability"[\s\S]*?default\s*=\s*false/) && config.match?(/optionalBoolDefault\(env, "AI_ENABLED", false\)/) && config.match?(/optionalBoolDefault\(env, "EXTENSIONS_ENABLED", false\)/)
check(results, "no-live-defaults", no_live ? "PASS" : "BLOCKED", no_live ? "live providers and optional features are opt-in by default" : "a live or externally connected default could not be proven")

coverage = JSON.parse(text(File.join(root, "validation/final-audit/coverage-manifest.v1.json"))) rescue {"checks" => []}
coverage_ok = coverage.fetch("checks", []).all? { |entry| present(root, entry.fetch("path")) }
check(results, "test-validator-coverage", coverage_ok ? "PASS" : "BLOCKED", coverage_ok ? "controller tests and offline validator entrypoints are present" : "one or more required test or validator entrypoints are missing")

todo = text(File.join(root, "TODO.md"))
handoff = text(File.join(root, "HANDOFF.md"))
unchecked = todo.scan(/^\s*- \[ \]/).length
known_gap_lines = handoff.lines.count { |line| line.match?(/^\s*- /) && line.match?(/not|unavailable|outstanding|remain/i) }
if unchecked > 0 || known_gap_lines > 0
  gaps << {"name" => "known-environment-gaps", "status" => "environment-gated", "message" => "operator credentials, live cloud, image, cluster, or production evidence gaps remain"}
  check(results, "known-environment-gaps", "WARN", "known environment-gated work remains outside this offline audit", true)
else
  check(results, "known-environment-gaps", "PASS", "no known environment-gated gaps were recorded")
end

%w[aws gcloud gh docker kubectl packer terraform].each do |tool|
  unless system("command -v #{tool} >/dev/null 2>&1")
    gaps << {"name" => "missing-#{tool}", "status" => "environment-gated", "message" => "#{tool} is unavailable for provider-specific validation"}
  end
end

status = if results.any? { |entry| entry["status"] == "BLOCKED" }
           "BLOCKED"
         elsif results.any? { |entry| entry["status"] == "WARN" }
           "WARN"
         else
           "PASS"
         end
summary = {"pass" => results.count { |entry| entry["status"] == "PASS" }, "warnings" => results.count { |entry| entry["status"] == "WARN" }, "blocked" => results.count { |entry| entry["status"] == "BLOCKED" }}
document = {
  "apiVersion" => "final-audit.leorunners.io/v1",
  "kind" => "FinalAuditReport",
  "metadata" => {"name" => "leo-runners-final-audit", "version" => "1.0.0"},
  "spec" => {
    "status" => status, "read_only" => true, "live_mutation_invoked" => false,
    "redaction" => {"status" => "redacted", "raw_output_included" => false, "secrets_scanned" => true},
    "checks" => results, "environment_gaps" => gaps, "summary" => summary
  }
}
serialized = JSON.pretty_generate(document) + "\n"
raise "audit report contains secret-shaped content" if secret?(serialized)
if report && !report.empty?
  FileUtils.mkdir_p(File.dirname(report))
  temporary = "#{report}.tmp.#{$$}"
  File.open(temporary, "w", 0o600) { |file| file.write(serialized) }
  File.rename(temporary, report)
end
if format == "json"
  STDOUT.write(serialized)
else
  results.each { |entry| STDOUT.puts "#{entry['status']} #{entry['name']} #{entry['message']}" }
  STDOUT.puts "#{status} final-audit summary pass=#{summary['pass']} warn=#{summary['warnings']} blocked=#{summary['blocked']}"
end
exit(status == "PASS" ? 0 : (status == "WARN" ? 2 : 1))
RUBY
