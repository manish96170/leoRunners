package policy

import (
	"errors"
	"reflect"
	"testing"
)

func testAdapter(t *testing.T) *Adapter {
	t.Helper()
	a, err := New(Config{Enabled: true, Tenants: map[string]TenantPolicy{
		"tenant-a": {Enabled: true, Extensions: map[string]bool{"diagnostics": true}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAcceptsOnlyDeterministicAdvisoryOutput(t *testing.T) {
	a := testAdapter(t)
	first := Output{JobID: "job-1", Actions: []ExtensionAction{{Action: ActionLink, URL: "https://example.test"}, {Action: ActionAnnotate, Title: "note"}, {Action: ActionAnnotate, Title: "note"}}, Annotations: []Annotation{{Path: "main.go", StartLine: 4, EndLine: 4, Level: "warning", Message: "check"}}, Metadata: map[string]string{"source": "test"}}
	second := Output{JobID: "job-1", Actions: []ExtensionAction{{Action: ActionAnnotate, Title: "note"}, {Action: ActionLink, URL: "https://example.test"}}, Annotations: first.Annotations, Metadata: first.Metadata}
	d1, err := a.Evaluate("tenant-a", "diagnostics", first)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := a.Evaluate("tenant-a", "diagnostics", second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d1.Output, d2.Output) || len(d1.Output.Actions) != 2 {
		t.Fatalf("non-deterministic output: %#v %#v", d1.Output, d2.Output)
	}
	if d1.Output.Version != CurrentVersion || d1.Output.TenantID != "tenant-a" || d1.Output.ExtensionID != "diagnostics" || !d1.Accepted {
		t.Fatalf("decision = %#v", d1)
	}
}

func TestRejectsLifecycleSecurityAndCredentialActions(t *testing.T) {
	unsafe := []AdvisoryAction{"retry", "rerun", "cancel", "provision", "terminate", "change-security", "access-credential", "grant-permission", "schedule"}
	for _, action := range unsafe {
		a := testAdapter(t)
		decision, err := a.Evaluate("tenant-a", "diagnostics", Output{JobID: "job-1", Actions: []ExtensionAction{{Action: action}}})
		if !errors.Is(err, ErrPolicyViolation) || decision.Accepted {
			t.Fatalf("action %q: decision=%#v err=%v", action, decision, err)
		}
	}
}

func TestTenantAndExtensionEnablement(t *testing.T) {
	a := testAdapter(t)
	if _, err := a.Evaluate("tenant-b", "diagnostics", Output{JobID: "job"}); !errors.Is(err, ErrTenantDenied) {
		t.Fatalf("tenant error = %v", err)
	}
	if _, err := a.Evaluate("tenant-a", "other", Output{JobID: "job"}); !errors.Is(err, ErrExtensionDenied) {
		t.Fatalf("extension error = %v", err)
	}
	if _, err := (&Adapter{}).Evaluate("tenant-a", "diagnostics", Output{JobID: "job"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
}

func TestRejectsInvalidOutputAndDoesNotRetainCallerSlices(t *testing.T) {
	a := testAdapter(t)
	output := Output{JobID: "job", Actions: []ExtensionAction{{Action: ActionExplain}}, Annotations: []Annotation{{Path: "x.go", Level: "bad", Message: "x"}}}
	if _, err := a.Evaluate("tenant-a", "diagnostics", output); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("invalid output error = %v", err)
	}
	output.Annotations[0].Level = "warning"
	decision, err := a.Evaluate("tenant-a", "diagnostics", output)
	if err != nil {
		t.Fatal(err)
	}
	output.Actions[0].Title = "mutated"
	if decision.Output.Actions[0].Title != "" {
		t.Fatal("decision changed when caller action was mutated")
	}
	decision.Output.Actions[0].Title = "changed"
	if output.Actions[0].Title != "mutated" {
		t.Fatal("decision retained caller slice")
	}
}

func TestOutputIdentityAndLimits(t *testing.T) {
	a, err := New(Config{Enabled: true, MaxActions: 1, Tenants: map[string]TenantPolicy{"t": {Enabled: true, Extensions: map[string]bool{"x": true}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Evaluate("t", "x", Output{TenantID: "other", JobID: "j"}); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("identity error = %v", err)
	}
	if _, err := a.Evaluate("t", "x", Output{JobID: "j", Actions: []ExtensionAction{{Action: ActionExplain}, {Action: ActionLink}}}); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("limit error = %v", err)
	}
}
