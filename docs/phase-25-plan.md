# Phase 25 Plan

Phase 25 defines the approved evidence workflow for controlled live
validation. Live validation is a bounded, disposable operation that proves
provider connectivity and selected lifecycle behavior. It is never a normal
startup dependency, an extension authorization path, or a substitute for
unit, integration, and offline validation.

## Objectives

- Require explicit approval and confirmation before provisioning anything.
- Record provider checkpoints, cleanup proof, and observability verification
  in one versioned evidence record.
- Keep reports useful for audit and diagnosis without storing secrets,
  credentials, raw webhook payloads, bootstrap material, or unbounded labels.
- Abort quickly on unsafe scope, missing prerequisites, unexpected resources,
  failed cleanup, or signals that could affect shared infrastructure.
- Make rollback and evidence retention reproducible for AWS, GCP, and GitHub.

## Approval and preparation

The operator must create an approved validation request before running the
live command. The request identifies the operator, approver, change or issue,
environment, provider, region or project, disposable resource prefix, tenant
and repository scope, maximum resource count, maximum runtime, budget limit,
cleanup deadline, notification route, and rollback owner.

The request must explicitly state whether the run is read-only, provider
connectivity-only, or lifecycle validation. The default is read-only. A
lifecycle run requires the documented confirmation flag and must use a
disposable scope. Production, shared runner pools, customer data, and
long-lived credentials are prohibited.

Before provisioning, the operator records the repository revision, tool
versions, manifest/configuration hashes, and a redacted preflight result.
Credentials are loaded through the approved local or CI secret mechanism and
are never placed in command arguments, files under the evidence directory,
logs, or screenshots.

## Approved workflow

1. Review and approve the validation request and rollback plan.
2. Run offline schema, policy, security, Terraform, and controller checks.
3. Run the cloud-validation preflight without provisioning and inspect the
   redacted report.
4. Confirm provider identity, disposable scope, limits, timeouts, and cleanup
   hooks. Stop if any value is missing or broader than approved.
5. Reconfirm the explicit live-validation confirmation flag immediately
   before the first mutating call.
6. Execute the smallest provider checkpoint sequence and capture each result
   using the evidence schema below.
7. Verify logs, metrics, alarms, and dashboard signals for the validation
   operation.
8. Terminate and delete every validation resource, then run independent
   cleanup discovery and reconciliation.
9. Close the run only after cleanup proof, alarm recovery, and report
   redaction checks pass. Mark the run failed when any proof is incomplete.

The controller must remain able to reconcile ordinary state throughout the
run. Validation resources must be distinguishable by approved tags or labels,
and cleanup must be idempotent so a retry cannot delete unrelated resources.

## Provider checkpoints

Each provider records only the checkpoints applicable to the selected mode.
Identifiers are hashed or truncated in the evidence record unless an exact
value is required in a restricted operational system.

### AWS

- identity and region preflight succeeds for the approved account scope;
- launch request is accepted with the approved image, instance profile,
  network, tags, and resource limits;
- the runner reaches the expected lifecycle state and emits its bounded
  readiness signal;
- registration or callback verification succeeds only when explicitly in
  scope; and
- termination completes, the instance is absent from the approved discovery
  query, and no validation-tagged dependency remains.

### GCP

- identity, project, and zone preflight match the approved request;
- instance creation uses the approved image, service account, network, and
  labels;
- readiness and registration checkpoints complete within the deadline; and
- deletion completes and a label-scoped discovery query returns no remaining
  validation resource.

### GitHub

- repository and token scope are verified without recording the token;
- webhook signature, JIT configuration, runner group, and registration
  checkpoints are validated when selected;
- only the approved repository or organization scope is touched; and
- the ephemeral runner is removed or expires, with no unexpected runner,
  webhook, label, or registration residue.

A provider checkpoint is `pass` only when its positive assertion and its
  scope assertion both pass. An API response alone is not cleanup proof.

## Evidence schema

The durable report is a versioned JSON document with this shape:

```json
{
  "schema_version": "controlled-validation-evidence.v1",
  "run_id": "cv-20260913-001",
  "status": "passed",
  "mode": "lifecycle",
  "provider": "aws",
  "started_at": "2026-09-13T10:00:00Z",
  "finished_at": "2026-09-13T10:04:00Z",
  "operator": "redacted-operator-id",
  "approval": {"ticket": "redacted-ticket", "approved_by": "redacted"},
  "scope": {"region_or_project": "redacted", "resource_prefix": "lr-cv-20260913"},
  "revision": {"git_sha": "0123456789ab", "config_sha256": "redacted"},
  "checkpoints": [{"name": "identity", "status": "passed", "started_at": "...", "duration_ms": 42}],
  "observability": {"logs": "passed", "metrics": "passed", "alarms": "passed", "dashboard": "passed"},
  "cleanup": {"status": "passed", "deadline": "...", "discovery_remaining": 0, "reconcile_passed": true},
  "abort_reason": null,
  "artifacts": [{"kind": "redacted-log", "sha256": "redacted", "retention_until": "..."}]
}
```

Every checkpoint includes a stable name, status, timestamps, duration, and a
short sanitized reason. Failed or aborted runs include `abort_reason` and the
last completed checkpoint. Evidence must not include tokens, secret values,
raw event bodies, user-data, private URLs, full account or project IDs,
customer identifiers, or unrestricted provider responses.

## Redaction and retention

Redact before persistence, transmission, display, or attachment. Apply key
allowlists for fields, secret-pattern scrubbing, bounded string lengths,
bounded arrays, and low-cardinality dimensions. Hash values only when the
correlation value is needed; hashing is not permission to retain a secret.
The report checksum may be retained for integrity, but not the unredacted
source report.

Retain the evidence record, approval, configuration hash, checkpoint summary,
cleanup proof, and redacted logs for the approved operational retention
period. Delete temporary command output and provider response dumps sooner.
Access to evidence is owner-only and read access is audited.

## Observability verification

During the run, verify that structured logs contain the run ID and bounded
provider/mode/status dimensions, while excluding payloads and secrets.
Verify that lifecycle counters and duration metrics increase once per
checkpoint and that p99 duration is present when the sample threshold is met.

Verify configured alarms and dashboard panels by observing the expected
validation signal, recording alarm state transitions and dashboard query
results, and then confirming recovery after cleanup. Alarm actions must be
notification-only. Missing data, stale data, unexpected dimensions, or a
dashboard that cannot distinguish the validation run from shared traffic is a
failure of evidence quality.

## Abort criteria

Abort before or during the run when any of the following occurs:

- approval, confirmation, identity, scope, credentials, or cleanup hook is
  missing or does not match the request;
- a requested resource, image, network, role, repository, or label is outside
  the approved boundary;
- a quota, budget, count, deadline, or concurrency limit is reached;
- an unexpected resource, event, callback, runner, or customer impact is
  detected;
- secrets or unredacted payloads appear in output;
- readiness, registration, telemetry, or provider health checkpoints time
  out or return an unsafe response; or
- cleanup cannot be proven by bounded discovery before the deadline.

On abort, stop creating resources, preserve only the redacted evidence, run
the approved cleanup path, and escalate to the rollback owner. Never broaden
permissions or scope to make a failed run pass.

## Rollback and exit criteria

Rollback disables the validation trigger, revokes temporary access, restores
the last known-good configuration, and removes validation resources through
the idempotent provider cleanup path. Re-run discovery, reconciliation, and
alarm recovery checks after rollback. Any remaining resource is an incident,
not a successful validation.

Phase 25 is complete for a run when the approval and hashes are recorded,
every selected provider checkpoint has bounded evidence, observability has
been verified, all resources have cleanup proof, the report is redacted and
retained according to policy, and the rollback owner has accepted the result.
