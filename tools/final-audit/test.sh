#!/usr/bin/env bash
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
command -v ruby >/dev/null 2>&1 || { echo 'Ruby is required' >&2; exit 2; }
sh -n "$root/tools/final-audit/validate.sh"
bash -n "$root/tools/final-audit/validate.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/leo-final-audit.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

set +e
"$root/tools/final-audit/validate.sh" --report "$tmp/report.json" --format json >"$tmp/output.json" 2>"$tmp/error"
status=$?
set -e
[ "$status" -eq 0 ] || [ "$status" -eq 1 ] || [ "$status" -eq 2 ]
test -s "$tmp/report.json"
test -s "$tmp/output.json"
test "$(stat -f '%Lp' "$tmp/report.json" 2>/dev/null || stat -c '%a' "$tmp/report.json")" = 600
! grep -Eiq 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]+|BEGIN .*PRIVATE KEY|fixture-secret-token' "$tmp/report.json"
ruby -rjson - "$root/validation/final-audit/schema.v1.json" "$tmp/report.json" <<'RUBY'
schema = JSON.parse(File.read(ARGV[0]))
data = JSON.parse(File.read(ARGV[1]))
raise "wrong apiVersion" unless data["apiVersion"] == "final-audit.leorunners.io/v1"
raise "wrong kind" unless data["kind"] == "FinalAuditReport"
spec = data.fetch("spec")
raise "audit is not read-only" unless spec["read_only"] == true && spec["live_mutation_invoked"] == false
raise "missing required audit checks" unless spec["checks"].length >= 6
raise "invalid status" unless %w[PASS WARN BLOCKED].include?(spec["status"])
raise "report is not redacted" unless spec.dig("redaction", "status") == "redacted" && spec.dig("redaction", "raw_output_included") == false
raise "schema contract missing" unless schema["$id"].include?("final-audit")
puts "final audit report contract passed"
RUBY

fixture=$(mktemp -d "$tmp/fixture.XXXXXX")
mkdir -p "$fixture/validation/final-audit" "$fixture/docs" "$fixture/security" "$fixture/deploy/kubernetes" "$fixture/controller/internal/config" "$fixture/tools/cloud-validation" "$fixture/terraform/environments/dev" "$fixture/tools/config-validation" "$fixture/tools/github-validation" "$fixture/tools/ami-benchmark" "$fixture/tools/measurement-validation" "$fixture/tools/multicloud-comparison" "$fixture/tools/event-sink-validation" "$fixture/tools/terraform-validation"
cp "$root/validation/final-audit/phase-manifest.v1.json" "$fixture/validation/final-audit/phase-manifest.v1.json"
cp "$root/validation/final-audit/coverage-manifest.v1.json" "$fixture/validation/final-audit/coverage-manifest.v1.json"
for file in "Master Codex Prompt — Intelligent Multi-Cloud Ephemeral CI Platform.md" docs/architecture.md docs/production-readiness.md docs/security-hardening.md docs/final-readiness.md docs/cloud-validation.md docs/controlled-validation-evidence.md docs/live-validation-operations.md docs/preflight-scope-operations.md docs/observability-operations.md; do mkdir -p "$fixture/$(dirname "$file")"; : >"$fixture/$file"; done
cat >"$fixture/PLAN.md" <<'EOF'
## Current milestone: Phase 53 final audit
### Completed
- Phase 53
EOF
cat >"$fixture/security/policy.v1.yaml" <<'EOF'
defaultDecision: deny
mutateSecurityControlsAutomatically: false
allowControllerCredentials: false
allowCloudCredentials: false
imdsv2-required: true
privileged: false
hostNetwork: false
hostPathMounts: false
enabledByDefault: false
EOF
cat >"$fixture/deploy/kubernetes/deployment.yaml" <<'EOF'
runAsNonRoot: true
readOnlyRootFilesystem: true
allowPrivilegeEscalation: false
- ALL
EOF
cat >"$fixture/controller/internal/config/config.go" <<'EOF'
AIConfig{Provider: "disabled"}
optionalBoolDefault(env, "AI_ENABLED", false)
optionalBoolDefault(env, "EXTENSIONS_ENABLED", false)
EOF
cat >"$fixture/tools/cloud-validation/validate.sh" <<'EOF'
allow_live=0
live_action=none
EOF
cat >"$fixture/deploy/kubernetes/configmap.yaml" <<'EOF'
CAPACITY_PROVIDER: fake
GITHUB_JIT_ENABLED: "false"
AI_ENABLED: "false"
EXTENSIONS_ENABLED: "false"
EOF
cat >"$fixture/terraform/environments/dev/variables.tf" <<'EOF'
variable "enable_observability" { default = false }
EOF
cat >"$fixture/TODO.md" <<'EOF'
# TODO
EOF
cat >"$fixture/HANDOFF.md" <<'EOF'
# Handoff
EOF
for file in tools/config-validation/test.sh tools/config-validation/validate.sh tools/cloud-validation/test.sh tools/cloud-validation/validate.sh tools/github-validation/test.sh tools/github-validation/validate.sh tools/ami-benchmark/test.sh tools/measurement-validation/test.sh tools/measurement-validation/validate.sh tools/multicloud-comparison/test.sh tools/multicloud-comparison/validate.sh tools/event-sink-validation/test.sh tools/event-sink-validation/validate.sh tools/terraform-validation/test.sh tools/terraform-validation/validate.sh; do : >"$fixture/$file"; done
cat >"$fixture/tools/cloud-validation/validate.sh" <<'EOF'
allow_live=0
live_action=none
EOF
set +e
"$root/tools/final-audit/validate.sh" --root "$fixture" >/dev/null 2>&1
fixture_status=$?
set -e
[ "$fixture_status" -eq 0 ] || { echo 'expected clean audit fixture to pass' >&2; exit 1; }
printf '%s\n' 'final-audit tests passed'
