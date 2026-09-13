# Phase 27 Plan

Phase 27 defines the read-only preflight boundary for controlled validation.
Before a live run can be approved, the operator must prove that the current
identity, provider boundary, repository scope, resource limits, and cleanup
ownership match an approved scope file. This phase produces evidence; it does
not provision resources or broaden permissions.

## Objectives

- Verify cloud and source-control identity without mutating external state.
- Make the approved account, project, organization, repository, region,
  tenant, resource, and budget boundaries explicit.
- Detect scope drift before a mutation gate is considered.
- Keep provider output and reports secret-safe and bounded.
- Define deterministic abort behavior and a clear handoff into live validation.

## Read-only boundary

Every Phase 27 check must use a read-only API, metadata endpoint, or local
configuration inspection. The preflight must not create, update, delete,
register, deregister, attach, assume, or rotate anything. Read-only does not
mean harmless: identity and resource-discovery calls can still reveal sensitive
information, so output is summarized and redacted before it is persisted.

The preflight records the command version, revision, configuration digest,
timestamp, operator reference, and a unique `run_id`. It records normalized
facts such as account or project identity, region, enabled APIs, quota
availability, and matching resource counts rather than raw SDK responses.

## Approved scope files

An approved scope file is the signed or ticket-linked input for a run. It is
immutable for the duration of preflight and live execution and is referenced by
content digest in the report. At minimum it declares:

- provider and allowed account, project, organization, or subscription;
- allowed regions, zones, VPC or network, subnet, and security boundary;
- repository, organization, branch or revision, tenant, and runner group;
- disposable resource prefix plus required tags or labels;
- maximum resource count, instance shape, runtime, budget, and cleanup
  deadline;
- allowed image or machine-image family and required capacity assumptions;
- permitted integrations, checkpoints, and observability window; and
- operator, approver, rollback owner, notification route, and expiry.

The scope file must not contain credentials, access tokens, private keys,
webhook bodies, user-data, or secret values. A missing, expired, unsigned,
unreadable, or digest-mismatched scope file is an immediate preflight failure.
The effective scope is the intersection of the approved scope, the current
configuration, and provider-discovered capabilities. It is never the union.

## Phase 27 checkpoints

Use stable checkpoint names in the Phase 25 evidence envelope:

1. `preflight_source_scope`: revision, repository, branch or event, tenant,
   and runner group match the approved file.
2. `preflight_aws_identity`: caller account, partition, region, and role
   session match the AWS scope.
3. `preflight_gcp_identity`: project, organization or folder, region, and
   service-account identity match the GCP scope when GCP is selected.
4. `preflight_github_scope`: repository, organization, runner group, and JIT
   permissions are within the approved GitHub boundary.
5. `preflight_resource_scope`: images, networks, subnets, security groups,
   quotas, capacity, and required APIs satisfy the request without expansion.
6. `preflight_cleanup_scope`: tags or labels, discovery queries, owner
   permissions, deadline, and reconciliation path can identify all temporary
   resources.
7. `preflight_report_safety`: the report is redacted, bounded, schema-valid,
   and tied to the scope and configuration digests.

Each checkpoint records status, timestamps, duration, a sanitized reason, and
the scope assertion separately from the provider result. A provider call that
succeeds while its returned identity or resource boundary mismatches the
approved scope is a failure.

## Mismatch handling

Treat any mismatch as fail-closed. Stop preflight, do not request approval for
a mutating step, and produce an `aborted` or `failed` redacted report. The
operator must notify the approver when identity, revision, region, project,
repository, tenant, resource prefix, limits, cleanup owner, or expiry differs.

Do not repair drift by changing the scope file during a run, selecting a
different account, retrying with broader credentials, ignoring an unknown
field, or substituting a nearby region or project. To proceed, create a new
approved scope file, recompute its digest, rerun all read-only checks, and
obtain approval again. Preserve the failed report as evidence of the blocked
attempt without retaining raw provider output.

## Completion criteria

Phase 27 is complete for a run only when every selected identity and scope
checkpoint passes, the report is redacted and schema-valid, the scope and
configuration digests are recorded, and an authorized approver confirms the
same effective scope. Passing preflight is readiness evidence, not permission
to mutate. A separate Phase 26 mutation gate remains mandatory.

The phase is complete as a platform capability when the checks, fixtures,
runbook, review procedure, and live-run handoff are documented and operators
can demonstrate that a mismatch prevents mutation.
