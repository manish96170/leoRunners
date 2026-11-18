# Benchmark comparison

`benchmark-compare` compares repeated JSON evidence from the existing Phase 5
tools. It is a standalone Go module using only the standard library and makes
no cloud, GitHub, network, or credential calls. It accepts one or more files
per side by repeating `--baseline` and `--candidate`.

```sh
go run . --baseline fixtures/baseline-benchmark.json \
  --candidate fixtures/candidate-benchmark.json --format markdown
```

The process exits `0` when all comparable metrics pass, `2` when a regression
gate fails, and `1` for invalid input or invocation. Use `--threshold-percent`
for the allowed relative change (default `10`) and `--threshold-ms` for the
allowed absolute change (default `0`). A metric is a regression only when its
median or p95 exceeds both configured thresholds. Set `--threshold-percent 0`
and `--threshold-ms 25` to require a 25 ms absolute increase, for example.

## Input formats and evidence

The tool directly consumes JSON from `tools/benchmark` and
`tools/workload-validation`:

- Benchmark input is the existing array of samples. Its `durations` values are
  Go JSON durations in nanoseconds. Each phase is reported by its existing
  name, plus a derived `total`.
- Workload-validation input is the existing report object. `platform_timings`
  become `platform.<stage>`, `workload_timings` become `workload.<stage>`, and
  the report's `duration` becomes `total`. Its durations are also nanoseconds.

Evidence is always recorded as `synthetic` or `real`. `mode: fake` reports and
benchmark IDs beginning with `fake-` are inferred as synthetic; command-mode
workload reports and other benchmark IDs are inferred as real/observed. Use
`--evidence synthetic` or `--evidence real` when a collector's provenance is
known but not represented in its legacy schema. Mixed labels within one side
are rejected unless overridden.

An optional normalized wrapper is also accepted:

```json
{"evidence":"real","reports":[{"id":"run-1", "duration":123000000}]}
```

The JSON output contains every metric's count, min, median, p95, and max in
milliseconds, side evidence labels, source paths, deltas, regression reasons,
and the final pass/fail result. Markdown is intended for review artifacts.

Repeated files are combined; the comparison is per metric and does not invent
values for metrics missing on one side.
