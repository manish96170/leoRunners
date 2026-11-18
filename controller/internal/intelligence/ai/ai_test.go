package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func boolPtr(v bool) *bool { return &v }

func TestTriggerRuleMatchesAllDimensions(t *testing.T) {
	rule := TriggerRule{
		Repository: "acme/widgets", Workflow: "build", Branch: "main",
		PullRequest: boolPtr(true), PullRequestNum: 42, Labels: []string{"linux", "trusted"}, Manual: boolPtr(false),
	}
	ctx := TriggerContext{
		Repository: "acme/widgets", Workflow: "build", Branch: "main",
		PullRequest: true, PullRequestNum: 42, Labels: []string{"trusted", "linux", "extra"}, Manual: false,
	}
	if !rule.Matches(ctx) {
		t.Fatal("expected trigger to match")
	}
	ctx.Branch = "release"
	if rule.Matches(ctx) {
		t.Fatal("branch mismatch unexpectedly matched")
	}
}

func TestDisabledByDefaultAndConfigurationValidation(t *testing.T) {
	a, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Analyze(context.Background(), Request{}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("default Analyze error = %v", err)
	}
	if _, err := New(Config{Enabled: true}); err == nil {
		t.Fatal("enabled configuration without provider was accepted")
	}
}

func TestAnalyzeBoundsLogAndContext(t *testing.T) {
	fake := &FakeProvider{Response: Response{
		Classification: ClassificationTestFailure,
		Explanation:    Explanation{Summary: "assertion failed"},
	}}
	analyzer, err := New(Config{Enabled: true, Provider: fake, Timeout: time.Second, MaxLogBytes: 5,
		Triggers: []TriggerRule{{Repository: "acme/widgets"}}})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := analyzer.Analyze(context.Background(), Request{
		Trigger: TriggerContext{Repository: "acme/widgets"},
		Failure: FailureLog{JobID: "job-1", Log: "0123456789"},
	})
	if err != nil || !decision.Accepted {
		t.Fatalf("Analyze() = %#v, err=%v", decision, err)
	}
	requests := fake.SnapshotRequests()
	if len(requests) != 1 || requests[0].Failure.Log != "01234" || !requests[0].Failure.Truncated {
		t.Fatalf("bounded request = %#v", requests)
	}
}

func TestAnalyzeRequiresMatchingTrigger(t *testing.T) {
	fake := &FakeProvider{}
	analyzer, err := New(Config{Enabled: true, Provider: fake, Triggers: []TriggerRule{{Workflow: "release"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = analyzer.Analyze(context.Background(), Request{Trigger: TriggerContext{Workflow: "build"}, Failure: FailureLog{JobID: "job", Log: "failed"}})
	if !errors.Is(err, ErrNoTrigger) {
		t.Fatalf("error = %v", err)
	}
	if len(fake.SnapshotRequests()) != 0 {
		t.Fatal("provider called for an unmatched trigger")
	}
}

func TestPolicyGateRejectsLifecycleAndSecurityActions(t *testing.T) {
	_, err := (Gate{}).Evaluate(Response{Classification: ClassificationUnknown, Recommendations: []Recommendation{{Action: ActionTerminate}, {Action: ActionChangeSecurity}}})
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("unsafe response error = %v", err)
	}
	decision, err := (Gate{}).Evaluate(Response{Classification: ClassificationInfrastructure, Recommendations: []Recommendation{{Action: ActionAnnotate}}})
	if err != nil || !decision.Accepted || len(decision.RejectedActions) != 0 {
		t.Fatalf("safe response = %#v, err=%v", decision, err)
	}
}

func TestAnalyzeHonorsBoundedContext(t *testing.T) {
	fake := &FakeProvider{Delay: time.Hour}
	analyzer, err := New(Config{Enabled: true, Provider: fake, Timeout: 5 * time.Millisecond, Triggers: []TriggerRule{{}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = analyzer.Analyze(context.Background(), Request{Trigger: TriggerContext{}, Failure: FailureLog{JobID: "job", Log: strings.Repeat("x", 3)}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestFakeProviderDoesNotExposeMutableRequest(t *testing.T) {
	fake := &FakeProvider{Response: Response{Classification: ClassificationUnknown}}
	request := Request{Trigger: TriggerContext{Labels: []string{"linux"}}, Failure: FailureLog{JobID: "job", Log: "failure"}, Metadata: map[string]string{"key": "value"}}
	if _, err := fake.Analyze(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Trigger.Labels[0] = "changed"
	request.Metadata["key"] = "changed"
	snapshot := fake.SnapshotRequests()
	if snapshot[0].Trigger.Labels[0] != "linux" || snapshot[0].Metadata["key"] != "value" {
		t.Fatal("fake provider retained mutable caller data")
	}
}
