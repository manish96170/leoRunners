# Runner Lifecycle

```text
queued -> provisioning -> ready -> assigned -> running -> completed
             |             |         |          |
             +-> failed    +-> failed +-> failed +-> cancelled

completed/cancelled/failed -> terminating -> terminated
```

Every job attempt has one runner lease. A rerun creates a new attempt and a new lease. A runner is never returned to a general pool.

Persist at least: repository, workflow, run, job, attempt, runner ID, provider, provider instance ID, labels, timestamps, expiry, controller owner, and last observed status.

All transitions are idempotent. Termination is safe to repeat. Reconciliation repairs missing or delayed events by comparing persisted state with GitHub and the provider. The reaper terminates anything past its maximum lifetime, including partially provisioned instances.

Cancellation is checked before provisioning, during readiness polling, before assignment, and during cleanup. Provider calls receive deadlines and cancellation contexts.
