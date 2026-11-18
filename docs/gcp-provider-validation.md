# GCP Provider Validation

This runbook validates the GCP provider independently from controller, AWS, GitHub, and multi-cloud orchestration. It is intended for a disposable provider check before production activation.

## Operating boundary

The default path is read-only and offline. Mock fixtures and contract validators establish expected behavior without Google Cloud credentials. No command should create, delete, upload, or apply resources unless the operator has explicitly selected the guarded live path and approved the exact scope.

The independent check is not a production deployment. Use a dedicated validation ownership label, a short deadline, and a resource prefix that cannot be confused with customer resources.

## Required scope handoff

Before starting, obtain the reviewed activation request and scope file. Confirm that they specify:

- GCP project ID, region, and zone.
- VPC network, subnet, firewall boundary, network tags, and egress mode.
- Exact image or instance-template identifier and architecture.
- Exact runtime service-account email and permitted API or access-scope boundary.
- Resource prefix, unique ownership label, maximum resources, and deadline.
- Repository or organization, runner group, and labels.
- Named operator, notification owner, escalation path, and scope hash.

Never substitute a personal default project, default zone, broad wildcard scope, unreviewed resource identifier, or an unrelated service account. Credentials are supplied through the approved identity mechanism and are never copied into reports, logs, startup scripts, fixtures, or shell history.

## Mock validation

Run the repository's offline validators and fixture tests first. The mock path should prove project and zone scope, exact labels and ownership marker behavior, service-account and network contracts, bounded retries and timeouts, cancellation semantics, stale-instance handling, cleanup discovery, secret redaction, and evidence-schema compatibility.

Record the validator versions and result in the handoff. A failed, warning-without-review, or unavailable prerequisite blocks live validation. Mock success proves contract behavior only; it does not prove that the selected project, zone, quota, image, network, service account, or IAM permissions are usable.

## Live validation

Live validation requires all of the following before the first mutating call:

1. Mock validation is `PASS`.
2. The activation request is approved and its deadline has not expired.
3. GCP identity, project, region, and zone match the reviewed scope.
4. The caller and runtime service account have only the reviewed provider permissions.
5. The image or template, network, labels, service account, and ownership marker match exactly.
6. The operator has confirmed the disposable resource limit and cleanup owner.

Perform read-only checks first. Then create at most the approved number of disposable runners using the approved idempotency and ownership values. Observe only the required checkpoints: creation, bootstrap, registration, cancellation, deletion, and cleanup discovery. Keep retries bounded and stop at the deadline.

Do not run customer jobs, use production runner groups, discover unrelated project resources, or widen permissions during this check. A provider failure is evidence to investigate, not a reason to bypass the activation gate.

## Abort and cleanup

Abort before creation for any identity, project, zone, hash, permission, input, service-account, network, label, deadline, notification, or redaction failure. Abort immediately for an unexpected resource, missing ownership label, secret exposure, unbounded retry, registration outside the approved boundary, or evidence write failure.

Cleanup is required after every attempted instance creation, including a timeout or partial bootstrap. Delete only resources with the unique validation ownership label. Repeat a scoped discovery query after deletion and require zero matching active resources. If discovery is unavailable or cleanup cannot be proven before the deadline, classify the run as `BLOCKED`, stop provisioning, retain redacted evidence, and escalate to the named owner. Do not create a replacement instance.

Alarms, dashboards, extensions, and AI advisories are notification and evidence mechanisms only. They must not trigger unreviewed creation, deletion, credential changes, or policy uploads.

## Evidence handoff

Create one versioned report containing the run ID, scope hash, project, region, zone, provider version, tool versions, timestamps, checkpoint statuses, durations, retry counts, and cleanup discovery result. Include the explicit classifications `MOCK`, `LIVE`, `CLEANUP`, and `EXIT`; distinguish `PASS`, `WARN`, `BLOCKED`, and `UNAVAILABLE`.

The report must contain bounded, redacted evidence references rather than raw Google Cloud responses, credentials, tokens, startup scripts, or unrestricted logs. Write it atomically with owner-only permissions and retain it according to the approved deadline and evidence policy. Attach the report fingerprint and operator approval reference to the activation handoff.

Handoff is accepted only when the report validates successfully, cleanup is proven, and the operator records the decision. A failed or incomplete report is not evidence of readiness.

## Completion checklist

- [ ] Reviewed activation request and scope hash are present.
- [ ] Offline/mock validators pass.
- [ ] GCP identity, project, region, and zone match the approved scope.
- [ ] Live resources use exact image/template, network, service account, labels, and ownership marker.
- [ ] Provider checkpoints complete within the deadline.
- [ ] Cancellation and deletion are idempotent.
- [ ] Fresh scoped cleanup discovery proves no validation resources remain.
- [ ] Redacted evidence report validates and is handed off.
- [ ] Any environment limitation is recorded as `BLOCKED` or `UNAVAILABLE`.
