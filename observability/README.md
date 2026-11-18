# Observability Alerts

\`alert-config.v1.yaml\` is the versioned, provider-neutral alert contract for
Leo Runners. It describes low-cardinality metric dimensions and human-facing
notification actions. It does not contain CloudWatch resources or credentials.

The allowlisted dimensions are \`provider\`, \`region\`, \`state\`, \`outcome\`,
\`capacity_owner\`, \`error_class\`, and the bounded registry key \`extension\`.
Tenant IDs, job IDs, runner IDs, request IDs, commit SHAs, extension invocation
IDs, and other unbounded values must remain in logs/events rather than metric
labels.

Extension health signals use the following metrics: \`extension_failures_total\`,
\`extension_timeouts_total\`, \`extension_panics_total\`,
\`extension_events_dropped_total\`, and \`extension_reports_dropped_total\`.
They are notification-only and may use only the allowlisted dimensions.

Run \`tools/observability-validation/validate.sh\` to validate all fixtures.
Warnings are printed for disabled alerts and return success. Contract,
cardinality, action, or secret violations fail with exit code 1. Missing tools,
files, or unsupported input fail with exit code 2. Validator output contains
only paths and rule names; fixture values are never echoed.
