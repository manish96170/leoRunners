# Recovery evidence fixtures

These deterministic, redacted fixtures model file-backup recovery,
DynamoDB-style revision protection, idempotent event replay, lease fencing,
stale-runner reaping, and a two-replica takeover. They contain no credentials
and do not represent a live cloud run.

The passing fixture must validate. The unsafe fixtures must be rejected for
secret-shaped content or violated recovery invariants.
