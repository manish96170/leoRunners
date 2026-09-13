package eval

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leo-runners/ci-platform/controller/internal/intelligence/ai"
	"github.com/leo-runners/ci-platform/controller/internal/intelligence/stats"
)

func TestEvaluateIsDeterministicAndUsesHistoricalTelemetry(t *testing.T) {
	f := Fixture{Version: "intelligence-evaluation.v1", DatasetID: "fixture-1", Provenance: ProvenanceSynthetic, Policy: "deterministic.v1", Telemetry: []TelemetrySample{
		{ID: "b", Provider: "aws", Outcome: stats.OutcomeFailure, TimingsMS: map[string]float64{"queue": 20}},
		{ID: "a", Provider: "aws", Outcome: stats.OutcomeSuccess, TimingsMS: map[string]float64{"queue": 10}},
	}, Cases: []Case{{ID: "case-1", Log: "failure details", MaxLogBytes: 4, ExpectedClass: ai.ClassificationTestFailure, ExpectedAction: ai.ActionAnnotate, ExpectedAccepted: true, Response: ai.Response{Classification: ai.ClassificationTestFailure, Recommendations: []ai.Recommendation{{Action: ai.ActionAnnotate}}}}}}
	report, err := Evaluate(f)
	if err != nil {
		t.Fatal(err)
	}
	if report.HistoricalStats.Count != 2 || report.BoundedLogBytes != 4 || report.ClassificationAccuracy != 1 || report.ActionAccuracy != 1 || report.AcceptanceAccuracy != 1 {
		t.Fatalf("report = %+v", report)
	}
	if err := Compare(report, Thresholds{MinClassificationAccuracy: 1, MinActionAccuracy: 1, MinAcceptanceAccuracy: 1, MaxBoundedLogBytes: 4, MaxPolicyViolations: 0}); err != nil {
		t.Fatal(err)
	}
	second, err := Evaluate(f)
	if err != nil || second.Fingerprint != report.Fingerprint {
		t.Fatalf("non-deterministic report: %v %+v", err, second)
	}
}

func TestUnsafeAdvisoryIsCountedAndRegressionFails(t *testing.T) {
	f := Fixture{Version: "intelligence-evaluation.v1", DatasetID: "unsafe", Provenance: ProvenanceSynthetic, Policy: "deterministic.v1", Telemetry: []TelemetrySample{{ID: "a", Provider: "aws", Outcome: stats.OutcomeFailure}}, Cases: []Case{{ID: "case-1", Log: "x", MaxLogBytes: 8, ExpectedClass: ai.ClassificationUnknown, ExpectedAction: ai.ActionNone, Response: ai.Response{Classification: ai.ClassificationUnknown, Recommendations: []ai.Recommendation{{Action: ai.ActionTerminate}}}}}}
	report, err := Evaluate(f)
	if err != nil {
		t.Fatal(err)
	}
	if report.PolicyViolations != 1 || report.RejectedCases != 1 {
		t.Fatalf("unsafe report = %+v", report)
	}
	if !errors.Is(Compare(report, Thresholds{MaxPolicyViolations: 0}), ErrRegression) {
		t.Fatal("unsafe advisory did not fail regression gate")
	}
}

func TestAIEnabledFixtureRejected(t *testing.T) {
	f := Fixture{Version: "intelligence-evaluation.v1", DatasetID: "bad", Provenance: ProvenanceReal, Policy: "p", AIEnabled: true, Telemetry: []TelemetrySample{{ID: "a", Provider: "aws", Outcome: stats.OutcomeSuccess}}, Cases: []Case{{ID: "a", Log: "x", MaxLogBytes: 1}}}
	if !errors.Is(func() error { _, err := Evaluate(f); return err }(), ErrInvalidFixture) {
		t.Fatal("enabled AI fixture accepted")
	}
}

func TestEvaluateJSONRejectsSecretsAndUnknownFields(t *testing.T) {
	valid := `{"version":"intelligence-evaluation.v1","dataset_id":"d","provenance":"synthetic","ai_enabled":false,"policy_version":"p","telemetry":[{"id":"t","provider":"aws","outcome":"success"}],"cases":[{"id":"c","log":"safe","max_log_bytes":4,"expected_action":"none","expected_classification":"unknown","expected_accepted":true,"response":{"classification":"unknown"}}]}`
	if _, err := EvaluateJSON(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	secret := strings.Replace(valid, `"safe"`, `"Bearer token-value"`, 1)
	if _, err := EvaluateJSON(strings.NewReader(secret)); !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("secret error = %v", err)
	}
	unknown := strings.Replace(valid, `"policy_version":"p"`, `"policy_version":"p","unknown":true`, 1)
	if _, err := EvaluateJSON(strings.NewReader(unknown)); !errors.Is(err, ErrInvalidFixture) {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := Load(bytes.NewBufferString(valid)); err != nil {
		t.Fatal(err)
	}
}

func TestCommittedEvaluationFixturesAreConsumedByHarness(t *testing.T) {
	for _, name := range []string{"evaluation-synthetic.v1.json", "evaluation-real.v1.json"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "..", "intelligence", name)
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			report, err := EvaluateJSON(file)
			if err != nil {
				t.Fatal(err)
			}
			if report.AIEnabled || report.TelemetryCount == 0 || report.CaseCount == 0 || report.Fingerprint == "" {
				t.Fatalf("unsafe or incomplete report: %+v", report)
			}
			if err := Compare(report, Thresholds{MinClassificationAccuracy: 1, MinActionAccuracy: 1, MinAcceptanceAccuracy: 1, MaxPolicyViolations: 0, MaxBoundedLogBytes: 96}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
