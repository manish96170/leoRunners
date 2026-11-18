# Scheduler

The scheduler receives a normalized job request and selects an eligible capacity pool. It evaluates labels and resource requirements, ownership policy, security profile, architecture, OS, regional constraints, provider availability, and capacity. Cost and startup latency are telemetry inputs first; optimization comes later.

The request is cloud-neutral:

```text
OS, architecture, CPU, memory, disk, image profile, labels,
Docker requirement, security profile, repository policy, expiry
```

The scheduler must not construct EC2 instance types or GCP machine names. Provider adapters translate the request to cloud-specific settings.

Concurrency is one independent lease per eligible job. Capacity reservation must be atomic in state, and duplicate webhook delivery must not create a second lease.

Initial policy modes: customer-first, managed-first, customer-only, managed-only, and fallback. Phase 1 implements policy evaluation against fake capacity; cloud scoring is deferred.
