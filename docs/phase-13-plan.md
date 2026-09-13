# Phase 13: Shared Controller State and Multi-Replica Operations

## Purpose

Phase 13 moves the controller's durable state from the local file repository to
an AWS-backed shared repository so more than one controller replica can safely
consume webhooks, reconcile runners, and acquire work. The implementation
target is a DynamoDB single table using on-demand capacity, customer-managed
KMS encryption, point-in-time recovery, and a regional write authority.

This phase includes the Go DynamoDB adapter, fencing lease foundation, runtime
backend selection, and infrastructure/IAM scaffolding. The local file
repository remains the rollback and migration source until the DynamoDB
cutover procedure has passed the gates below.

## Deliverables

1. Define the DynamoDB single-table schema and access-pattern contract in
   [`docs/shared-state.md`](shared-state.md).
2. Create the future Terraform module contract in
   [`terraform/modules/controller-state/README.md`](../terraform/modules/controller-state/README.md)
   and its example variables file.
3. Implemented a DynamoDB repository adapter behind the existing
   `state.Repository` interface with mocked contract tests.
4. Add repository contract tests that run against memory, file, and DynamoDB
   implementations.
5. Implemented fencing-aware distributed lease acquisition and renewal before enabling
   more than one active controller replica.

## Proposed rollout

### 13.0 Contract and threat model

- Freeze item types, key prefixes, revision semantics, idempotency keys, and
  ownership fields.
- Treat JIT configuration, installation tokens, PATs, cloud credentials, and
  workload secrets as prohibited attributes. They must never enter the table,
  event data, logs, backups, or Terraform state.
- Define one home Region for writes and reconciliation. Reads may be local to
  that Region; cross-Region failover is an explicit operational procedure.
- Document the controller IAM role separately from runner instance roles.

### 13.1 Terraform and IAM scaffolding

- Provision one encrypted DynamoDB table with `PAY_PER_REQUEST` billing.
- Enable TTL on the `expires_at` attribute, point-in-time recovery, deletion
  protection, and tags identifying the environment and data owner.
- Use a customer-managed KMS key where organizational policy requires key
  ownership and rotation control.
- Grant the controller only the table operations required by the adapter, with
  table and index ARNs scoped to the named resources.
- Keep table creation, KMS administration, backup restoration, and IAM role
  administration outside the runtime controller role.

### 13.2 Adapter and fencing

- Map conditional `CreateItem`, `UpdateItem`, and `DeleteItem` operations to
  the existing revision checks.
- Use conditional writes for event idempotency and lease ownership.
- Acquire a monotonically increasing fencing token with the lease. Every
  mutating operation made by a worker must carry and validate that token.
- Renew leases before their expiry and stop work when renewal loses a
  conditional-write race.
- Make termination and cleanup idempotent so a former owner cannot undo a newer
  owner's state.

### 13.3 File-state migration

- Quiesce webhook intake and workers, or run a dual-read shadow period with a
  single writer.
- Export a point-in-time file snapshot without exposing it in logs or source
  control; protect it with owner-only permissions and encrypted storage.
- Validate IDs, timestamps, revisions, expiry values, and one-to-one job/lease/
  runner relationships before import.
- Import with conditional writes and deterministic keys. Re-running the import
  must be safe and must not overwrite newer records.
- Compare counts and hashes by record class, then run reconciliation in dry-run
  mode before enabling writes.
- Keep the immutable source snapshot until the rollback window expires.

### 13.4 Multi-replica rollout

- Start with one replica using DynamoDB in shadow/read-only verification.
- Run two replicas with one active scheduler and a fencing-protected lease
  holder; the second replica handles health and reconciliation tests only.
- Enable concurrent webhook delivery and verify delivery idempotency, lease
  exclusivity, revision conflicts, and cleanup recovery.
- Scale reconciliation workers only after metrics show bounded lease age,
  conditional-write conflict rates, and no duplicate provider attempts.
- Roll back by stopping new writers, draining leases, and switching to the
  preserved file snapshot or last validated table snapshot.

## Acceptance gates

- Repository contract tests pass for all implementations, including restart,
  conditional conflict, event replay, lease expiry, and fencing cases.
- A lost response followed by retry creates at most one logical record and one
  provider attempt.
- Two replicas cannot both hold the same active lease or successfully mutate a
  record with a stale fencing token.
- TTL is treated as asynchronous storage cleanup only; reconciliation and reaper
  logic enforce expiry immediately.
- Backups and restored copies contain no prohibited secret material.
- IAM Access Analyzer and policy simulation show no access outside the intended
  table, indexes, KMS data-key use, and required observability destinations.
- A controlled failover and rollback exercise is recorded before production
  multi-replica enablement.

## Explicit non-goals

- No automatic cross-account IAM or table provisioning.
- No multi-writer global-table failover in this phase.
- No storing runner bootstrap secrets or GitHub credentials in DynamoDB.
- No removal of file-state support until migration and rollback evidence exists.

References: [DynamoDB read consistency](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.ReadConsistency.html),
[DynamoDB global table design](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/bp-global-table-design.html),
and [IAM policy elements](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements.html).
