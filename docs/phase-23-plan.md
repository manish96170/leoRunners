# Phase 23 Plan

Phase 23 turns the extension boundary into a production operating contract.
Registration, tenant enablement, execution limits, failure isolation, and
advisory policy must be explicit before an extension can receive production
events. The core controller remains authoritative for lifecycle, security,
credentials, provider calls, and cleanup.

## Objectives

- Register versioned extensions with immutable identity, owner, compatibility,
  capabilities, and an explicit default-disabled state.
- Enable extensions per tenant through reviewed configuration, with separate
  tenant and extension gates and no implicit access to other tenants.
- Enforce finite per-extension queues, bounded handler and shutdown timeouts,
  bounded reports, and a documented backpressure policy.
- Isolate slow, failed, panicking, or unavailable consumers so they cannot
  block webhook acknowledgement, scheduling, provisioning, or cleanup.
- Emit low-cardinality execution metrics for received, queued, dropped,
  succeeded, failed, timed-out, panicked, and undelivered reports.
- Keep all extension output advisory. Recommendations and annotations require
  deterministic policy evaluation; lifecycle, security, credential, and
  provider actions are rejected and never executed from extension output.
- Provide triage, rollback, retention, and validation procedures for operators.

## Production registration

Every extension is registered from a reviewed, version-controlled manifest.
The manifest must identify a stable `extensionId`, display name, contract
version, owner, source or image digest, compatibility range, capabilities, and
runbook. Identity is immutable for the lifetime of a contract version; a
breaking change receives a new ID or version and a separate rollout.

Registration is configuration, not authorization. It does not grant provider,
IAM, GitHub, credential, network, or lifecycle access. The dispatcher must
reject duplicate IDs, unsupported versions, missing identity, and invalid
capability declarations before startup. The approved registry snapshot and
its hash belong in the deployment record.

## Tenant enablement

Extensions are disabled unless both the global policy and the tenant policy
are enabled. Each tenant allowlist names exact extension IDs. A tenant change
requires owner approval, a reason, effective time, output limits, retention,
and a rollback target. Do not use wildcard tenant or extension grants.

Events and outputs must carry a validated tenant identity. The extension layer
must not infer tenant access from repository, branch, job, or arbitrary event
fields. Disabled tenants and extensions receive no events. Tenant separation
is verified with a negative test before production rollout.

## Runtime limits and isolation

Use finite queues and one worker boundary per enabled extension. Configure and
record queue size, handler timeout, report buffer, shutdown timeout, maximum
output counts, and retention. When a queue is full, record a drop and continue
core processing; do not block the event publisher or retry indefinitely.

Handlers must honor their context. A timeout, error, panic, or close failure
increments the relevant metric and is reported without taking down unrelated
consumers. Extension code must not run provider calls or mutate controller
state directly. Shutdown cancels dispatch, waits only for the configured
deadline, and leaves the core controller able to reconcile resources.

## Metrics and alerts

Use the extension ID only as a bounded, reviewed dimension. Never put tenant,
repository, job, run, runner, request, commit, URL, token, or exception text
into metric dimensions. Required counters are:

- `extension_events_received_total`
- `extension_events_queued_total`
- `extension_events_dropped_total`
- `extension_executions_succeeded_total`
- `extension_executions_failed_total`
- `extension_executions_timed_out_total`
- `extension_executions_panicked_total`
- `extension_reports_dropped_total`

Alert on sustained drops, timeout or panic rate, failed execution rate,
report loss, queue saturation, and unexpected disabled-to-enabled changes.
Alerts notify operators only. They must not terminate runners, change tenant
policy, grant credentials, or invoke automated lifecycle remediation.

## Delivery sequence

1. Validate the manifest, schema version, owner, digest, capabilities, and
   redaction and retention rules offline.
2. Register the extension while globally and tenant disabled; record the
   registry snapshot and configuration diff.
3. Run contract, isolation, tenant-negative, timeout, panic, queue-full, and
   shutdown tests with synthetic events.
4. Enable one tenant in observe-only mode and confirm bounded metrics, logs,
   advisory policy decisions, and no lifecycle side effects.
5. Expand by tenant and workload class only after the agreed observation
   window; keep a tested configuration rollback available.
6. Record baseline event rate, queue utilization, execution latency, failure
   rate, retention expiry, and notification ownership.

## Exit criteria

- Registration and tenant enablement are reviewed, auditable, and reversible.
- Queue, handler, report, output, and shutdown bounds are configured and
  tested under saturation and failure.
- One failed or malicious consumer cannot affect core scheduling or cleanup.
- Metrics and alerts use approved low-cardinality dimensions and have owners,
  thresholds, runbooks, and notification tests.
- Advisory output is rejected when it requests lifecycle, security,
  credential, or provider authority.
- Retention, rollback, and read-only validation evidence is attached to the
  production release record.
