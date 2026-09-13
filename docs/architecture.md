# Architecture

## Scope

The platform is a Go control plane for disposable GitHub Actions runners. The first production-shaped slice is GitHub `workflow_job` intake through scheduling, provider provisioning, runner readiness, completion, and cleanup. Phase 0 does not provision infrastructure.

## Boundaries

```text
GitHub webhook/API
        |
        v
Controller -> State repository -> Lifecycle state machine
        |                |
        v                v
    Scheduler       Event/telemetry sink
        |
        v
RunnerProvider (fake first, AWS next, GCP later)
```

The controller owns GitHub semantics and lifecycle policy. A provider owns cloud-specific machine creation, readiness, status, and termination. Ownership mode, capacity pool, region, and security profile are data on a provider registration, not branches in the scheduler.

## Decisions

- Go: concurrency, cancellation, compact deployment, and operational tooling.
- HTTP webhook plus GitHub REST client initially; no Kubernetes requirement.
- Explicit state transitions with persisted compare-and-set semantics.
- Event-driven orchestration plus periodic reconciliation.
- Structured events are emitted from lifecycle transitions and never used as the sole source of truth.
- AI and analytics consume events outside the critical path.

## Deployment direction

Start as one controller process with a worker loop and reaper. Evaluate ECS/Fargate, EC2, or Lambda after observing webhook volume and reconciliation duration. Self-hosted and managed deployments use the same binaries with different provider and state configuration.
