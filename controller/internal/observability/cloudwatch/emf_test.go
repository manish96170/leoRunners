package cloudwatch

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
)

func testExporter(t *testing.T, writer *bytes.Buffer) *Exporter {
	t.Helper()
	e, err := New(Config{
		Namespace:          "LeoRunners/Controller",
		Dimensions:         map[string]string{"ServiceName": "controller", "Environment": "test"},
		DimensionAllowlist: []string{"ServiceName", "Environment", "Provider", "Operation"},
		Writer:             writer,
		Now:                func() time.Time { return time.UnixMilli(1700000000123).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMarshalEMFDocument(t *testing.T) {
	e := testExporter(t, nil)
	b, err := e.Marshal(MetricSet{
		Timestamp:  time.UnixMilli(1700000000456).UTC(),
		Dimensions: map[string]string{"Provider": "aws", "Operation": "provision"},
		Metrics: []Metric{
			{Name: "ProvisioningDuration", Value: 12.5, Unit: "Milliseconds"},
			{Name: "RunnerCount", Value: 1, Unit: "Count", StorageResolution: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(b, &document); err != nil {
		t.Fatal(err)
	}
	aws, ok := document["_aws"].(map[string]any)
	if !ok || aws["Timestamp"] != float64(1700000000456) {
		t.Fatalf("unexpected _aws metadata: %#v", document["_aws"])
	}
	if document["ServiceName"] != "controller" || document["Environment"] != "test" || document["Provider"] != "aws" {
		t.Fatalf("dimensions missing from root: %#v", document)
	}
	if _, ok := document["JobID"]; ok {
		t.Fatal("high-cardinality job identity must not be emitted")
	}
	if !strings.Contains(string(b), `"StorageResolution":1`) {
		t.Fatalf("high-resolution metric definition missing: %s", b)
	}

	var directive []any
	metrics := aws["CloudWatchMetrics"].([]any)
	directive = metrics
	if len(directive) != 1 {
		t.Fatalf("expected one directive, got %d", len(directive))
	}
}

func TestMarshalRejectsLimitsAndUnsafeFields(t *testing.T) {
	e := testExporter(t, nil)
	tooMany := make([]Metric, maxMetrics+1)
	for i := range tooMany {
		tooMany[i] = Metric{Name: "m" + string(rune('a'+i%26)) + string(rune('0'+i/26)), Value: 1}
	}
	tests := []struct {
		name string
		set  MetricSet
	}{
		{"too many metrics", MetricSet{Metrics: tooMany}},
		{"too many dimensions", MetricSet{Dimensions: dimensionMap(31), Metrics: []Metric{{Name: "x", Value: 1}}}},
		{"nan", MetricSet{Metrics: []Metric{{Name: "x", Value: math.NaN()}}}},
		{"sensitive metric", MetricSet{Metrics: []Metric{{Name: "github_token_age", Value: 1}}}},
		{"unallowlisted dimension", MetricSet{Dimensions: map[string]string{"JobID": "job-1"}, Metrics: []Metric{{Name: "x", Value: 1}}}},
		{"sensitive dimension", MetricSet{Dimensions: map[string]string{"RunnerToken": "redacted"}, Metrics: []Metric{{Name: "x", Value: 1}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.Marshal(tc.set); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func dimensionMap(count int) map[string]string {
	values := make(map[string]string, count)
	for i := 0; i < count; i++ {
		values["Environment"+string(rune('a'+i))] = "test"
	}
	return values
}

func TestMarshalUsesConfiguredClockAndWriteIsAtomic(t *testing.T) {
	var buffer bytes.Buffer
	e := testExporter(t, &buffer)
	if err := e.EmitMetricSet(MetricSet{Metrics: []Metric{{Name: "Count", Value: 1, Unit: "Count"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buffer.String(), "\n") || !strings.Contains(buffer.String(), `"Timestamp":1700000000123`) {
		t.Fatalf("unexpected output: %s", buffer.String())
	}

	var concurrent bytes.Buffer
	workers := 16
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.Write(&concurrent, MetricSet{Metrics: []Metric{{Name: "Count", Value: 1}}}); err != nil {
				t.Errorf("write failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if lines := strings.Count(concurrent.String(), "\n"); lines != workers {
		t.Fatalf("expected %d complete events, got %d", workers, lines)
	}
}

func TestEmitTelemetryEventDropsIdentifiersAndMetadata(t *testing.T) {
	var buffer bytes.Buffer
	e := testExporter(t, &buffer)
	err := e.Emit(telemetry.Event{
		ID: "delivery-secret", Type: "ready", JobID: "job-123", RunnerID: "runner-123", Provider: "gcp",
		At: time.UnixMilli(1700000000999), Metadata: map[string]string{"token": "should-never-appear", "safe": "also-dropped"},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := buffer.String()
	for _, forbidden := range []string{"delivery-secret", "job-123", "runner-123", "should-never-appear", "safe"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("telemetry value %q leaked into EMF: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, `"Provider":"gcp"`) || !strings.Contains(output, `"Operation":"ready"`) {
		t.Fatalf("approved dimensions missing: %s", output)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	for _, cfg := range []Config{
		{Namespace: "AWS/Reserved", DimensionAllowlist: []string{"Environment"}},
		{Namespace: "", DimensionAllowlist: []string{"Environment"}},
		{Namespace: "App", Dimensions: map[string]string{"JobID": "x"}, DimensionAllowlist: []string{"Environment"}},
		{Namespace: "App", DimensionAllowlist: []string{"SecretToken"}},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("expected invalid config to fail: %#v", cfg)
		}
	}
}
