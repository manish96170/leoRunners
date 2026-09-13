# Phase 16 Plan

Phase 16 adds backend-neutral observability for lifecycle operations. Structured events carry operational correlation IDs and timing fields without credentials or raw payloads. Metrics use allowlisted low-cardinality dimensions so repository, job, runner, request, and secret values cannot create unbounded series.

The controller exposes Prometheus-compatible `/metrics` output for local and cluster scraping. A CloudWatch Embedded Metric Format exporter is available as an injectable sink and does not make AWS calls by itself. CloudWatch routing, retention, alarms, and dashboards are deployment concerns.

Initial metrics include lifecycle event counters, lifecycle duration histograms, active runner gauges, provider, region, state, outcome, capacity-owner, and error-class dimensions where appropriate. Keep timestamps and correlation IDs in logs/events, not metric labels.
