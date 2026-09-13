# Preflight Scope Operations

This runbook describes the read-only identity and scope review required before
controlled live validation. It complements the Phase 26 live-validation
runbook. No step in this document provisions resources or changes external
state.

## 1. Freeze the inputs

Create a unique `run_id` and owner-controlled evidence directory. Collect the
approved scope file, ticket or approval reference, revision, configuration
digest, workload manifest digest, provider selection, and tool versions.
Verify the scope file is present, unexpired, immutable for the run, and linked
to the intended revision. Record only its digest and bounded metadata in the
report.

The scope must explicitly identify the provider account or project, region or
zones, repository and tenant, runner group, disposable prefix, required tags
or labels, resource and runtime limits, budget, cleanup deadline, and owners.
Never place credentials or secret values in the scope file or command line.

## 2. Verify source and local scope

In read-only mode, verify that the checked-out revision, repository, branch or
event, tenant, runner group, workload manifest, image reference, and selected
provider match the approved scope. Check that configured resource counts,
instance shape, runtime, timeout, and budget are within the approved maxima.

Record normalized pass or fail results, digests, and bounded identifiers. Do
not attach webhook payloads, environment dumps, user-data, credential files,
or unrestricted command output. A missing value is unknown, not an implicit
approval.

## 3. Verify provider identity

Run only the provider's identity and metadata checks needed for the selected
scope. Confirm the effective identity, account or project, partition or
organization boundary, region, and role or service-account session against
the approved scope.

For AWS, review the caller identity, configured region, account and partition,
and the selected role session. Confirm that read-only discovery can inspect the
approved VPC, subnet, image, security boundary, quotas, and disposable tags.
For GCP, review the active project, organization or folder boundary, region,
and service-account identity, then confirm read-only access to the approved
network, image, quota, and label scope. For GitHub, verify the repository or
organization, revision or event, runner group, and intended JIT registration
boundary without registering a runner.

Provider-specific checks may differ, but they must produce the same bounded
claims: who is acting, where it can act, what it can discover, and whether the
approved disposable boundary is enforceable. Never treat a successful API call
as proof that its identity is approved.

## 4. Verify cleanup scope

Before any approval request, prove that temporary resources can be found and
owned by the run. Confirm the exact tag or label selector, repository or tenant
selector, provider discovery permissions, reconciliation path, cleanup owner,
and deadline. The selector must be narrow enough to avoid shared resources and
complete enough to find dependencies.

A discovery error, timeout, throttling response, ambiguous selector, or missing
owner is `unknown` and blocks the run. Do not infer zero resources from an
empty result returned by an unverified query.

## 5. Produce a secret-safe report

Build the `controlled-validation-evidence.v1` report only from normalized
results. Include `run_id`, scope and configuration digests, provider,
timestamps, checkpoint summaries, bounded reasons, approval reference,
observability readiness, cleanup readiness, and an artifact checksum list.
Redact account-sensitive identifiers when the approved retention policy does
not require them, and never include tokens, keys, authorization headers,
cookies, secrets, raw SDK responses, webhook bodies, user-data, or environment
variable dumps.

Run redaction and schema validation after report generation and before atomic
publication. Scan both JSON values and attachment metadata for secret-shaped
content and unbounded identifiers. A report that cannot be proven safe is not
published as passing; preserve only a sanitized failure summary and notify the
owner.

## 6. Abort criteria

Abort before approval or mutation when any of the following occurs:

- identity, account, project, organization, region, or role does not match;
- repository, revision, tenant, runner group, or provider selection differs;
- scope digest, expiry, signature, configuration, or manifest is invalid;
- requested resources exceed count, shape, runtime, budget, or quota limits;
- image, network, security boundary, API, or endpoint is unavailable or
  outside scope;
- cleanup selectors, permissions, reconciliation, owner, or deadline are not
  proven;
- report redaction, schema validation, artifact bounds, or evidence retention
  fails; or
- provider output is ambiguous, stale, throttled, or unavailable.

Mark the run `aborted` when it stops before execution and `failed` when a
preflight assertion fails. Record a short sanitized reason, stop further
checks that could create confusion, and notify the approver or rollback owner.
Do not broaden access or edit the approved scope in place to make the check
pass.

## 7. Live-run handoff

Hand off only the redacted, schema-valid report and the immutable scope and
configuration digests. The handoff packet contains the approved ticket,
operator and approver references, effective provider identity, selected
checkpoints, limits, cleanup deadline, rollback owner, notification route,
and evidence location. It does not contain credentials or raw provider data.

The receiving operator compares the packet with the live configuration again
immediately before the Phase 26 mutation gate. Any change in identity, scope,
revision, limits, provider, or cleanup plan invalidates the handoff and
requires a new preflight and approval. The handoff authorizes review, not
mutation; the explicit live-validation confirmation remains the final gate.

After handoff, retain the preflight report as the first evidence checkpoint.
During execution, keep all resource names, log dimensions, and queries within
the approved bounded selectors so that cleanup and observability remain
provable.
