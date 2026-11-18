package stats

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestAnalyzeComputesDeterministicMetrics(t *testing.T) {
	samples := []Sample{
		{ID: "b", Provider: "aws", Outcome: OutcomeFailure, CacheObserved: true, CacheHit: false, Timings: map[string]time.Duration{"queue": 4 * time.Second, "boot": 10 * time.Second}, Resources: Resources{CPU: 4, MemoryGB: 8}},
		{ID: "a", Provider: "gcp", Outcome: OutcomeSuccess, CacheObserved: true, CacheHit: true, Timings: map[string]time.Duration{"queue": 2 * time.Second, "boot": 6 * time.Second}, Resources: Resources{CPU: 2, MemoryGB: 4, GPU: 1}},
		{ID: "c", Provider: "aws", Outcome: OutcomeCancelled, Timings: map[string]time.Duration{"queue": 6 * time.Second}, Resources: Resources{CPU: 6, MemoryGB: 12}},
	}
	report, err := Analyze(samples)
	if err != nil {
		t.Fatal(err)
	}
	if report.Count != 3 || report.Successes != 1 || report.Failures != 2 || report.FailureRate != 2.0/3.0 {
		t.Fatalf("outcome metrics: %+v", report)
	}
	if report.CacheObserved != 2 || report.CacheHits != 1 || report.CacheHitRate != 0.5 {
		t.Fatalf("cache metrics: %+v", report)
	}
	queue := report.Timings["queue"]
	if queue.Count != 3 || queue.MeanMS != 4000 || queue.MedianMS != 4000 || queue.P95MS != 6000 {
		t.Fatalf("queue metrics: %+v", queue)
	}
	if got := report.Providers["aws"]; got.Count != 2 || got.Failures != 2 || got.Timings.Count != 3 {
		t.Fatalf("provider metrics: %+v", got)
	}
	if got := report.Providers["aws"].TimingsByPhase["queue"]; got.Count != 2 || got.MedianMS != 5000 {
		t.Fatalf("provider phase metrics: %+v", got)
	}
	if report.Resources.MeanCPU != 4 || report.Resources.MaxMemoryGB != 12 || math.Abs(report.Resources.MeanGPU-1.0/3.0) > 1e-12 {
		t.Fatalf("resource metrics: %+v", report.Resources)
	}
	if report.ResourcesByProvider["gcp"].MaxGPU != 1 {
		t.Fatalf("provider resources: %+v", report.ResourcesByProvider)
	}

	reversed := []Sample{samples[2], samples[0], samples[1]}
	reversedReport, err := Analyze(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report, reversedReport) {
		t.Fatalf("report depends on input order:\n%+v\n%+v", report, reversedReport)
	}
}

func TestAnalyzeValidation(t *testing.T) {
	valid := Sample{ID: "id", Provider: "aws", Outcome: OutcomeSuccess, Timings: map[string]time.Duration{"queue": time.Second}}
	cases := []struct {
		name   string
		sample Sample
		target error
	}{
		{"missing id", Sample{Provider: "aws", Outcome: OutcomeSuccess}, ErrInvalidSample},
		{"unknown outcome", Sample{ID: "id", Provider: "aws", Outcome: "unknown"}, ErrInvalidSample},
		{"cache hit without observation", Sample{ID: "id", Provider: "aws", Outcome: OutcomeSuccess, CacheHit: true}, ErrInvalidSample},
		{"negative timing", Sample{ID: "id", Provider: "aws", Outcome: OutcomeSuccess, Timings: map[string]time.Duration{"queue": -time.Second}}, ErrInvalidSample},
		{"negative memory", Sample{ID: "id", Provider: "aws", Outcome: OutcomeSuccess, Resources: Resources{MemoryGB: -1}}, ErrInvalidSample},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := Analyze([]Sample{test.sample})
			if !errors.Is(err, test.target) {
				t.Fatalf("error = %v, want %v", err, test.target)
			}
		})
	}
	if _, err := Analyze([]Sample{valid, valid}); !errors.Is(err, ErrDuplicateSample) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := Analyze(nil); !errors.Is(err, ErrNoSamples) {
		t.Fatalf("empty error = %v", err)
	}
}

func TestSampleJSONUsesMillisecondsAndReportIsJSONSafe(t *testing.T) {
	sample := Sample{ID: "job-1", Provider: "aws", Outcome: OutcomeSuccess, Timings: map[string]time.Duration{"boot": 1500 * time.Millisecond}}
	data, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"id":"job-1","provider":"aws","outcome":"success","cache_observed":false,"cache_hit":false,"timings_ms":{"boot":1500},"resources":{"cpu":0,"memory_gb":0,"gpu":0}}` {
		t.Fatalf("unexpected JSON: %s", data)
	}
	report, err := Analyze([]Sample{sample})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(report); err != nil {
		t.Fatal(err)
	}
}

func TestPercentileNearestRankAndEmptyProviderTiming(t *testing.T) {
	samples := make([]Sample, 0, 20)
	for i := 0; i < 20; i++ {
		samples = append(samples, Sample{ID: string(rune('a' + i)), Provider: "aws", Outcome: OutcomeSuccess, Timings: map[string]time.Duration{"phase": time.Duration(i+1) * time.Second}})
	}
	samples = append(samples, Sample{ID: "without-timing", Provider: "gcp", Outcome: OutcomeFailure})
	report, err := Analyze(samples)
	if err != nil {
		t.Fatal(err)
	}
	if report.Timings["phase"].P95MS != 19000 {
		t.Fatalf("p95 = %v", report.Timings["phase"].P95MS)
	}
	if report.Providers["gcp"].Timings != (DurationStats{}) {
		t.Fatalf("empty provider timing = %+v", report.Providers["gcp"].Timings)
	}
}
