package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLifecycleEventValidation(t *testing.T) {
	event := LifecycleEvent{EventType: "runner.ready", Timestamp: time.Unix(10, 0).UTC()}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []LifecycleEvent{{}, {EventType: "x", Timestamp: time.Now(), Duration: -time.Second}} {
		if err := invalid.Validate(); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected invalid event, got %v", err)
		}
	}
}

func TestMetricDimensionsRejectHighCardinality(t *testing.T) {
	if err := (Dimensions{"job_id": "job-123"}).Validate(); !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("expected rejected dimension, got %v", err)
	}
	if err := (Dimensions{"provider": "aws", "region": "us-east-1"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (MetricSample{Name: "runner.ready", Kind: MetricCounter, Unit: UnitCount, Value: 1, Timestamp: time.Now(), Dimensions: Dimensions{"provider": "aws"}}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorBoundsAndDefensiveSnapshot(t *testing.T) {
	c := NewCollector(2)
	for i := 0; i < 3; i++ {
		if err := c.AddCounter("jobs.total", 1, Dimensions{"provider": "fake"}); err != nil {
			t.Fatal(err)
		}
	}
	if c.Len() != 2 || c.Dropped() != 1 {
		t.Fatalf("unexpected collector state: len=%d dropped=%d", c.Len(), c.Dropped())
	}
	snapshot := c.Snapshot()
	snapshot[0].Dimensions["provider"] = "changed"
	if c.Snapshot()[0].Dimensions["provider"] != "fake" {
		t.Fatal("snapshot shared collector dimensions")
	}
}

func TestCollectorConcurrentRecord(t *testing.T) {
	c := NewCollector(1000)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = c.AddCounter("jobs.total", 1, nil)
			}
		}()
	}
	wg.Wait()
	if c.Len() != 400 {
		t.Fatalf("got %d samples", c.Len())
	}
}

func TestJSONLoggerRedactsSecretsRecursively(t *testing.T) {
	var output bytes.Buffer
	secret := "opaque-jit-value"
	logger := NewJSONLogger(&output, LoggerOptions{RedactedValues: []string{secret}})
	err := logger.Info("provisioning "+secret, Correlation{JobID: "job-1"}, map[string]any{
		"token": secret, "nested": map[string]any{"message": "contains " + secret}, "authorization": "Bearer abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), "Bearer abc") {
		t.Fatalf("secret leaked: %s", output.String())
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["level"] != string(LevelInfo) {
		t.Fatalf("unexpected level: %#v", record["level"])
	}
}

func TestLifecycleLoggingAndRedactJSON(t *testing.T) {
	var output bytes.Buffer
	logger := NewJSONLogger(&output, LoggerOptions{})
	err := logger.Lifecycle(LifecycleEvent{EventType: "runner.terminated", Timestamp: time.Now().UTC(), Correlation: Correlation{RunnerID: "r-1"}, Duration: time.Second, Attributes: map[string]string{"status": "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	sanitized, err := RedactJSON([]byte(`{"token":"secret","state":"ready"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sanitized), "secret") || !strings.Contains(string(sanitized), redacted) {
		t.Fatalf("unexpected sanitized JSON: %s", sanitized)
	}
}
