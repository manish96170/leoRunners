# Phase 26 Plan

Phase 26 operationalizes controlled live validation without making live
execution an automatic deployment step. It defines how operators capture
preflight evidence, produce reports compatible with
`controlled-validation-evidence.v1`, obtain approval, execute bounded
checkpoints, and close or escalate incomplete runs.

## Objectives

- Capture a redacted, reproducible preflight record before any mutating call.
- Make preflight and live results compatible with the Phase 25 evidence
  schema.
- Require independent approval gates for identity, scope, budget, and cleanup.
- Treat every selected checkpoint and cleanup assertion as explicit evidence.
- Make interrupted or incomplete runs visible and non-passing by default.
- Keep live execution operator-approved because it can create billable
  resources, affect external integrations, and expose shared operational
  boundaries.

## Preflight evidence

The operator first creates a run identifier and records the approved request,
rollback owner, repository revision, tool versions, configuration hashes,
provider, region or project, resource prefix, limits, and cleanup deadline.
The preflight command runs in read-only mode and captures only sanitized
results. Credentials are loaded by the approved secret mechanism and are not
written to the report, command line, logs, or attachments.

The preflight record must prove:

- provider identity matches the approved account, project, organization, or
  repository scope;
- the selected mode and requested resources are within the approved limits;
- required images, networks, roles, quotas, APIs, and endpoints are present;
- cleanup discovery and reconciliation hooks are available;
- log, metric, alarm, and dashboard queries can identify the run with bounded
  dimensions; and
- the evidence directory and retention policy are owner-controlled.

Preflight failure stops the run before approval of a mutating step. A
successful preflight is evidence of readiness, not authorization to provision.

## Schema-compatible reports

Preflight and execution results are written into the same
`controlled-validation-evidence.v1` envelope. Preflight-only work uses a
non-passing status until the live workflow is explicitly approved and closed.
Checkpoint names remain stable across providers; provider-specific details
belong in bounded sanitized reasons or approved artifact references.

Example preflight-compatible report:

```json
{
  "schema_version": "controlled-validation-evidence.v1",
  "run_id": "cv-20260913-002",
  "status": "aborted",
  "mode": "connectivity",
  "provider": "aws",
  "started_at": "2026-09-13T11:00:00Z",
  "finished_at": "2026-09-13T11:01:00Z",
  "operator": "redacted-operator-id",
  "approval": {"ticket": "redacted-ticket", "approved_by": "redacted"},
  "scope": {"region_or_project": "redacted", "resource_prefix": "lr-cv-20260913"},
  "revision": {"git_sha": "0123456789ab", "config_sha256": "redacted"},
  "checkpoints": [
    {"name": "preflight_identity", "status": "passed", "started_at": "2026-09-13T11:00:10Z", "duration_ms": 42},
    {"name": "preflight_cleanup", "status": "failed", "started_at": "2026-09-13T11:00:30Z", "duration_ms": 80, "reason": "cleanup hook unavailable"}
  ],
  "observability": {"logs": "passed", "metrics": "passed", "alarms": "skipped", "dashboard": "skipped"},
  "cleanup": {"status": "skipped", "deadline": "2026-09-13T11:10:00Z", "discovery_remaining": 0, "reconcile_passed": false},
  "abort_reason": "preflight failed before mutating approval",
  "artifacts": []
}
```

Reports must be validated after redaction and before publication. Raw provider
responses, credentials, user-data, webhook bodies, and unbounded identifiers
are never attachments to a Phase 26 report.

## Approval gates

The workflow has four gates:

1. **Readiness gate:** preflight passes and the report is schema-valid,
   redacted, and tied to the approved revision.
2. **Scope gate:** an operator and approver confirm provider identity, region
   or project, repository and tenant scope, resource count, runtime, budget,
   and disposable naming or tagging.
3. **Mutation gate:** immediately before the first mutating API call, the
   operator supplies the explicit live-validation confirmation and records the
   approval decision and timestamp.
4. **Closure gate:** every selected checkpoint, observability assertion, and
   cleanup proof passes, or the run is marked `failed`, `aborted`, or
   `rolled_back` and escalated.

Approval does not permit scope expansion. A changed revision, provider,
identity, resource boundary, or cleanup deadline requires a new preflight and
approval decision.

## Checkpoints and cleanup

Record each checkpoint with stable name, status, start time, duration, and a
bounded sanitized reason. A checkpoint passes only when both its positive
provider result and its approved-scope assertion pass. A successful API call
alone is insufficient.

Cleanup is a required checkpoint sequence: invoke idempotent deletion,
perform fresh tag or label-scoped discovery, and run reconciliation or an
equivalent consistency check. Record query time, attempt count, remaining
count, and the deadline. Any unavailable or throttled discovery result is
unknown and therefore not proof of zero resources.

## Incomplete runs

An interrupted, timed-out, signal-terminated, or partially observed run is
never marked `passed`. Stop new mutations, preserve the redacted partial
report, execute the approved cleanup path, and continue bounded discovery and
reconciliation until the deadline. Mark the report `aborted` when execution
stopped before completion, `failed` when an assertion failed, or
`rolled_back` when rollback completed.

If cleanup or evidence redaction cannot be proven, open an incident reference,
notify the rollback owner, and retain only sanitized evidence. Do not retry a
partial run under the same identity until resource ownership and cleanup state
are understood.

## Why live execution remains operator-approved

Live validation crosses boundaries that offline tests cannot safely simulate:
it may create billable cloud resources, consume quotas, invoke provider APIs,
register or remove GitHub runners, and interact with shared logs and alarms.
Its safety depends on current credentials, account scope, quotas, network
state, and cleanup conditions that can change after code is built. Keeping
the final mutation decision with an authorized operator preserves explicit
accountability and provides a deliberate stop point when preflight evidence
does not match the approved request.

Phase 26 is complete when the workflow, report compatibility, approval gates,
checkpoint rules, incomplete-run handling, and operator responsibility are
documented and reviewed against the Phase 25 evidence contract.
