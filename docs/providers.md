# Providers

The provider boundary is deliberately narrow:

```go
type RunnerProvider interface {
    Provision(context.Context, RunnerSpec) (RunnerInstance, error)
    WaitReady(context.Context, RunnerInstance) error
    Status(context.Context, RunnerInstance) (RunnerStatus, error)
    Terminate(context.Context, RunnerInstance) error
}
```

The exact Go types are part of Phase 1. Provider methods must be cancellable, bounded by deadlines, and idempotent where practical. The fake provider must simulate capacity, delays, failures, status changes, and repeated termination.

Phase 2 AWS uses Launch Template plus `RunInstances`, tags every instance with platform/job ownership metadata, requires IMDSv2, and applies separate controller and runner IAM roles. GCP is a later adapter using Compute Engine instance templates. macOS and external providers remain future adapters.
