package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFakeTimelineIsDeterministicAndOrdered(t *testing.T) {
	first, err := (FakeTimeline{ID: "sample-1", Durations: DefaultDurations()}).Replay()
	if err != nil {
		t.Fatal(err)
	}
	second, err := (FakeTimeline{ID: "sample-1", Durations: DefaultDurations()}).Replay()
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("replaying a fake timeline changed its output")
	}
	if first.Durations["total_startup"] != 22600 {
		t.Fatalf("total startup = %d", first.Durations["total_startup"])
	}
}

func TestPercentilesUseNearestRank(t *testing.T) {
	values := []int64{10, 20, 30, 40, 50}
	if got := percentile(values, .50); got != 30 {
		t.Fatalf("p50 = %d", got)
	}
	if got := percentile(values, .95); got != 50 {
		t.Fatalf("p95 = %d", got)
	}
	if got := percentile(values, .99); got != 50 {
		t.Fatalf("p99 = %d", got)
	}
}

func TestProvenanceLinksContractAndManifest(t *testing.T) {
	dir := t.TempDir()
	contract := filepath.Join(dir, "contract.json")
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(contract, []byte(`{"metadata":{"version":"1.0.0"},"spec":{"source":{"ami_id":"ami-0123456789abcdef0"},"artifact":{"version":"1.0.0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"artifact":{"image_id":"ami-0123456789abcdef0","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","source_ami":"ami-0123456789abcdef0","profile_version":"1.0.0"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	provenance, err := loadProvenance(contract, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !provenance.Redacted || !provenance.SecretFree || provenance.ImageDigest == "" {
		t.Fatalf("unexpected provenance: %+v", provenance)
	}
}

func TestRegressionGateFailsOnExceededBaseline(t *testing.T) {
	report := Report{Samples: []Sample{{}}, Summary: map[string]Percentiles{"total_startup": {P95: 120, P99: 120}}, Thresholds: Thresholds{MaxTotalStartupMS: 200, MaxP95TotalStartupMS: 200, MaxP99TotalStartupMS: 200, MaxRegressionPercent: 10}, Provenance: Provenance{ImageDigest: "sha256:x", Redacted: true, SecretFree: true}}
	baseline := &Report{Summary: map[string]Percentiles{"total_startup": {P95: 100}}}
	checkGates(&report, baseline)
	if report.Status != "FAIL" || report.Gates["regression"] {
		t.Fatalf("expected regression failure: %+v", report)
	}
}
