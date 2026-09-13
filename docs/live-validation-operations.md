# Live Validation Operations

This runbook describes how to operate an approved controlled validation run.
It assumes the offline checks and evidence contract from Phase 25 have
already been reviewed. Live execution remains opt-in and operator-approved.

## 1. Capture preflight evidence

Create a unique `run_id` and an owner-controlled evidence directory. Record
the approved ticket, operator, rollback owner, revision, configuration and
manifest hashes, provider, region or project, resource prefix, limits, and
cleanup deadline.

Run the cloud preflight in read-only mode. Capture a redacted report using the
`controlled-validation-evidence.v1` envelope. The report must include
`schema_version`, `run_id`, `status`, `mode`, `provider`, timestamps, approval,
scope, revision, checkpoints, observability, cleanup, `abort_reason`, and
`artifacts`. Use bounded checkpoint names and sanitized reasons; reference
attachments by checksum rather than embedding provider output.

Stop when identity, scope, quota, image, network, role, credential, cleanup,
or observability checks do not match the approval. A passing preflight means
the request is ready for review; it does not authorize a mutating call.

## 2. Obtain approval

The operator and approver review the preflight report and confirm:

- provider identity and region or project;
- repository, organization, tenant, and network scope;
- disposable tags or labels and maximum resource count;
- maximum runtime, budget, timeout, and cleanup deadline;
- selected connectivity or lifecycle checkpoints; and
- rollback owner, notification route, and evidence retention.

Immediately before the first mutating call, the operator rechecks these
values and records the explicit live-validation confirmation. Re-run preflight
and obtain approval again if the revision, scope, provider, limits, or cleanup
plan changes.

## 3. Execute bounded checkpoints

Capture one record for every selected checkpoint. Each record contains a
stable name, `passed`, `failed`, `skipped`, or `timed_out` status, timestamps,
duration, and a short sanitized reason. Record the scope assertion separately
from the provider API result.

Use the smallest approved sequence for the selected provider. Do not broaden
permissions, add resources, retry indefinitely, or alter shared capacity to
make a checkpoint pass. Registration, callback, or webhook checks are
performed only when listed in the approval.

## 4. Verify observability

Query the validation window using the run ID and bounded provider, mode, and
status dimensions. Record log and metric counts, duration results, alarm state
transitions, dashboard panel results, and the query window. Do not attach raw
logs, event bodies, user-data, credentials, or unrestricted SDK responses.

Confirm alarms recover after cleanup. Alarm actions remain notification-only;
evidence collection cannot authorize termination, capacity changes,
credential grants, or security-policy changes.

## 5. Prove cleanup

Cleanup has three required assertions:

1. The provider deletion call succeeds or returns an explicitly idempotent
   not-found result.
2. Fresh tag, label, or repository-scoped discovery finds zero validation
   resources and temporary dependencies.
3. Controller reconciliation or the provider equivalent confirms no pending
   validation state before the deadline.

Record cleanup attempts, query time, result count, reconciliation status, and
any incident reference. A throttled, timed-out, or unavailable discovery
query is not zero and cannot close the run.

## 6. Handle incomplete runs

Signals, timeouts, provider errors, lost telemetry, interrupted commands, and
partial cleanup produce a non-passing run. Stop new mutations immediately,
preserve the redacted partial report, invoke the approved cleanup path, and
continue bounded discovery until the cleanup deadline.

Use `aborted` when execution stopped before completion, `failed` when an
assertion failed, and `rolled_back` when the rollback actions completed. If
remaining resources, unreconciled state, alarm recovery failure, or
unredacted evidence is possible, notify the rollback owner and open an
incident. Never relabel an incomplete run as `passed`.

## 7. Close and retain evidence

Redact, schema-validate, checksum, and publish the report only after cleanup
and observability recovery are verified. Retain the approved report,
preflight, approval, hashes, checkpoint summaries, cleanup proof, and
incident references for the approved period. Delete raw provider responses
and temporary command output earlier. Restrict access to the owner-controlled
location and audit reads.

The final reviewer signs off only when scope, checkpoints, observability,
redaction, cleanup, and rollback evidence are complete. Live validation is
operator-approved because it can create billable resources, consume quotas,
change external runner state, and affect shared operational signals. Offline
validation cannot establish those current-world properties, so no automated
startup or normal deployment path may silently initiate a live run.
