# Phase 1 Plan

## Goal

Build and test the core Go controller without real EC2 provisioning.

## Packages

```text
controller/cmd/runner-controller
controller/internal/domain
controller/internal/github
controller/internal/lifecycle
controller/internal/scheduler
controller/internal/providers
controller/internal/state
controller/internal/telemetry
controller/internal/policy
```

## Work sequence

1. Define immutable `Job`, `RunnerSpec`, `Runner`, `Lease`, capacity-pool, and lifecycle-event types.
2. Define valid state transitions and reject invalid transitions.
3. Implement a state repository with conditional/idempotent operations and a local adapter for tests.
4. Normalize GitHub webhook payloads and verify HMAC signatures.
5. Implement scheduler policy and capacity reservation.
6. Implement a fake provider with controllable readiness, failure, and termination behavior.
7. Orchestrate queue-to-cleanup and cancellation with contexts and deadlines.
8. Add reconciliation and reaper passes over persisted records.
9. Add structured telemetry at every transition.

## Tests

Cover duplicate delivery, concurrent jobs, cancellation during each lifecycle phase, rerun isolation, provider failure, timeout, repeated cleanup, controller restart replay, stale state, and invalid transitions. Add one end-to-end test using the fake provider.

## Exit criteria

- All Phase 1 lifecycle tests pass without cloud credentials.
- Twenty concurrent fake jobs produce twenty independent leases.
- Duplicate events produce one lease.
- Cancellation and expiry terminate exactly the associated runner.
- Reconciliation repairs interrupted work deterministically.
- AI can be absent and the controller still works.

## Risks

GitHub event semantics and JIT runner assignment need validation against a real test repository in Phase 3. Persistence choice, webhook hosting, and fork security policy remain open.
