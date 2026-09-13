# Phase 21 Plan

Phase 21 provides the extension boundary for analytics, cache, cost, security,
and intelligence consumers. Extensions register immutable metadata, receive
versioned events through bounded queues, and run with per-extension timeouts.
Slow, failing, or panicking extensions are isolated and reported without
blocking runner provisioning or cleanup.

Extension output is advisory. Deterministic policy accepts only bounded
annotations or recommendations; lifecycle, security, credential, and provider
actions remain owned by the core controller. Disabled extensions receive no
events, and the event/data layer remains the source for replay and retention.
