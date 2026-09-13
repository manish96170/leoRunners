# Phase 59: Independent GCP Provider Validation

## Objective

Validate the GCP provider independently from controller, AWS, GitHub, and multi-cloud orchestration. The phase must prove that the provider contract, project and zone scope, service-account boundary, network inputs, lifecycle behavior, and cleanup evidence are correct before GCP-backed runner execution is enabled.

This is a validation phase. It does not broaden the approved workload scope, enable unattended provisioning, or replace the production activation approval gate.

## Validation tracks

### Mock and offline track

Run first and on every change. This track must not contact Google Cloud or require credentials. It covers:

- Provider and API version contracts.
- GCP project, zone, region, resource-prefix, and ownership fields in the reviewed scope.
- Machine template or image identifiers, architecture, labels, service-account identity, and idempotency rules.
- VPC, subnet, firewall, network-tag, and egress expectations.
- Required lifecycle checkpoints: request, instance creation, bootstrap, registration, cancellation, deletion, and cleanup discovery.
- Retry, timeout, cancellation, quota, and stale-resource behavior.
- Redaction, secret non-persistence, bounded evidence, and atomic owner-only report handling.

The mock track is a prerequisite for any live check. A failed or indeterminate mock result blocks the phase.

### Live GCP track

Run only after the mock track passes and an operator approves the reviewed activation request. The live track uses a disposable, least-privilege validation scope and a fixed short deadline. It must validate only the approved project, zone, resource prefix, image or instance template, subnet, firewall boundary, service account, and runner labels.

The live track should prove, in order:

1. Read-only identity and project/zone scope checks match the approved request.
2. The provider can resolve the approved image, template, service account, and network inputs without mutation.
3. A single disposable runner can be created with the expected labels, network tags, and ownership marker.
4. The runner reaches the intended registration checkpoint, or the run records a bounded, diagnosable failure.
5. Cancellation and deletion are idempotent.
6. Cleanup discovery finds no resources bearing the validation ownership marker.
7. The report is complete, redacted, and linked to the activation handoff.

The live track must never use a customer workload, production runner group, broad project-wide discovery query, or unreviewed project, zone, service account, or network.

## Required GCP scope

The approved scope must identify, without embedding credentials:

- GCP project ID, region, zone, and any regional quota boundary.
- Resource prefix and unique validation ownership label value.
- Approved VPC network, subnet, firewall rules, network tags, and egress mode.
- Exact image or instance-template identifiers and architecture.
- Exact service-account email and the permitted runtime scopes or APIs.
- Runner group, repository or organization boundary, and required labels.
- Maximum instance count, machine type, disk limit, and validation deadline.
- Notification owner and escalation contact.

The scope hash must be recorded in the activation request and evidence report. Any mismatch between the reviewed scope and live project, zone, resource labels, service account, network attributes, or runner labels is an immediate block.

Credentials must come from the approved local or CI identity mechanism. Do not place service-account keys, access tokens, private keys, startup-script secrets, or raw provider responses in the repository or evidence artifacts.

## Abort and cleanup rules

Abort before provisioning when identity, project, zone, scope hash, permissions, image/template, service account, network, labels, deadline, or notification checks fail. Abort on an unexpected resource, unbounded retry, missing ownership label, registration outside the approved boundary, or any secret exposure.

After any attempted instance creation, cleanup is mandatory even when registration fails. Delete only resources carrying the unique validation ownership marker, retry cleanup within the bounded deadline, and verify deletion through a fresh scoped discovery query. If cleanup cannot be proven, mark the run `BLOCKED`, preserve the redacted failure evidence, and escalate; do not retry provisioning.

No automatic lifecycle action may be triggered by an alarm, extension, AI recommendation, or incomplete report.

## Evidence handoff

Publish one versioned, redacted report with:

- Run ID, reviewed scope hash, provider, project, region, zone, and timestamps.
- Mock and live track status, explicit exit classification, and tool versions.
- Lifecycle checkpoints, bounded durations, retry counts, and failure reasons.
- Resource ownership label, service-account identity in redacted or approved form, and cleanup discovery result, without raw secrets or unrestricted payloads.
- Report fingerprint, validator result, operator approval reference, and retention deadline.

Write the report atomically with owner-only permissions. A failed, incomplete, unredacted, or cleanup-unproven report cannot be used as an activation handoff. The handoff is valid only when the mock track passes, the live track passes or is explicitly reviewed as an environment limitation, and the operator records the resulting decision.

## Exit criteria

- Offline and mock validation pass without cloud mutation.
- Live validation, when approved, is constrained to the reviewed GCP project, zone, service account, network, and resource scope.
- Every attempted instance has a verified cleanup result.
- Evidence is schema-compatible, redacted, bounded, and durably handed off.
- Any environment gap is clearly classified as `BLOCKED` or `UNAVAILABLE`, never silently treated as success.
