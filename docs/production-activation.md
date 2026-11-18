# Production Activation Operations

This runbook governs a possible transition from read-only evidence to a
bounded live-validation exercise. It is an approval workflow, not an automatic
deployment. All commands and defaults remain read-only unless an operator has
approved the exact request and the live-validation boundary has been entered.

## 1. Prepare the request

Create an activation request with a unique request ID and record:

- immutable repository revision and artifact/image digests;
- provider identity, account or project, region or zone, GitHub repository,
  runner group, and resource prefix;
- maximum resource count, instance class, lifetime, and estimated cost;
- SHA-256 hashes of the scope file, configuration, scripts, manifests, and
  evidence inputs;
- start and expiry deadlines in UTC;
- operator, approver, cleanup owner, and escalation contact; and
- the exact live actions that may be performed.

Keep credentials outside the request. Never use a wildcard account, project,
repository, region, resource prefix, or resource limit.

## 2. Run the read-only handoff

From a clean checkout of the requested revision, run the repository’s offline
validators and the cloud-validation preflight in read-only mode. Supply the
approved scope and write the evidence report to a new owner-only path. The
handoff reviewer checks that:

- provider identities are present but secrets are absent;
- observed identities and locations exactly match the request;
- scope, configuration, script, manifest, and evidence hashes match;
- request start and expiry deadlines remain valid;
- required checks are `PASS`, with accepted environment warnings recorded;
- no provider mutation, Terraform apply, image build, runner registration,
  webhook mutation, or cleanup command ran; and
- the report is schema-valid, redacted, atomic, and linked to the request ID.

The handoff may be marked `READY` only after a second reviewer confirms those
checks. `WARN` is not approval. `BLOCKED`, failed cleanup evidence, a missing
report, or an incomplete checkpoint set stops the process.

## 3. Review and approve

The accountable operator reviews the full evidence set, verifies the hashes
again, and records an authenticated approval. The approval must bind:

- request ID;
- reviewed revision and scope hash;
- permitted provider operations;
- expiry and cleanup deadlines;
- operator and approver identities; and
- rollback and escalation contacts.

Approval is invalid if it is copied to another revision, scope, account,
deadline, or operation set. A shell flag or environment variable can satisfy a
confirmation prompt only as an additional guard; it cannot replace the
authenticated approval record.

## 4. Later live-validation boundary

Only after the handoff is `READY` and approval is recorded may the operator
start the separate live-validation procedure. That procedure must:

1. re-check the request, hashes, identities, scope, and deadlines;
2. acquire any required lease or fencing token;
3. create only the approved bounded ephemeral resources;
4. emit redacted lifecycle, startup, workload, and cleanup checkpoints;
5. stop on timeout, cancellation, scope drift, unexpected resources, or any
   failed security invariant; and
6. discover and remove resources using the exact request identity, then verify
   that none remain.

The normal controller, CI, and local validation paths must not silently enter
this boundary. No live validation is implied by this document or by passing
offline tests.

## 5. Abort and cleanup

Abort immediately when identity or scope differs, a hash changes, the request
expires, a bound cannot be enforced, a secret is exposed, approval is absent,
or an action falls outside the approved operation set. If a resource already
exists, stop new assignments and execute the approved cleanup path. Record:

- the abort reason and timestamp;
- resources discovered by request identity;
- cleanup attempts and provider outcomes;
- remaining-resource count; and
- escalation owner and next action.

Cleanup is successful only when discovery confirms zero matching resources and
the final evidence report records the result. A cleanup failure leaves the
activation failed and requires escalation; it must not be hidden by a passing
application or workload result.

## 6. Retain and close

Store the request, approval, preflight handoff, hashes, checkpoint reports,
redacted provider evidence, cleanup proof, and final disposition together.
Use versioned schemas, bounded references, atomic writes, and owner-only file
permissions. Exclude tokens, private keys, raw secrets, raw webhook bodies,
and unbounded provider payloads.

Close the activation only after the operator confirms the final status and
cleanup proof. Retain the evidence for the applicable change-management,
security, and incident-retention period. Do not delete or shorten retention
while an incident, audit, cleanup dispute, rollback review, or approval
challenge remains open.

## Decision record

Record one final disposition:

- `NOT_READY`: preflight or handoff is incomplete;
- `BLOCKED`: a required invariant, scope check, hash, approval, or security
  boundary failed;
- `ABORTED`: live validation began but stopped and cleanup completed or is
  under escalation;
- `PASSED`: the approved live exercise completed, cleanup was proven, and the
  evidence set is retained; or
- `WARN`: only for documented environment gaps that do not authorize live
  mutation and require an explicit operator plan.

This runbook does not authorize production deployment or claim that live
validation has taken place.
