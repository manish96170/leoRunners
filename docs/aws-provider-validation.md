# AWS Provider Validation

This runbook validates the AWS provider independently from controller, GitHub, and multi-cloud orchestration. It is intended for a disposable provider check before production activation.

## Operating boundary

The default path is read-only and offline. Mock fixtures and contract validators establish expected behavior without AWS credentials. No command should provision, terminate, upload, or apply resources unless the operator has explicitly selected the guarded live path and approved the exact scope.

The independent check is not a production deployment. Use a dedicated validation ownership marker, a short deadline, and a resource prefix that cannot be confused with customer resources.

## Required scope handoff

Before starting, obtain the reviewed activation request and scope file. Confirm that they specify:

- AWS account ID and region.
- VPC, subnet, security group, and availability-zone boundaries.
- Exact launch-template or AMI identifier and architecture.
- Resource prefix, unique ownership marker, maximum resources, and deadline.
- Repository or organization, runner group, and labels.
- Named operator, notification owner, escalation path, and scope hash.

Never substitute a personal default profile, default region, broad wildcard scope, or unreviewed resource identifier. Credentials are supplied through the approved identity mechanism and are never copied into reports, logs, user data, fixtures, or shell history.

## Mock validation

Run the repository's offline validators and fixture tests first. The mock path should prove provider contract compatibility, exact tags and client-token behavior, bounded retries and timeouts, cancellation semantics, stale-resource handling, cleanup discovery, IAM policy conditions, secret redaction, and evidence-schema compatibility.

Record the validator versions and result in the handoff. A failed, warning-without-review, or unavailable prerequisite blocks live validation. Mock success proves contract behavior only; it does not prove that the selected AWS account, region, quota, image, network, or IAM role is usable.

## Live validation

Live validation requires all of the following before the first mutating call:

1. Mock validation is `PASS`.
2. The activation request is approved and its deadline has not expired.
3. AWS identity and region match the reviewed scope.
4. The caller has only the reviewed provider permissions.
5. The launch input, network, labels, tags, and ownership marker match exactly.
6. The operator has confirmed the disposable resource limit and cleanup owner.

Perform read-only checks first. Then launch at most the approved number of disposable runners using the approved idempotency token. Observe only the required checkpoints: launch, bootstrap, registration, cancellation, termination, and cleanup discovery. Keep retries bounded and stop at the deadline.

Do not run customer jobs, use production runner groups, discover unrelated resources, or widen permissions during this check. A provider failure is evidence to investigate, not a reason to bypass the activation gate.

## Abort and cleanup

Abort before launch for any identity, scope, hash, permission, input, label, deadline, notification, or redaction failure. Abort immediately for an unexpected resource, missing ownership tag, secret exposure, unbounded retry, registration outside the approved boundary, or evidence write failure.

Cleanup is required after every attempted launch, including a timeout or partial bootstrap. Terminate only resources with the unique validation ownership marker. Repeat discovery after termination and require zero matching active resources. If discovery is unavailable or cleanup cannot be proven before the deadline, classify the run as `BLOCKED`, stop provisioning, retain redacted evidence, and escalate to the named owner. Do not launch a replacement run.

Alarms, dashboards, extensions, and AI advisories are notification and evidence mechanisms only. They must not trigger unreviewed provisioning, termination, credential changes, or policy uploads.

## Evidence handoff

Create one versioned report containing the run ID, scope hash, account, region, provider version, tool versions, timestamps, checkpoint statuses, durations, retry counts, and cleanup discovery result. Include the explicit classifications `MOCK`, `LIVE`, `CLEANUP`, and `EXIT`; distinguish `PASS`, `WARN`, `BLOCKED`, and `UNAVAILABLE`.

The report must contain bounded, redacted evidence references rather than raw AWS responses, credentials, tokens, user data, or unrestricted logs. Write it atomically with owner-only permissions and retain it according to the approved deadline and evidence policy. Attach the report fingerprint and operator approval reference to the activation handoff.

Handoff is accepted only when the report validates successfully, cleanup is proven, and the operator records the decision. A failed or incomplete report is not evidence of readiness.

## Completion checklist

- [ ] Reviewed activation request and scope hash are present.
- [ ] Offline/mock validators pass.
- [ ] AWS identity and region match the approved scope.
- [ ] Live resources use exact inputs, labels, tags, and ownership marker.
- [ ] Provider checkpoints complete within the deadline.
- [ ] Cancellation and termination are idempotent.
- [ ] Fresh cleanup discovery proves no validation resources remain.
- [ ] Redacted evidence report validates and is handed off.
- [ ] Any environment limitation is recorded as `BLOCKED` or `UNAVAILABLE`.
