# Multi-cloud comparison

Phase 51 compares bounded AWS and GCP evidence without making cloud calls. It
calculates deterministic min, median, and nearest-rank p95 values for startup,
lifecycle, cache hit rate, network, and cost evidence.

```sh
./tools/multicloud-comparison/validate.sh
./tools/multicloud-comparison/test.sh
./tools/multicloud-comparison/validate.sh measurements/phase-51-comparison.v1.json /tmp/phase-51-report.json
```

The input is versioned and contains explicit samples, provider identity,
provenance, and direction-aware regression thresholds. Missing AWS or GCP
samples are reported as `unavailable`; they are never interpreted as zero.
Confidence is labelled from sample count and execution provenance.

Synthetic fixtures are not cloud measurements. Real controlled evidence is
still only evidence for the supplied workload, scope, and collection protocol;
the report deliberately disables general real-world provider claims.
