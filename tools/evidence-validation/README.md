# Controlled validation evidence

`validate.sh` checks redacted, versioned JSON artifacts produced by approved live validation. It is offline and performs no cloud calls.

Exit codes are `0` for valid evidence, `1` for unsafe or invalid content, and `2` when Ruby, the schema, or a requested file is unavailable.

The contract records run identity, provider and region, ordered lifecycle checkpoints, timestamps and bounded durations, mandatory cleanup proof, and references to metrics, logs, alarms, and dashboards. References identify externally retained evidence; raw logs, metric payloads, credentials, tokens, and environment values are intentionally not accepted.

Run the focused tests with `tools/evidence-validation/test.sh`.
