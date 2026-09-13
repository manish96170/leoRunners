# Controlled Validation Evidence

This runbook defines the evidence package for an approved live validation
run. It applies to AWS, GCP, and GitHub provider checkpoints and complements
the offline checks in `tools/cloud-validation`. A live run is optional,
bounded, disposable, and separately authorized.

## Evidence package

Store one redacted `controlled-validation-evidence.v1` report per run. Keep
related artifacts in the same owner-controlled location:

- approval and change reference;
- repository revision, tool versions, and configuration/manifest hashes;
- preflight result and selected validation mode;
- one record per provider checkpoint;
- log, metric, alarm, and dashboard verification summaries;
- cleanup discovery and reconciliation proof;
- abort or rollback record when applicable; and
- checksums and retention expiry for attached redacted artifacts.

The report is the source of truth for the run. Attachments are supporting
evidence and must be referenced by checksum, not embedded as unrestricted
provider output.

## Required report fields

```text
schema_version, run_id, status, mode, provider
started_at, finished_at, operator, approval
scope, revision, checkpoints, observability, cleanup
abort_reason, artifacts
```

Use stable statuses: `passed`, `failed`, `aborted`, or `rolled_back`.
Checkpoint statuses are `passed`, `failed`, `skipped`, or `timed_out`.
Each checkpoint records its name, status, start time, duration, and a bounded
sanitized reason. The scope records only the approved region/project and
resource prefix in redacted or truncated form.

## Collection procedure

1. Create the approved request and assign an operator and rollback owner.
2. Capture the preflight report before any mutating API call.
3. Confirm identity, scope, resource count, time limit, budget, and cleanup
   deadline against the approval.
4. Capture checkpoint summaries as they complete; do not save raw provider
   responses as evidence.
5. Query logs and metrics by run ID and bounded dimensions. Record counts,
   durations, alarm transitions, dashboard panel results, and query windows.
6. Trigger cleanup and record both API completion and independent
   label/tag-scoped discovery results.
7. Run reconciliation or equivalent cleanup verification until the approved
   deadline. Record zero remaining resources or an incident reference.
8. Redact and validate the report, calculate its checksum, and publish only
   the approved evidence package.

## Provider checkpoint summary

Record the following minimum assertions when selected:

| Provider | Checkpoints | Cleanup proof |
| --- | --- | --- |
| AWS | identity, launch, readiness, registration/callback when approved, termination | tag-scoped discovery finds zero instances and dependencies |
| GCP | identity, create, readiness, registration when approved, delete | label-scoped discovery finds zero instances and dependencies |
| GitHub | repository/token scope, webhook/JIT when approved, registration, removal/expiry | runner and temporary registration residue are absent |

For every provider, record the scope assertion separately from the API result.
An HTTP 2xx or successful SDK response does not establish that the resource
was created in the approved scope or that cleanup is complete.

## Redaction checklist

Before persistence or sharing, verify that the report contains no:

- access key, bearer token, webhook secret, private key, session credential,
  bootstrap token, or secret value;
- raw webhook/event body, user-data, command output, or unrestricted SDK
  response;
- full account, project, organization, repository, customer, or runner
  identity where a truncated or hashed value is sufficient;
- private endpoint, signed URL, IP address, or provider error containing
  sensitive request context; or
- unbounded label, branch, job, workflow, tenant, or arbitrary payload value.

Use a field allowlist, pattern scrubbing, maximum lengths, bounded arrays, and
low-cardinality dimensions before writing the file. Search the final report
and attachments for secret-shaped values. If a secret is found, stop sharing,
revoke or rotate it, mark the run failed, and preserve only the sanitized
incident reference.

## Logs, metrics, alarms, and dashboards

Verification must show that the run is observable without changing shared
service behavior:

- logs contain the run ID, provider, mode, checkpoint, and status, with no
  payload or secret fields;
- counters and duration metrics increase for the selected checkpoints, using
  bounded dimensions only;
- p99 duration is recorded when enough samples exist, otherwise the report
  records that the sample was insufficient;
- expected alarm state transitions and missing-data behavior are recorded;
- the dashboard shows the validation window and can distinguish it from
  shared traffic; and
- after cleanup, alarms recover and the dashboard shows no continuing
  validation activity.

Alarm actions must remain notification-only. Do not use evidence collection
to authorize termination, capacity changes, credential grants, or security
policy changes.

## Cleanup proof

Cleanup is complete only when all three checks pass:

1. The provider cleanup call returns success or an explicitly idempotent
   not-found result.
2. A fresh tag/label or repository-scoped discovery query finds zero
   validation resources and no temporary dependency.
3. Controller reconciliation or the provider's equivalent consistency check
   confirms no pending validation state before the cleanup deadline.

Record the query time, scope, result count, cleanup attempt count, and any
  remaining-resource incident reference. A timeout, throttled discovery
  query, or unavailable cleanup signal is `failed`, not zero.

## Abort and rollback

Abort for scope drift, identity mismatch, missing confirmation, limit breach,
unexpected resources or callbacks, secret leakage, provider timeout, unsafe
health signals, or unproven cleanup. Stop new mutations immediately and use
the approved cleanup path.

Rollback restores the prior configuration, disables the validation trigger,
revokes temporary access, removes disposable resources, and repeats discovery
and reconciliation. Notify the rollback owner when any resource remains,
alarm does not recover, or the report cannot be redacted. The run status must
be `aborted`, `failed`, or `rolled_back`; never `passed` with an open cleanup
or evidence gap.

## Retention and review

Retain the redacted report, approval, hashes, checkpoint summaries, cleanup
proof, and incident references for the approved operational period. Delete
temporary output and raw provider responses earlier. Restrict access to the
owner-controlled evidence location and audit reads.

The reviewer signs off only when scope, checkpoints, observability, redaction,
cleanup, and rollback evidence are complete. A successful provider API call
without independent cleanup proof is insufficient for approval.
