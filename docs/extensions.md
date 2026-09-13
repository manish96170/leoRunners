# Extensions

Extensions consume versioned platform events and may produce insights, recommendations, annotations, or requested actions. Lifecycle and security actions pass through deterministic policy and the controller; extensions cannot directly terminate runners, grant permissions, or expose secrets.

Initial boundary: event sink and read-only data access. Do not implement caching, analytics, cost optimization, security analysis, or AI in Phase 0. This keeps the core controller useful with every extension disabled.

Future extension packages may include caching, analytics, cost, notifications, security, performance, and intelligence. They should not import provider internals.
