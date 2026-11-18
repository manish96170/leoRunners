#!/bin/sh
set -eu

# Offline Phase 48 contract gate. The Go tests use httptest and fakes; this
# script intentionally never invokes curl, gh, or a cloud/GitHub endpoint.
tool_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$tool_dir/../.." && pwd)

fail() {
    printf 'github-validation: FAIL: %s\n' "$*" >&2
    exit 1
}

require_text() {
    file=$1
    text=$2
    grep -F -- "$text" "$file" >/dev/null 2>&1 || fail "missing contract in $file: $text"
}

github_dir=$repo_dir/controller/internal/github
runners_dir=$repo_dir/controller/internal/runners
bootstrap=$repo_dir/bootstrap/runner/bootstrap.sh

[ -f "$bootstrap" ] || fail "bootstrap script is missing"
[ -x "$bootstrap" ] || fail "bootstrap script is not executable"

# API request safety: labels and positive runner groups are mandatory, tokens
# are sourced per request, and retry/cancellation remain context-bound.
require_text "$github_dir/jit.go" 'tokenSource BearerTokenSource'
require_text "$github_dir/jit.go" 'len(request.Labels) == 0'
require_text "$github_dir/jit.go" 'request.RunnerGroupID == nil'
require_text "$github_dir/jit.go" 'Authorization", "Bearer "+token'
require_text "$github_dir/retry.go" 'ctx.Err()'
require_text "$github_dir/runners.go" 'runner.ID == runnerID && runner.Name == runnerName'
require_text "$github_dir/runners.go" 'strings.EqualFold(runner.Status, "online")'
require_text "$github_dir/events.go" 'DeliveryID string'
require_text "$github_dir/events.go" 'NormalizeWorkflowJobHeaders'
require_text "$github_dir/scope.go" 'ErrScopeDenied'
require_text "$github_dir/scope.go" 'repository %q is not approved'
require_text "$github_dir/scope.go" 'fork repositories are denied'
require_text "$runners_dir/runners.go" 'ErrAssignmentInProgress'
require_text "$runners_dir/runners.go" 'beginAssignment(request.Job.ID)'
require_text "$runners_dir/runners.go" 'RegistrationCleanup'
require_text "$runners_dir/runners.go" 'RemoveRunner(cleanupCtx'
require_text "$runners_dir/runners.go" 'configured runner group does not match the approved policy'

# The transient JIT value can cross only into provider metadata/bootstrap. It
# must not be persisted in lifecycle data or logged by the assignment service.
require_text "$runners_dir/runners.go" 'spec.Metadata[MetadataJITConfig] = jit.EncodedConfig'
require_text "$runners_dir/runners.go" 'github_runner_id'
if grep -n 'EncodedConfig' "$runners_dir/runners.go" | grep -E 'persistRegistration|Data:' >/dev/null 2>&1; then
    fail "JIT encoded config appears in registration persistence"
fi

# Bootstrap must consume exactly one source, use the one-time --jitconfig path,
# forward cancellation, and remove staging material on every exit path.
require_text "$bootstrap" 'source_count'
require_text "$bootstrap" 'rm -f "$jit_config_file"'
require_text "$bootstrap" 'trap '\''forward_signal TERM'\'' TERM'
require_text "$bootstrap" '"$runner_run_script" --jitconfig "$jit_config"'
require_text "$bootstrap" 'unset jit_config'
if grep -E 'config\.sh|--url|--token|GITHUB_TOKEN|GH_TOKEN|gh auth' "$bootstrap" >/dev/null 2>&1; then
    fail "bootstrap contains persistent registration or credential handling"
fi

# Keep the production boundary fail-closed in the checked-in fixtures too.
for fixture in "$tool_dir/fixtures/scope.safe" "$tool_dir/fixtures/scope.fork" "$tool_dir/fixtures/scope.repository" "$tool_dir/fixtures/scope.label"; do
    [ -f "$fixture" ] || fail "missing scope fixture: $fixture"
done
require_text "$tool_dir/fixtures/scope.safe" 'expected=PASS'
for fixture in "$tool_dir/fixtures/scope.fork" "$tool_dir/fixtures/scope.repository" "$tool_dir/fixtures/scope.label"; do
    require_text "$fixture" 'expected=DENY'
done

(
    cd "$repo_dir/controller"
    go test ./internal/github ./internal/runners ./internal/security
)
"$repo_dir/bootstrap/runner/test.sh"

printf 'github-validation: PASS\n'
