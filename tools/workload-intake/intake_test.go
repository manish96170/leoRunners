package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveAcceptsCleanPinnedCheckoutAndNeverRunsWorkflow(t *testing.T) {
	repo, commit := fixtureRepo(t, "go.mod", "module example.test/fixture\n\ngo 1.27\n")
	report, err := Derive(repo, commit, Limits{MaxFiles: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "PASS" || report.Execution.WorkflowCommandsRun {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.Evidence) != 1 || report.Evidence[0].Source != "go.mod" {
		t.Fatalf("unexpected evidence: %+v", report.Evidence)
	}
	if report.Manifest.Capabilities[0].ID != "go" {
		t.Fatalf("unexpected capability: %+v", report.Manifest.Capabilities)
	}
}

func TestDeriveRejectsDirtyAndUnpinnedCheckouts(t *testing.T) {
	repo, commit := fixtureRepo(t, "go.mod", "module example.test/fixture\n")
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Derive(repo, commit, Limits{MaxFiles: 8, MaxBytes: 4096}); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected dirty rejection, got %v", err)
	}
	if _, err := Derive(repo, strings.Repeat("a", 40), Limits{MaxFiles: 8, MaxBytes: 4096}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected pin rejection, got %v", err)
	}
}

func TestDeriveRejectsSecretShapedContentAndBounds(t *testing.T) {
	repo, commit := fixtureRepo(t, "package.json", `{"name":"fixture","token":"super-secret-value"}`)
	if _, err := Derive(repo, commit, Limits{MaxFiles: 8, MaxBytes: 4096}); err == nil || !strings.Contains(err.Error(), "secret-shaped") {
		t.Fatalf("expected secret rejection, got %v", err)
	}
	repo, commit = fixtureRepo(t, "go.mod", "module example.test/fixture\n")
	if _, err := Derive(repo, commit, Limits{MaxFiles: 8, MaxBytes: 4}); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("expected byte bound rejection, got %v", err)
	}
}

func TestJSONReportExcludesRawContent(t *testing.T) {
	repo, commit := fixtureRepo(t, "Makefile", "build:\n\t@echo never-run\n")
	report, err := Derive(repo, commit, Limits{MaxFiles: 8, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "never-run") || strings.Contains(string(data), "build:") {
		t.Fatalf("raw content leaked: %s", data)
	}
}

func fixtureRepo(t *testing.T, name, content string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "Test"}} {
		runGit(t, dir, args...)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "fixture")
	return dir, strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return string(out)
}
