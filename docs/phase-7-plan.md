# Phase 7 Plan

Phase 7 makes performance and cost comparisons evidence-based. The comparison tool consumes repeated Phase 5/6 reports and computes per-metric min, median, p95, and max values. It labels synthetic and real evidence separately and exits non-zero when configured regression gates fail.

The cost estimator accepts caller-supplied rates and quantities, produces itemized compute, storage, network, and cache totals, and labels the result as an estimate rather than billing truth. It does not fetch live prices.

## Controlled comparison

1. Use the same repository commit, workflow, region, architecture, protocol, and workload for baseline and candidate.
2. Collect repeated runs and retain raw JSON evidence.
3. Validate both records with `benchmarks/validate.sh`.
4. Run `tools/benchmark-compare` with explicit absolute and percentage thresholds.
5. Run `tools/cost-estimator` with exact price assumptions and record their source/date separately.
6. Investigate any regression before rollout; never turn synthetic output into a provider claim.

The fixtures intentionally include a candidate regression so the gate behavior is testable offline.
