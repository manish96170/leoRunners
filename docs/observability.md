# Observability

Emit structured lifecycle events:

`JobQueued`, `RunnerProvisioning`, `RunnerReady`, `JobStarted`, `JobCompleted`, `JobFailed`, `JobCancelled`, `RunnerTerminated`, `ProvisioningFailed`, and `RunnerTimeout`.

Each event carries correlation IDs and repository, workflow, run, job, attempt, runner, provider, region, image, instance type, timestamps, duration, resource profile, cache data, failure reason, and estimated cost when available. Logs are operational diagnostics; persisted lifecycle state remains authoritative.

Initial sinks should be interfaces with a local structured logger and metrics implementation. CloudWatch is the likely AWS operational sink, while durable job/runner state may use DynamoDB. Raw logs, structured events, aggregate metrics, and future AI context are separate data classes.
