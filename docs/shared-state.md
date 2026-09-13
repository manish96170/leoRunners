# Shared State Contract

This document defines the proposed DynamoDB representation of the existing
`controller/internal/state.Repository`. It is a contract for the future
adapter, not a deployable table or a migration command.

## Table settings

- Partition key: `pk` (string).
- Sort key: `sk` (string).
- Billing mode: `PAY_PER_REQUEST` initially. Revisit only with measured,
  stable traffic and an explicit capacity review.
- TTL attribute: `expiresAt` (number, Unix seconds) on expiring records.
- Optional GSI1: `gsi1pk`, `gsi1sk` for bounded state scans.
- Optional GSI2: `gsi2pk`, `gsi2sk` for expiry/reaper work when the table is too
  large for partition-local queries.
- Encryption: AWS owned encryption is the minimum; use a customer-managed KMS
  key when separation of duties, key policy control, or retention policy
  requires it.

DynamoDB TTL is asynchronous. It is a storage-cost and retention mechanism,
not the controller's timer. The reaper must query due records and perform
provider termination and state transitions itself.

## Item layout

The adapter should keep a stable item envelope and encode the complete domain
record as typed attributes. Attribute names below are illustrative and should
be frozen with contract tests before implementation.

| Record | `pk` | `sk` | Optional index keys |
| --- | --- | --- | --- |
| Job | `JOB#<jobID>` | `META` | `JOBSTATE#<state>`, `UPDATED#<time>#<jobID>` |
| Runner | `RUNNER#<runnerID>` | `META` | `RUNNERSTATE#<state>`, `EXPIRE#<time>#<runnerID>` |
| Lease | `LEASE#<leaseID>` | `META` | `LEASESTATE#<state>`, `EXPIRE#<time>#<leaseID>` |
| Lifecycle event | `EVENT#<idempotencyKey>` | `META` | `EVENTJOB#<jobID>#<time>` |
| Controller lease | `CONTROL#<scope>` | `LEASE` | none |

The primary keys provide exact reads and idempotent event identity. GSI values
are denormalized query projections, not independent sources of truth. Every
projection must include a unique suffix so records with the same timestamp do
not overwrite one another.

## Access patterns

| Operation | Recommended request | Consistency |
| --- | --- | --- |
| Get job, runner, or lease | `GetItem` by exact `pk`/`sk` | Strong in the home Region when a read-after-write decision is required |
| Create record | `PutItem` with `attribute_not_exists(pk)` | Conditional |
| Save record revision | `UpdateItem` with `revision = :expected` and ownership/fence checks | Conditional |
| Insert lifecycle event | `PutItem` with `attribute_not_exists(pk)`; same key means replay | Conditional/idempotent |
| List jobs/runners/leases by state | `Query` the corresponding GSI partition | Eventually consistent unless a table/LSI design provides a local strong read |
| List due records | `Query` expiry projection by time bucket, then filter `expiresAt <= now` | Eventually consistent plus recheck before mutation |
| Acquire controller scope lease | `PutItem` if absent/expired, or conditional update of an expired holder | Conditional |
| Renew controller or job lease | `UpdateItem` requiring holder and fencing token match | Conditional |
| Release lease | Conditional `UpdateItem`/tombstone requiring holder and token | Conditional |

Avoid table scans in request paths. If a repair operation needs a full scan,
bound it with a segment, page limit, retry budget, and operator-visible report.

## Conditional writes and revisions

The current repository uses `expectedRevision == 0` for create and the stored
revision for update. The DynamoDB adapter should preserve that behavior:

```text
create: attribute_not_exists(pk)
update: revision = :expectedRevision AND recordType = :type
        AND (fenceToken = :token OR attribute_not_exists(fenceToken))
```

The update must atomically increment `revision`, update `updatedAt`, and refresh
the GSI projection. A failed condition maps to `ErrRevisionConflict`, not a
blind retry. Retry only after re-reading and deciding whether the operation is
still valid.

For operations that change multiple records, use a DynamoDB transaction only
within one Region and only when the item count/size and contention profile are
known. The adapter must not assume a transaction provides cross-Region atomicity.

## Distributed leases and fencing

Each controller scope has one durable lease item containing:

- `holderID`: unique controller instance identity;
- `fenceToken`: monotonically increasing ownership generation;
- `leaseUntil`: server-compatible expiry timestamp;
- `updatedAt` and `revision`;
- optional `heartbeatAt` for diagnostics.

Acquire only when no holder exists or the prior lease is expired. On success,
increment the fence token in the same conditional update and return it to the
worker. Every job/runner mutation must include the token and reject stale
tokens. A process that loses renewal must stop scheduling and provider cleanup
work; it must not rely on a local clock or continue after a conditional failure.

Fencing protects durable state. It does not cancel an already-issued cloud API
request, so provider calls still need context deadlines, idempotency tokens, and
post-call reconciliation.

## Retention and prohibited data

Use `expiresAt` on jobs, runners, leases, and events only when the retention
policy permits deletion. Keep a separate immutable audit/export path if legal
or operational retention exceeds table TTL. TTL deletion can lag and must not
be treated as proof that a runner was terminated.

Never persist:

- GitHub JIT `encoded_jit_config` or installation/PAT tokens;
- cloud access keys, session credentials, or private keys;
- workload secrets, environment values, or arbitrary bootstrap metadata;
- raw webhook signatures or authorization headers.

Store only secret-free identifiers, hashes where needed for correlation, typed
status values, timestamps, provider IDs, and bounded redacted diagnostic data.

## Migration and recovery

Migration is a controlled, single-writer operation:

1. Snapshot and validate file state.
2. Stop or fence file-state writers.
3. Import records using deterministic keys and conditional creates.
4. Compare record counts, IDs, lifecycle-event idempotency keys, and relationship
   invariants.
5. Run a read-only reconciliation against cloud resources.
6. Switch the repository implementation behind a feature flag.
7. Preserve the file snapshot until rollback sign-off.

Backups and exports must use encrypted storage, owner-only access, and the same
secret exclusion rules as the live table. Restores must be followed by a
revision/fence audit before any controller is allowed to write.

## Multi-replica rules

The first production design is single-Region, single-writer for scheduling and
reconciliation, with health-checked standby replicas. All replicas may accept
webhooks only if delivery idempotency and job-level ownership are enforced by
the table. A regional failover requires explicit writer fencing and a validated
endpoint switch.

Do not enable active-active global-table writes for lifecycle records without a
separate conflict model. Global tables replicate changes, but concurrent writes
to the same item can conflict and local transactions do not become cross-Region
transactions. Region pinning keeps lease ownership and strong read-after-write
decisions understandable.

References: [DynamoDB conditional writes](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Expressions.ConditionExpressions.html),
[DynamoDB TTL](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/TTL.html),
[DynamoDB backups](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/BackupRestore.html),
and [DynamoDB global tables](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/globaltables_HowItWorks.html).
