package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

func writeJSON(w io.Writer, result Comparison) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func writeMarkdown(w io.Writer, result Comparison) error {
	status := "PASS"
	if !result.Passed {
		status = "REGRESSION"
	}
	if _, err := fmt.Fprintf(w, "# Benchmark Comparison\n\nStatus: **%s**\n\n| Side | Evidence | Samples | Files |\n|---|---|---:|---|\n| Baseline | `%s` | %d | %s |\n| Candidate | `%s` | %d | %s |\n\nThresholds: %.2f%% and %.3f ms.\n\n| Metric | Baseline median | Candidate median | Baseline p95 | Candidate p95 | Regression |\n|---|---:|---:|---:|---:|---|\n", status, result.Baseline.Evidence, result.Baseline.Samples, strings.Join(result.Baseline.Files, ", "), result.Candidate.Evidence, result.Candidate.Samples, strings.Join(result.Candidate.Files, ", "), result.ThresholdPercent, result.ThresholdMS); err != nil {
		return err
	}
	metrics := make([]string, 0, len(result.Metrics))
	for metric := range result.Metrics {
		metrics = append(metrics, metric)
	}
	sort.Strings(metrics)
	for _, metric := range metrics {
		item := result.Metrics[metric]
		base, candidate, p95Base, p95Candidate := "-", "-", "-", "-"
		if item.Baseline != nil {
			base, p95Base = fmt.Sprintf("%.3f ms", item.Baseline.MedianMS), fmt.Sprintf("%.3f ms", item.Baseline.P95MS)
		}
		if item.Candidate != nil {
			candidate, p95Candidate = fmt.Sprintf("%.3f ms", item.Candidate.MedianMS), fmt.Sprintf("%.3f ms", item.Candidate.P95MS)
		}
		regression := "no"
		if item.Regression {
			regression = "YES"
		}
		if _, err := fmt.Fprintf(w, "| `%s` | %s | %s | %s | %s | %s |\n", metric, base, candidate, p95Base, p95Candidate, regression); err != nil {
			return err
		}
	}
	if len(result.Regressions) > 0 {
		_, _ = fmt.Fprintf(w, "\nRegressions: `%s`\n", strings.Join(result.Regressions, "`, `"))
	}
	return nil
}
