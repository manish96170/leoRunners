package main

import "time"

type Evidence string

const (
	EvidenceSynthetic Evidence = "synthetic"
	EvidenceReal      Evidence = "real"
)

type Observation struct {
	Metric   string
	Duration time.Duration
	Evidence Evidence
	Source   string
	SampleID string
}

type Stats struct {
	Count    int     `json:"count"`
	MinMS    float64 `json:"min_ms"`
	MedianMS float64 `json:"median_ms"`
	P95MS    float64 `json:"p95_ms"`
	MaxMS    float64 `json:"max_ms"`
}

type SideResult struct {
	Evidence Evidence         `json:"evidence"`
	Files    []string         `json:"files"`
	Metrics  map[string]Stats `json:"metrics"`
	Samples  int              `json:"samples"`
}

type MetricComparison struct {
	Baseline                *Stats   `json:"baseline,omitempty"`
	Candidate               *Stats   `json:"candidate,omitempty"`
	MedianDeltaMS           *float64 `json:"median_delta_ms,omitempty"`
	P95DeltaMS              *float64 `json:"p95_delta_ms,omitempty"`
	MedianRegressionPercent *float64 `json:"median_regression_percent,omitempty"`
	P95RegressionPercent    *float64 `json:"p95_regression_percent,omitempty"`
	Regression              bool     `json:"regression"`
	Reason                  string   `json:"reason,omitempty"`
}

type Comparison struct {
	SchemaVersion    string                      `json:"schema_version"`
	ThresholdPercent float64                     `json:"threshold_percent"`
	ThresholdMS      float64                     `json:"threshold_ms"`
	Baseline         SideResult                  `json:"baseline"`
	Candidate        SideResult                  `json:"candidate"`
	Metrics          map[string]MetricComparison `json:"metrics"`
	Regressions      []string                    `json:"regressions"`
	Passed           bool                        `json:"passed"`
}
