package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunUsesTemporaryWorktreeAndRedactsOutput(t *testing.T) {
	repo, commit := fixtureRepo(t, "go.mod", "module fixture.test\n\ngo 1.27.1\n", "runner_test.go", "package fixture\nimport \"testing\"\nfunc TestOutput(t *testing.T) { t.Log(\"token=super-secret-value\") }\n")
	request := requestFile(t, commit, "go test -v ./...")
	report, err := Run(request, repo, options{Timeout: 30 * time.Second, MaxOutput: 4096})
	if err != nil {
		t.Fatalf("run failed: %v; report=%+v", err, report)
	}
	if report.Status != "PASS" || report.ExitCode != 0 || !report.WorktreeRemoved {
		t.Fatalf("unexpected report: %+v", report)
	}
	if strings.Contains(report.Stdout+report.Stderr, "super-secret-value") || !strings.Contains(report.Stdout+report.Stderr, "[REDACTED]") {
		t.Fatalf("output was not redacted: %+v", report)
	}
}

func TestRunRejectsDirtyAndUnsafeRequests(t *testing.T) {
	repo, commit := fixtureRepo(t, "go.mod", "module fixture.test\n\ngo 1.27.1\n", "dummy.txt", "clean")
	request := requestFile(t, commit, "go test ./...")
	raw := readJSON(t, request)
	spec := raw["spec"].(map[string]any)
	workload := spec["workload"].(map[string]any)
	workload["command"] = "curl https://example.invalid"
	workload["commandDigest"] = digest("curl https://example.invalid")
	unsafe := filepath.Join(t.TempDir(), "unsafe.json")
	writeJSON(t, unsafe, raw)
	if _, err := Run(unsafe, repo, options{}); err == nil || (!strings.Contains(err.Error(), "allowlist") && !strings.Contains(err.Error(), "unsupported argument")) {
		t.Fatalf("expected unsafe command rejection, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dirty"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(request, repo, options{}); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected dirty rejection, got %v", err)
	}
}

func TestRunTimeoutAndAtomicOutput(t *testing.T) {
	repo, commit := fixtureRepo(t, "go.mod", "module fixture.test\n\ngo 1.27.1\n", "slow_test.go", "package fixture\nimport (\"testing\"; \"time\")\nfunc TestSlow(t *testing.T) { time.Sleep(2*time.Second) }\n")
	request := requestFile(t, commit, "go test ./...")
	report, err := Run(request, repo, options{Timeout: 20 * time.Millisecond, MaxOutput: 4096})
	if err == nil || report.Status != "BLOCKED" || !report.TimedOut {
		t.Fatalf("expected timeout, report=%+v err=%v", report, err)
	}
	out := filepath.Join(t.TempDir(), "evidence.json")
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(out, data); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("report mode %o", info.Mode().Perm())
	}
}

func fixtureRepo(t *testing.T, file, content, extraFile, extra string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "Test"}} {
		gitTest(t, dir, args...)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, extraFile), []byte(extra), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, dir, "add", ".")
	gitTest(t, dir, "commit", "-q", "-m", "fixture")
	return dir, strings.TrimSpace(gitTest(t, dir, "rev-parse", "HEAD"))
}

func requestFile(t *testing.T, commit, command string) string {
	t.Helper()
	now := time.Now().UTC()
	raw := map[string]any{"apiVersion": "workload-intake.leorunners.io/v1", "kind": "WorkloadIntake", "metadata": map[string]any{"name": "test-intake"}, "spec": map[string]any{
		"repository": map[string]any{"commit": commit}, "workload": map[string]any{"id": "test-workload", "command": command, "commandDigest": digest(command), "shell": "bash"},
		"provenance":   map[string]any{"approval": map[string]any{"approvedAt": now.Add(-time.Minute).Format(time.RFC3339), "expiresAt": now.Add(time.Hour).Format(time.RFC3339)}},
		"observations": map[string]any{"network": map[string]any{"egressMode": "none", "externalAccessApproved": false}}, "redaction": map[string]any{"status": "redacted", "secretsScanned": true, "rawPayloadsExcluded": true},
	}}
	path := filepath.Join(t.TempDir(), "request.json")
	writeJSON(t, path, raw)
	return path
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}
func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return string(out)
}
