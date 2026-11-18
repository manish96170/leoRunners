# Multi-Cloud

The scheduler selects a capacity pool, not a cloud. A pool records provider, ownership, region, security profile, availability, pricing metadata, and labels. This supports customer AWS, customer GCP, managed AWS, managed GCP, and future external providers without changing GitHub-facing semantics.

AWS is the first implementation. GCP follows only after AWS lifecycle reliability and measurements are established. GCP will translate the same `RunnerSpec` into Compute Engine instance-template settings. Provider-specific failures are normalized into lifecycle errors for retry and reconciliation.
