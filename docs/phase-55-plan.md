# Phase 55: Production Activation Boundary

## Objective

Define the controlled handoff from local implementation and read-only
validation to a later, operator-approved live-validation exercise. Phase 55
does not provision cloud resources, modify runners, change Terraform, or grant
production access. It records the approval and evidence contract required
before anyone may cross that boundary.

## Deliverables

- An activation request containing the reviewed revision, approved scope,
  content hash, deadline, owner, and rollback contact.
- A read-only preflight report and evidence handoff that can be reproduced from
  the same revision and scope.
- Explicit checks for scope, report hashes, expiration, and approval identity.
- Abort criteria that fail closed before any provider mutation.
- Retention and review rules for reports, cleanup proof, and approvals.
- A later operator-approved live-validation procedure, kept separate from the
  default local and CI paths.

## Activation request

An activation request is a bounded authorization, not a deployment command. It
must identify:

- request ID and creation time;
- requesting team, accountable operator, and escalation contact;
- exact repository revision and immutable artifact/image references;
- provider, account or project, region or zone, and GitHub repository scope;
- permitted resource prefix, resource count, instance class, and maximum
  lifetime;
- SHA-256 hashes for the approved scope, configuration, validation scripts,
  and evidence bundle;
- start deadline and expiry deadline in UTC;
- expected cleanup owner and cleanup deadline; and
- explicit statement that live mutation is not authorized until preflight
  succeeds and the operator records approval.

Requests with wildcards, missing hashes, open-ended deadlines, unbounded
resources, embedded credentials, or an unspecified cleanup owner are invalid.

## Read-only handoff

The handoff begins with a fresh checkout of the requested revision. The
operator runs the read-only cloud preflight and local release/evidence gates,
then stores the resulting reports with the request ID. The handoff must show:

1. provider identity was inspected without exposing credential material;
2. every target account, project, region, zone, repository, and runner group
   matches the approved scope;
3. the reviewed files and evidence inputs match their recorded SHA-256 hashes;
4. deadlines and resource limits are still valid at handoff time;
5. required validators pass, with environment-only warnings separately listed;
6. no live mutation, apply, registration, or cleanup command was invoked by
   the read-only step; and
7. the evidence report is complete, redacted, atomically written, and owned by
   the operator account.

The handoff status is `READY` only when all required checks pass. A `WARN`
status may identify an explicitly accepted environment gap, but it never
authorizes live mutation by itself. A `BLOCKED` or incomplete report ends the
handoff.

## Boundary and approval

Live validation is a separate, later operation. It requires a human operator
to review the complete handoff, confirm the exact scope and deadline again, and
record approval tied to the request ID and reviewed hashes. The approval must
state the permitted provider actions and cleanup deadline. Automation may
consume this approval only after validating its signature or equivalent
authenticated record; a filename, environment variable, or command-line
confirmation alone is not approval.

The live procedure must use the approved revision and scope without local
edits. It must create only bounded ephemeral resources, emit checkpoint
evidence, verify cleanup, and stop at the first failed invariant. Production
activation is never implied by a passing local test or by the existence of an
activation request.

## Abort criteria

Abort before live mutation when any of the following occurs:

- identity, provider, region, project, repository, runner group, or resource
  prefix does not match the approved scope;
- any reviewed file, image, manifest, or report hash differs;
- the request is expired, outside its start window, or missing a UTC deadline;
- resource count, lifetime, cost, or instance limits cannot be enforced;
- credentials or secret values appear in configuration, logs, reports, or
  command output;
- a validator is missing, returns `BLOCKED`, or cannot prove a required
  security boundary;
- operator approval is absent, ambiguous, unauthenticated, or references a
  different request or revision;
- cleanup ownership, discovery, or deadline is not recorded;
- the provider reports an unexpected resource, account, quota, or permission;
- cancellation, timeout, fencing, or duplicate-assignment behavior is
  observed; or
- any live action would exceed the approved operation set.

If an abort occurs after resource creation, stop new work, preserve redacted
evidence, discover resources by the exact request identity, and run the
approved cleanup path. Cleanup failure is itself a failed activation and must
be escalated immediately.

## Evidence retention

Retain the activation request, approval record, preflight report, hashes,
checkpoint evidence, provider responses after redaction, cleanup proof, and
final disposition as one immutable evidence set. Reports must:

- use the versioned evidence schema where applicable;
- contain request ID, revision, provider scope, timestamps, status, and
  bounded evidence references;
- exclude tokens, private keys, full secrets, raw webhook payloads, and
  unbounded provider responses;
- be written atomically with owner-only permissions; and
- remain queryable by request ID without using raw resource identifiers as
  unbounded telemetry dimensions.

The retention period is set by the operator’s applicable incident, security,
and change-management policy. Do not shorten retention while an incident,
cleanup dispute, audit, or rollback review is open. At expiration, destroy the
evidence through the approved retention process and retain only the minimum
non-secret disposition metadata required by policy.

## Exit criteria

Phase 55 is complete when the activation and handoff documents are present,
the read-only boundary is explicit, abort and retention rules are reviewable,
and a later operator-approved live-validation step is clearly separated from
normal execution. This phase does not claim that AWS, GCP, GitHub, Docker,
Kubernetes, Terraform, or Packer live validation has occurred.
