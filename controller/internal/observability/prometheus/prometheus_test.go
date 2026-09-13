package prometheus

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGatherIsDeterministicAndEscapesLabels(t *testing.T) {
	e := New(Config{Namespace: "leo-ci", AllowedLabels: map[string]struct{}{"provider": {}, "phase": {}, "event": {}}})
	if err := e.RecordLifecycle("job\nqueued", "aws/us-east-1"); err != nil {
		t.Fatal(err)
	}
	if err := e.ObserveLifecycle("provision", "aws", 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := e.SetActiveRunners("aws", 2); err != nil {
		t.Fatal(err)
	}
	got := e.Gather()
	for _, want := range []string{
		"# TYPE leo_ci_active_runners gauge",
		"leo_ci_active_runners{provider=\"aws\"} 2",
		"# TYPE leo_ci_lifecycle_duration_seconds histogram",
		`leo_ci_lifecycle_duration_seconds_bucket{phase="provision",provider="aws",le="2.5"} 1`,
		`leo_ci_lifecycle_duration_seconds_sum{phase="provision",provider="aws"} 1.5`,
		`event="job\nqueued"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Gather() missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "active_runners") > strings.Index(got, "lifecycle_duration") {
		t.Error("metric families are not sorted")
	}
	if got != e.Gather() {
		t.Error("Gather() is not deterministic")
	}
}

func TestLabelAllowlistAndCardinality(t *testing.T) {
	e := New(Config{AllowedLabels: map[string]struct{}{"provider": {}}, MaxLabelValues: 1, MaxSeries: 1})
	if err := e.IncCounter("jobs-total", Labels{"provider": "aws", "job_id": "secret"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.Gather(), "job_id") {
		t.Fatal("unapproved label was exported")
	}
	if err := e.IncCounter("jobs-total", Labels{"provider": "gcp"}); err == nil {
		t.Fatal("expected label cardinality error")
	}
	if err := e.IncCounter("other", Labels{"provider": "aws"}); err == nil {
		t.Fatal("expected series cardinality error")
	}
}

func TestRegistrationConflictsAndInvalidValues(t *testing.T) {
	e := New(Config{AllowedLabels: map[string]struct{}{"provider": {}}})
	if err := e.Register("jobs", Counter, "jobs", "provider"); err != nil {
		t.Fatal(err)
	}
	if err := e.Register("jobs", Gauge, "jobs", "provider"); err == nil {
		t.Fatal("expected registration conflict")
	}
	if err := e.AddCounter("jobs", -1, nil); err == nil {
		t.Fatal("expected negative counter rejection")
	}
	if err := e.SetGauge("temperature", math.NaN(), nil); err == nil {
		t.Fatal("expected NaN rejection")
	}
	if err := e.Observe("duration", -1, nil); err == nil {
		t.Fatal("expected negative observation rejection")
	}
}

func TestHistogramBucketsAreCumulative(t *testing.T) {
	e := New(Config{DefaultBuckets: []float64{1, 2}, MaxSeries: 10})
	if err := e.Observe("duration", 1.5, nil); err != nil {
		t.Fatal(err)
	}
	output := e.Gather()
	if !strings.Contains(output, "duration_bucket{le=\"1\"} 0") || !strings.Contains(output, "duration_bucket{le=\"2\"} 1") || !strings.Contains(output, "duration_bucket{le=\"+Inf\"} 1") {
		t.Fatalf("non-cumulative histogram:\n%s", output)
	}
}

func TestServeHTTPUsesPrometheusContentType(t *testing.T) {
	e := New(Config{})
	if err := e.IncCounter("events", nil); err != nil {
		t.Fatal(err)
	}
	recording := httptest.NewRecorder()
	e.ServeHTTP(recording, httptest.NewRequest("GET", "/metrics", nil))
	if recording.Code != 200 {
		t.Fatalf("status = %d", recording.Code)
	}
	if got := recording.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if !strings.Contains(recording.Body.String(), "events 1\n") {
		t.Fatalf("body = %q", recording.Body.String())
	}
}

func TestNormalizedLabelCollisionIsRejected(t *testing.T) {
	e := New(Config{AllowedLabels: map[string]struct{}{"provider": {}}})
	if err := e.IncCounter("events", Labels{"pro-vider": "aws", "pro vider": "gcp"}); err == nil {
		t.Fatal("expected normalized label collision")
	}
	if got := e.Gather(); got != "" {
		t.Fatalf("failed sample registered a family: %q", got)
	}
}
