# Extensions Operations

This runbook covers production registration and operation of optional Leo
Runners extensions. Extensions are untrusted consumers of versioned events.
The controller owns lifecycle state, scheduling, provider operations,
credentials, security decisions, and cleanup.

## Registration and enablement

1. Review the versioned extension manifest. Confirm stable ID, contract
   version, owner, source or image digest, capabilities, runbook, output
   limits, and retention expiry.
2. Confirm global policy is disabled or observe-only while registering. Record
   the registry snapshot, configuration hash, approver, and change window.
3. Enable the exact extension ID for one approved tenant. Tenant enablement
   requires both the global policy and the tenant allowlist; neither setting
   is a wildcard grant.
4. Send synthetic events and verify the extension receives only the intended
   tenant's events. Confirm disabled tenants and extensions receive none.
5. Review advisory decisions before expanding enablement. Roll out by tenant
   or workload class, retaining the previous configuration for rollback.

Registration does not grant access to cloud accounts, IAM roles, GitHub
tokens, runner credentials, provider clients, or controller mutation APIs.
Capabilities describe behavior; they are not executable permissions.

## Runtime contract

Each enabled extension has its own bounded queue and worker. Record the
configured queue size, handler timeout, report buffer, shutdown timeout, and
output limits in the release record. A full queue drops the event for that
consumer and increments a metric; it must not block webhook acknowledgement,
scheduling, provisioning, or cleanup.

Handlers must honor cancellation. A slow handler is timed out. A returned
error or panic is counted and isolated. During shutdown, dispatch is canceled
and workers are given only the configured deadline. A close timeout is an
extension incident, not a reason to prolong controller shutdown indefinitely.

## Signals and alerts

Track received, queued, dropped, succeeded, failed, timed-out, and panicked
executions, plus dropped execution reports. Use only bounded dimensions such
as extension ID, outcome, event type, and environment. Never use tenant,
repository, workflow, run, job, runner, request, commit, URL, token, or raw
error text as a metric dimension.

Alert on:

- sustained queue drops or queue saturation;
- timeout, panic, or failure rate above the reviewed baseline;
- dropped reports or missing execution telemetry;
- unexpected registration, version, digest, or tenant-policy changes; and
- an extension consuming events after it was disabled.

Alerts are notification-only. Do not connect them to runner termination,
capacity mutation, credential grants, security-policy changes, or automatic
retries without a separately reviewed controller change.

## First response

1. Record extension ID, contract version, tenant scope, environment, region,
   first-seen time, alert dimensions, and deployment digest.
2. Check controller health, event-bus health, durable state, reconciliation,
   and core lifecycle metrics. Confirm runner provisioning and cleanup are
   unaffected.
3. Inspect bounded extension counters and correlated redacted logs. Compare
   received versus queued, dropped, failed, timed-out, panicked, and reported
   counts.
4. Determine whether the issue is configuration, tenant scope, event rate,
   queue saturation, handler latency, dependency failure, panic, or telemetry
   loss. Do not replay or delete events before preserving evidence.
5. Disable the affected extension or tenant if impact is possible. Verify that
   core event processing and reconciliation continue with the extension off.

## Triage

### Queue drops or saturation

Check event rate, worker throughput, handler duration, and the configured
queue bound. Disable the extension for affected tenants while preserving the
drop count. Do not increase the queue without a capacity review and a memory
bound. Replay only from the durable event source, with a bounded window and
deduplication.

### Timeouts, errors, or panics

Group failures by extension ID, finite outcome, and event type. Inspect the
extension release digest and dependency health. Keep the handler timeout
finite. Roll back or disable the consumer, then run a synthetic event and
confirm the failure is isolated. Escalate repeated panics or context
violations to the extension owner.

### Missing or inconsistent telemetry

Check controller health, event subscription, reporter buffer, log ingestion,
metric namespace, and deployment configuration. A missing report can mean
the consumer ran but telemetry was dropped; compare execution counters with
report counters. Treat unexplained telemetry loss as an observability issue,
not proof that no extension work occurred.

### Advisory policy rejection

Preserve the rejected output in redacted form and inspect its action names.
Only annotations, explanations, links, and recommendations are allowed.
Lifecycle, provider, credential, and security actions must remain rejected.
Do not weaken policy to make an extension appear healthy.

## Rollback and recovery

1. Stop the rollout and disable the affected extension or tenant allowlist.
2. Preserve the manifest, registry hash, configuration diff, metrics, redacted
   logs, advisory decisions, and event IDs required for investigation.
3. Restore the last known-good registry/configuration and digest through the
   approved deployment mechanism. Do not edit durable lifecycle state to hide
   extension symptoms.
4. Verify no extension remains subscribed when disabled, then run a bounded
   synthetic event, reconciliation pass, and cleanup check.
5. Re-enable one tenant only after failure, timeout, queue-drop, and telemetry
   signals are stable for the agreed observation window.

If the extension cannot be isolated or core lifecycle behavior is affected,
stop new intake and escalate to the controller and security owners. Use the
controller's normal idempotent cleanup path for any confirmed orphan; an
extension rollback must never terminate resources directly.

## Retention and privacy

Retain manifests, configuration approvals, bounded execution metrics, and
redacted operational logs according to the environment policy. Extension
records must include an expiry and evidence class. Delete expired advisory
data and replay material through the approved owner-only process. Do not
retain raw webhook payloads, bootstrap data, credentials, tokens, repository
secrets, or arbitrary extension input as a side channel.

Preserve incident evidence only for the approved investigation window, with
access restricted to the owning operators. Record extensions, tenants,
retention decisions, deletion evidence, and exceptions in the release or
incident record.

## Validation

Run offline checks from the repository root:

```sh
(cd controller && go test ./...)
(cd controller && go test -race ./...)
(cd controller && go vet ./...)
(cd controller && go build ./...)
sh extensions/validate.sh
```

Before production enablement, test duplicate and unsupported registration,
disabled tenant isolation, queue-full drops, handler timeout, panic recovery,
report loss, shutdown timeout, advisory rejection, and rollback. Use synthetic
events and a disposable tenant. Confirm that the controller acknowledges and
cleans up work with every extension disabled.

## Completion checklist

- [ ] Manifest owner, digest, version, capabilities, limits, and runbook are
      approved.
- [ ] Global and tenant enablement are explicit, scoped, and reversible.
- [ ] Queue, handler, report, output, and shutdown limits were exercised.
- [ ] Failure, timeout, panic, drop, and telemetry alerts have owners and
      tested notification routes.
- [ ] Advisory-only policy rejected forbidden actions in a negative test.
- [ ] Retention expiry and deletion evidence are recorded.
- [ ] Rollback and tenant-isolation evidence is attached to the release.
