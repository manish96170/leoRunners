# Phase 58: Independent AWS Provider Validation

## Objective

Validate the AWS provider independently from the full production activation flow. The phase must prove that the provider contract, permissions, launch inputs, lifecycle behavior, and cleanup evidence are correct before AWS-backed runner execution is enabled.

This phase is a validation phase. It does not broaden the approved workload scope, enable unattended provisioning, or replace the production activation approval gate.

## Validation tracks

### Mock and offline track

Run first and on every change. This track must not contact AWS or require credentials. It covers:

- Provider configuration and version contracts.
- AWS account, region, resource-prefix, and ownership fields in the reviewed scope.
- Launch-template or image identifiers, architecture, labels, tags, and idempotency-token rules.
- Required lifecycle checkpoints: request, launch, registration, cancellation, termination, and cleanup discovery.
- Retry, timeout, cancellation, and stale-resource behavior.
- Redaction, secret non-persistence, bounded evidence, and atomic owner-only report handling.
- IAM policy simulation fixtures and the rule that policy generation remains review-only.

The mock track is a prerequisite for any live check. A failed or indeterminate mock result blocks the phase.

### Live AWS track

Run only after the mock track passes and an operator approves the reviewed activation request. The live track uses a disposable, least-privilege validation scope and a fixed short deadline. It must validate only the approved account, region, resource prefix, image/template, subnet, security group, and runner labels.

The live track should prove, in order:

1. Read-only identity and scope checks match the approved request.
2. The provider can resolve the approved launch inputs without mutation.
3. A single disposable runner can be launched with the expected tags and client token.
4. The runner reaches the intended registration checkpoint, or the run records a bounded, diagnosable failure.
5. Cancellation and termination are idempotent.
6. Cleanup discovery finds no resources bearing the validation ownership marker.
7. The report is complete, redacted, and linked to the activation handoff.

The live track must never use a customer workload, production runner group, broad discovery query, or unreviewed account or region.

## Required AWS scope

The approved scope must identify, without embedding credentials:

- AWS account ID and region.
- Resource prefix and unique validation ownership marker.
- Approved VPC, subnet, security group, and availability-zone constraints.
- Exact launch-template/image identifiers and architecture.
- Runner group, repository or organization boundary, and required labels.
- Maximum instance count, instance type, disk limit, and validation deadline.
- Notification owner and escalation contact.

The scope hash must be recorded in the activation request and evidence report. Any mismatch between the reviewed scope and live identity or resource attributes is an immediate block.

Credentials must come from the approved local or CI identity mechanism. Do not place access keys, session tokens, private keys, user-data secrets, or raw provider responses in the repository or evidence artifacts.

## Abort and cleanup rules

Abort before provisioning when identity, region, scope hash, permissions, image/template, labels, deadline, or notification checks fail. Abort on an unexpected resource, unbounded retry, missing ownership tag, registration outside the approved boundary, or any secret exposure.

After any attempted launch, cleanup is mandatory even when registration fails. Terminate only resources carrying the unique validation ownership marker, retry cleanup within the bounded deadline, and verify termination through a fresh discovery query. If cleanup cannot be proven, mark the run `BLOCKED`, preserve the redacted failure evidence, and escalate; do not retry provisioning.

No automatic lifecycle action may be triggered by an alarm, extension, AI recommendation, or incomplete report.

## Evidence handoff

Publish one versioned, redacted report with:

- Run ID, reviewed scope hash, provider, account, region, and timestamps.
- Mock and live track status, explicit exit classification, and tool versions.
- Lifecycle checkpoints, bounded durations, retry counts, and failure reasons.
- Resource ownership marker and cleanup discovery result, without raw secrets or unrestricted payloads.
- Report fingerprint, validator result, operator approval reference, and retention deadline.

Write the report atomically with owner-only permissions. A failed, incomplete, unredacted, or cleanup-unproven report cannot be used as an activation handoff. The handoff is valid only when the mock track passes, the live track passes or is explicitly reviewed as an environment limitation, and the operator records the resulting decision.

## Exit criteria

- Offline and mock validation pass without cloud mutation.
- Live validation, when approved, is constrained to the reviewed AWS scope.
- Every attempted resource has a verified cleanup result.
- Evidence is schema-compatible, redacted, bounded, and durably handed off.
- Any environment gap is clearly classified as `BLOCKED` or `UNAVAILABLE`, never silently treated as success.
