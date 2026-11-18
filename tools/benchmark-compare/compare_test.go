package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatsMedianP95(t *testing.T) {
	values := []Observation{{Metric: "x", Duration: 1 * time.Millisecond}, {Metric: "x", Duration: 2 * time.Millisecond}, {Metric: "x", Duration: 3 * time.Millisecond}, {Metric: "x", Duration: 4 * time.Millisecond}, {Metric: "x", Duration: 100 * time.Millisecond}}
	stats := calculateStats(values)["x"]
	if stats.MedianMS != 3 || stats.P95MS != 100 || stats.MinMS != 1 || stats.MaxMS != 100 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestReadBothFormatsAndInference(t *testing.T) {
	dir := t.TempDir()
	benchmark := filepath.Join(dir, "benchmark.json")
	if err := os.WriteFile(benchmark, []byte(`[{"id":"fake-0001","durations":{"queue":1000000,"launch":2000000}}]`), 0600); err != nil {
		t.Fatal(err)
	}
	obs, evidence, err := readObservations(benchmark, "")
	if err != nil {
		t.Fatal(err)
	}
	if evidence != EvidenceSynthetic || len(obs) != 3 {
		t.Fatalf("benchmark: %s %d", evidence, len(obs))
	}
	workload := filepath.Join(dir, "workload.json")
	if err := os.WriteFile(workload, []byte(`{"schema_version":"workload-validation.v1","id":"run-1","mode":"command","duration":5000000,"platform_timings":{"queue":1000000},"workload_timings":{"tests":3000000}}`), 0600); err != nil {
		t.Fatal(err)
	}
	obs, evidence, err = readObservations(workload, "")
	if err != nil {
		t.Fatal(err)
	}
	if evidence != EvidenceReal || len(obs) != 3 {
		t.Fatalf("workload: %s %d", evidence, len(obs))
	}
}

func TestRegressionGate(t *testing.T) {
	base := []Observation{{Metric: "total", Duration: 100 * time.Millisecond}, {Metric: "total", Duration: 110 * time.Millisecond}}
	candidate := []Observation{{Metric: "total", Duration: 130 * time.Millisecond}, {Metric: "total", Duration: 140 * time.Millisecond}}
	result, err := compare(base, candidate, EvidenceReal, EvidenceReal, 10, 0, []string{"b"}, []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Regressions) != 1 {
		t.Fatalf("expected regression: %+v", result)
	}
}

func TestRegressionDoesNotFailForImprovement(t *testing.T) {
	base := []Observation{{Metric: "total", Duration: 100 * time.Millisecond}}
	candidate := []Observation{{Metric: "total", Duration: 90 * time.Millisecond}}
	result, err := compare(base, candidate, EvidenceSynthetic, EvidenceSynthetic, 0, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("unexpected regression: %+v", result)
	}
}

func TestJSONOutput(t *testing.T) {
	result, err := compare([]Observation{{Metric: "total", Duration: time.Second}}, []Observation{{Metric: "total", Duration: time.Second}}, EvidenceReal, EvidenceReal, 10, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := writeJSON(&output, result); err != nil {
		t.Fatal(err)
	}
	var decoded Comparison
	if err := json.Unmarshal([]byte(output.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Passed {
		t.Fatal("expected pass")
	}
}

func TestZeroBaselineRemainsJSONSerializable(t *testing.T) {
	result, err := compare([]Observation{{Metric: "total", Duration: 0}}, []Observation{{Metric: "total", Duration: time.Millisecond}}, EvidenceReal, EvidenceReal, 10, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := writeJSON(&output, result); err != nil {
		t.Fatal(err)
	}
	if result.Passed || !strings.Contains(output.String(), "regression") {
		t.Fatalf("expected zero-baseline regression: %s", output.String())
	}
}
