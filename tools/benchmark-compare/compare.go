package main

import (
	"fmt"
	"sort"
)

func compare(baseline, candidate []Observation, baselineEvidence, candidateEvidence Evidence, thresholdPercent, thresholdMS float64, baselineFiles, candidateFiles []string) (Comparison, error) {
	if thresholdPercent < 0 || thresholdMS < 0 {
		return Comparison{}, fmt.Errorf("thresholds cannot be negative")
	}
	baseStats, candidateStats := calculateStats(baseline), calculateStats(candidate)
	comparison := Comparison{SchemaVersion: "benchmark-compare.v1", ThresholdPercent: thresholdPercent, ThresholdMS: thresholdMS, Baseline: SideResult{Evidence: baselineEvidence, Files: append([]string(nil), baselineFiles...), Metrics: baseStats, Samples: countSamples(baseline)}, Candidate: SideResult{Evidence: candidateEvidence, Files: append([]string(nil), candidateFiles...), Metrics: candidateStats, Samples: countSamples(candidate)}, Metrics: make(map[string]MetricComparison), Passed: true}
	metricNames := make(map[string]bool)
	for metric := range baseStats {
		metricNames[metric] = true
	}
	for metric := range candidateStats {
		metricNames[metric] = true
	}
	metrics := make([]string, 0, len(metricNames))
	for metric := range metricNames {
		metrics = append(metrics, metric)
	}
	sort.Strings(metrics)
	for _, metric := range metrics {
		base, hasBase := baseStats[metric]
		candidate, hasCandidate := candidateStats[metric]
		item := MetricComparison{}
		if hasBase {
			copy := base
			item.Baseline = &copy
		}
		if hasCandidate {
			copy := candidate
			item.Candidate = &copy
		}
		if hasBase && hasCandidate {
			medianDelta, p95Delta := candidate.MedianMS-base.MedianMS, candidate.P95MS-base.P95MS
			medianPct, p95Pct := regressionPercent(base.MedianMS, candidate.MedianMS), regressionPercent(base.P95MS, candidate.P95MS)
			item.MedianDeltaMS, item.P95DeltaMS, item.MedianRegressionPercent, item.P95RegressionPercent = &medianDelta, &p95Delta, &medianPct, &p95Pct
			medianRegression := medianDelta > thresholdMS && medianPct > thresholdPercent
			p95Regression := p95Delta > thresholdMS && p95Pct > thresholdPercent
			item.Regression = medianRegression || p95Regression
			if medianRegression {
				item.Reason = "median exceeds configured thresholds"
			} else if p95Regression {
				item.Reason = "p95 exceeds configured thresholds"
			}
			if item.Regression {
				comparison.Passed = false
				comparison.Regressions = append(comparison.Regressions, metric)
			}
		}
		comparison.Metrics[metric] = item
	}
	return comparison, nil
}

func regressionPercent(baseline, candidate float64) float64 {
	if baseline == 0 {
		if candidate > 0 {
			// A finite sentinel keeps the result valid JSON while ensuring any
			// practical configured percentage threshold is exceeded.
			return 1e308
		}
		return 0
	}
	return (candidate - baseline) / baseline * 100
}

func countSamples(observations []Observation) int {
	ids := make(map[string]bool)
	for _, observation := range observations {
		ids[observation.Source+"\x00"+observation.SampleID] = true
	}
	return len(ids)
}
