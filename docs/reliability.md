# Reliability

Phase 4 adds two local reliability mechanisms.

## Durable state

Set `STATE_PATH` to use the file-backed repository. Each mutation is serialized, written to a temporary file with restrictive permissions, flushed, and atomically renamed. A backup snapshot is retained for recovery from a missing or corrupt primary file. Omit `STATE_PATH` only for ephemeral local tests.

## Reconciliation

The controller runs reconciliation periodically. `RECONCILE_INTERVAL` accepts a Go duration such as `30s` or `1m`. A pass compares durable jobs, leases, runners, and provider status, repairs safe state transitions, and reaps expired runners with bounded provider calls.

GitHub API requests retry transient failures with bounded exponential backoff and `Retry-After` support. Context cancellation always stops retries and provider operations.

This is still a single-process local repository. Production deployment needs a shared durable store and a coordinated worker/lease model before running multiple controller replicas.
