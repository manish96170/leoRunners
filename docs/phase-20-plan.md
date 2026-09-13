# Phase 20 Plan

Phase 20 establishes the structured CI event/data layer used by optional
extensions. Events have a version, deterministic content ID, lifecycle/job/
runner/provider correlation metadata, bounded attributes, and redaction before
publication. The durable state repository remains authoritative for lifecycle.

The in-process bus deduplicates event IDs and uses bounded subscriber buffers.
Slow consumers may be dropped and are measured; they cannot block provisioning,
cancellation, termination, or cleanup. The JSONL archive is optional and uses
owner-only permissions, flushes writes, and supports filtered replay.

Extensions consume events for analytics, cache, cost, security, and intelligence.
They may annotate or recommend, but lifecycle and security actions still pass
through deterministic policy.
