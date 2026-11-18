package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanFixturePassesWithExplicitOptInWarning(t *testing.T) {
	report, err := Scan("fixtures/clean")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Warn {
		t.Fatalf("status = %s, want warn", report.Status)
	}
	for _, f := range report.Findings {
		if f.Severity == Fail {
			t.Fatalf("unexpected failure: %+v", f)
		}
	}
}

func TestUnsafeFixtureFailsAllMajorPolicies(t *testing.T) {
	report, err := Scan("fixtures/unsafe")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Fail {
		t.Fatalf("status = %s, want fail", report.Status)
	}
	wanted := []string{"credential-material", "secret-like-material", "privileged-or-host-access", "host-mount", "broad-rbac", "ai-enabled-default"}
	for _, rule := range wanted {
		found := false
		for _, f := range report.Findings {
			if f.Rule == rule {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rule %q was not reported", rule)
		}
	}
}

func TestReportIsDeterministicExceptTimestamp(t *testing.T) {
	a, err := Scan("fixtures/unsafe")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Scan("fixtures/unsafe")
	if err != nil {
		t.Fatal(err)
	}
	a.Finished, b.Finished = a.Finished.UTC(), a.Finished.UTC()
	a.Finished = b.Finished
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) != string(bj) {
		t.Fatal("reports differ for identical input")
	}
}

func TestSecretValuesAreNotIncludedInReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	const secret = "super-secret-value-123"
	if err := os.WriteFile(path, []byte("ENV API_KEY="+secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), secret) {
		t.Fatal("secret leaked into report")
	}
}

func TestInvalidRoot(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing root accepted")
	}
}
