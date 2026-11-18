# Extension records v1

schema.v1.json defines the optional extension contract for analytics, cache,
security, and intelligence consumers. Each example is a standalone record
containing bounded metadata and an advisory result. Records are observations,
not commands.

## Boundary

Extensions may consume versioned lifecycle events and read-only projections.
They may publish metadata, annotations, recommendations, and confidence. They
must not provision, terminate, cancel, retry, schedule, assign, authorize,
change security policy, issue credentials, or expose secrets. The controller
and deterministic security policy remain authoritative. A failed, slow, or
disabled extension must never block job execution, cleanup, or termination.

## Registration and versioning

Register an extension by a lowercase stable extensionId and one of the four
declared extensionType values. Registration is configuration only; it grants
no cloud or lifecycle authority. Consumers must tolerate duplicate delivery,
unknown optional metadata values, out-of-order records, and schema evolution.

apiVersion and metadata.schemaVersion are versioned independently. Additive
fields require compatible consumers. A breaking change creates a new API
version and fixtures; an extension ID is not reused for a different contract.
Provider internals and raw event payloads are not extension interfaces.

## Metadata and advisories

Metadata values are bounded scalar observations. Do not include cache keys,
logs, source contents, environment variables, tokens, credentials, model
prompts, or personal data. Use aggregate counts and redacted classifications.
Advisories are reviewable output only. A blocked status describes a
recommendation and does not block the controller. Confidence is evidence
quality, not authority.

## Failure isolation and retention

Use finite queues, bounded processing time, measurable drops, and a dead-letter
path outside the lifecycle state store. A consumer can be disabled or removed
without changing runner behavior. Persist only the minimum redacted record and
honor retention.expiresAt; do not retain raw extension input as a side channel.
Replay is read-only and must not repeat lifecycle effects.

## Offline validation

Run from the repository root:

    ./extensions/validate.sh

The validator uses Ruby's standard JSON parser and checks version/kind,
domain-specific IDs, bounded scalar maps, retention, redaction, advisory
limits, duplicate records, and forbidden command or secret-shaped content.

## Runtime policy

`runtime-policy.v1.yaml` is the offline-validated deployment policy for the
registered read-only consumers. `global.tenant_allowlist` is the complete set
of tenants eligible for extension delivery; each consumer has an explicit
subset and no wildcard is accepted. Queue sizes, execution timeouts, and
redaction byte limits are finite. Only `read_events` and `emit_advisories` are
permitted capabilities, and redaction is mandatory and metadata-only.

Validate the policy and its negative fixtures with:

    ./tools/extension-validation/validate.sh
