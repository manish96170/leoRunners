# Workload Intake and Controlled Handoff

This runbook describes how an approved repository becomes a reproducible,
redacted workload input for controlled validation. It complements
`workloads/README.md`, the versioned workload manifest, and the controlled
validation evidence procedure. Intake is read-only with respect to cloud
resources and production runner behavior.

## Approval and repository scope

Before inspection, record an owner, reviewer, rollback owner, escalation
contact, retention deadline, and request ID. The request must identify:

- organization and repository;
- approved event, workflow, and branch or tag scope;
- exact commit or an allowed revision-resolution procedure;
- permitted source paths and artifact paths;
- maximum inspection and validation duration;
- maximum runner resources and expected cost boundary; and
- the next controlled-validation environment and provider scope.

The repository must be an approved source, not an arbitrary fork or a local
uncommitted tree. Reject wildcard organization or repository grants, missing
ownership, a moving-only revision, a workflow scope that cannot be bounded,
or a request that embeds credentials. Repository and workflow admission is
separate from permission to execute the workload.

## Fixed commit capture

Resolve the approved branch, tag, or pull request to a full commit ID before
reading files. Store the commit ID and a redacted source identity in the
intake record. Repeat the resolution check immediately before handoff; a
changed ref is a new intake and requires review.

The evidence record should include:

- request ID and repository identity;
- full commit ID and source ref used for resolution;
- inspection start and finish timestamps;
- manifest schema/version and tool versions;
- approved path and workflow scope; and
- reviewer and rollback-owner identities.

Do not persist tokens, checkout credentials, private URLs, webhook bodies, or
unrestricted command output. Do not claim reproducibility from a branch name,
short hash, dirty checkout, or a tool version that was not captured.

## Workflow and tool inventory

Inspect the fixed commit for workflow definitions, scripts, lockfiles,
runtime/version declarations, container files, service dependencies, test and
build commands, artifact paths, and cache configuration. Record observations
as generic capabilities in the workload manifest. Each capability needs:

1. a repository-relative source path;
2. a precise locator such as a key, command, or line context;
3. an `observed`, `inferred`, or `absent` status; and
4. a bounded explanation of why it affects the workload.

Inventory tools by stable name and version source. Separate required tools
from optional conveniences, and record whether the version is pinned,
range-constrained, or unresolved. Record services and network needs as
capabilities and endpoints, not as copied credentials or unrestricted
configuration. A failed inspection is evidence to review; it is not a reason
to add every available tool or permission.

## Manifest and digest

Create a new immutable manifest version rather than editing a manifest already
used by a benchmark or validation run. At minimum, link the manifest to:

- repository and full commit ID;
- inspection timestamp and toolchain inventory;
- workload commands and expected artifact outputs;
- CPU, memory, disk, timeout, and concurrency limits;
- cache mode, namespace, and invalidation assumptions;
- network and service requirements;
- evidence references for enabled or required capabilities; and
- reviewer, retention, and rollback metadata.

Run the workload manifest validator before review. Redact first, then compute
the SHA-256 digest over the exact UTF-8 bytes of the final manifest. Record
the digest in the intake and handoff records and verify it again after any
transfer. A changed byte, schema version, commit, command, toolchain, or
resource bound invalidates the handoff and requires a new review.

The manifest must not contain access keys, bearer tokens, private keys,
webhook secrets, environment values, customer data, complete lockfiles,
unbounded provider responses, or raw workflow payloads. Evidence references
should point to bounded, redacted observations rather than embedding source
contents.

## Comparability review

The reviewer confirms that a later run can be compared with its baseline.
Review at least:

- identical repository commit and manifest digest;
- equivalent image or toolchain versions and architecture;
- identical command, environment mode, and success criteria;
- consistent cache mode, namespace, warm/cold state, and invalidation;
- consistent CPU, memory, disk, timeout, concurrency, and network profile;
- explicit synthetic versus real provenance for measurements;
- stable units and sample-count requirements for latency and cost; and
- comparable pricing assumptions, region or zone, and provider scope.

If a provider, tool, cache state, command, or resource boundary differs,
label the result as non-comparable or start a new baseline. Missing data is
`WARN` or `BLOCKED` according to the approved gate; it is never silently
treated as a passing measurement. Keep raw provider output out of the
evidence package and retain only bounded, redacted summaries and checksums.

## Review, rollback, and handoff

The reviewer signs off only after scope, fixed commit, inventory, manifest
digest, redaction, comparability, security, and resource bounds pass. The
rollback owner records the prior manifest/profile version and the exact action
needed to restore it. Rollback means disabling the new intake, restoring the
previous version, revoking temporary access, and preserving a redacted
decision record; it does not mean mutating a prior manifest in place.

Hand off only the immutable manifest, digest, approved scope, validation
request, bounded evidence references, and review/rollback records. The
controlled-validation operator must re-check the commit, digest, scope,
deadline, and resource limits before any later execution. A successful intake
does not authorize cloud provisioning, GitHub registration, workflow
execution, image publication, or production scheduling.

Mark the handoff `READY` only when all required checks pass. Use `BLOCKED`
for scope drift, digest mismatch, secret exposure, missing evidence,
unbounded commands or resources, unresolved toolchain requirements, failed
redaction, or incomplete rollback ownership. Record `UNAVAILABLE` when an
environment dependency cannot be tested, without presenting it as success.

## Retention and audit

Retain the request, review decision, fixed-commit record, final manifest,
digest, bounded evidence references, comparability assessment, and rollback
record in the owner-controlled evidence location. Apply owner-only access and
the approved retention deadline. Delete temporary checkouts, command output,
credentials, and raw inspection material earlier. If a secret is discovered,
stop the handoff, revoke or rotate it, mark the intake failed, and retain only
the sanitized incident reference.
