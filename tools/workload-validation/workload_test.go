package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFakeReportIsDeterministicAndComplete(t *testing.T) {
	a := FakeReport("job-1", "org/repo", "ci.yml", "base-linux-x64")
	b := FakeReport("job-1", "org/repo", "ci.yml", "base-linux-x64")
	if a.Duration != b.Duration || a.PlatformTimes != b.PlatformTimes || a.WorkloadTimes != b.WorkloadTimes {
		t.Fatal("fake report changed between runs")
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.WorkloadTimes.Total() == 0 || a.PlatformTimes.Total() == 0 {
		t.Fatal("expected platform and workload timings")
	}
}

func TestCommandCapturesExitStatusAndRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	report := RunCommand(context.Background(), CommandConfig{Repository: dir, Workflow: "ci.yml", Command: "printf 'token=super-secret\\n'; exit 7", Timeout: time.Second})
	if report.Status != "failed" || report.ExitCode != 7 {
		t.Fatalf("got status=%s exit=%d", report.Status, report.ExitCode)
	}
	if strings.Contains(report.Log, "super-secret") || !strings.Contains(report.Log, "[REDACTED]") {
		t.Fatalf("secret was not redacted: %q", report.Log)
	}
	if strings.Contains(report.Command, "super-secret") {
		t.Fatalf("secret was retained in command: %q", report.Command)
	}
}

func TestCommandTimeout(t *testing.T) {
	report := RunCommand(context.Background(), CommandConfig{Repository: t.TempDir(), Workflow: "ci.yml", Command: "sleep 1", Timeout: 10 * time.Millisecond})
	if report.Status != "timed_out" || !report.TimedOut || report.ExitCode != -1 {
		t.Fatalf("unexpected timeout report: %+v", report)
	}
}

func TestJSONReportHasStableSchema(t *testing.T) {
	data := mustReportJSON(t, FakeReport("job-1", "org/repo", "ci.yml", "base-linux-x64"))
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "platform_timings", "workload_timings", "workload", "status", "exit_code"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestEvidenceRejectsUnredactedOrUnknownProvenance(t *testing.T) {
	report := FakeReport("job-1", "org/repo", "ci.yml", "base-linux-x64")
	report.Workload.Provenance = "unknown"
	if err := report.Validate(); err == nil {
		t.Fatal("expected unknown provenance to fail")
	}
	report = FakeReport("job-1", "org/repo", "ci.yml", "base-linux-x64")
	report.Workload.RawPayloadsExcluded = false
	if err := report.Validate(); err == nil {
		t.Fatal("expected raw payloads to fail closed")
	}
}

func mustReportJSON(t *testing.T, report Report) []byte {
	t.Helper()
	var builder strings.Builder
	if err := WriteJSON(&builder, report); err != nil {
		t.Fatal(err)
	}
	return []byte(builder.String())
}
