# Phase 7 Benchmark Evidence

This directory defines the versioned evidence contract for comparing a baseline runner with a candidate runner. The files are intentionally self-contained and can be checked offline.

## Files

- `schema.json` is the normative `phase-7.v1` JSON Schema.
- `baseline.json` and `candidate.json` are reproducible example records.
- `validate.sh` checks JSON syntax, required fields, timing/cost invariants, and that the pair is comparable.

Run the validator with the included fixtures:

```sh
./benchmarks/validate.sh
```

Or validate a pair of records:

```sh
./benchmarks/validate.sh path/to/baseline.json path/to/candidate.json
```

## Evidence rules

1. Every record declares `schema_version`, `role`, a comparison ID, and a stable pair key.
2. Baseline and candidate must use the same workload ID/version/commit, region, architecture, and comparison identity. The image ID and digest may differ because image performance is what is being compared.
3. `provenance.source` says where the record came from; `evidence_level` distinguishes a fixture from a measured or attested run. A fixture must never be presented as a cloud measurement.
4. Workload identity is the immutable 40-character source commit, not a branch or mutable tag. Image identity includes both the provider image ID and a content digest.
5. Timing is integer milliseconds. The `total` phase and `total_ms` must equal the sum of the six lifecycle phases preceding it: queue, provision, boot, registration, execution, and cleanup.
6. Costs are non-negative USD amounts. `total_usd` is the exact declared sum of compute, storage, and network cost; retain full precision in records and round only when presenting a report. Pricing source and timestamp must be retained for measured records.
7. Reproducibility requires the same workload commit, image digest, runner profile, region, architecture, tool versions, and benchmark protocol. Record those additional details in `provenance.notes` or extend the schema in a new version.
8. Do not include tokens, credentials, runner registration secrets, customer source, or raw logs containing secrets in evidence records.
9. A failed, cancelled, or timed-out run remains valid evidence when its failure class and measured partial phases are recorded; it must not be silently discarded.
10. Schema changes require a new `schema_version`, fixtures, validator behavior, and a documented migration note.

The sample records are fixtures only. Replace their identities, provenance, timings, and costs with values from a controlled run before drawing conclusions.
