# Intelligence Evaluation Validation

This offline validator checks telemetry-backed intelligence evaluation
fixtures. It does not contact an AI provider, cloud provider, or lifecycle
control plane. Evaluation fixtures must keep `ai_enabled: false`, declare
`synthetic` or `real` provenance, use bounded telemetry/log records, and keep
advisories inside the deterministic read-only policy boundary.

Run:

```sh
./tools/intelligence-validation/test.sh
./tools/intelligence-validation/validate.sh
```

The Go harness under `controller/internal/intelligence/eval` derives historical
stats, applies the existing `ai.Gate`, computes bounded-log and policy metrics,
and exposes a deterministic fingerprint for regression comparisons.
