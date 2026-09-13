package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	defaultTimeout = 2 * time.Minute
	maxTimeout     = 10 * time.Minute
	defaultOutput  = 64 << 10
	maxOutput      = 1 << 20
)

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var safePath = regexp.MustCompile(`^[A-Za-z0-9._/@+-]+$`)
var secretPattern = regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|bearer\s+[A-Za-z0-9._-]+|(?:password|passwd|secret|token|api[_-]?key)\s*[:=]\s*[^\s,;]+|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)`)

type options struct {
	Timeout   time.Duration
	MaxOutput int
}
type commandPlan struct {
	Dir, Name string
	Args      []string
}

func loadRequest(path string) (request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return request{}, err
	}
	var r request
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&r); err != nil {
		return request{}, fmt.Errorf("decode request: %w", err)
	}
	if err := validateRequest(r); err != nil {
		return request{}, err
	}
	return r, nil
}

func validateRequest(r request) error {
	if r.APIVersion != "workload-intake.leorunners.io/v1" || r.Kind != "WorkloadIntake" {
		return errors.New("request apiVersion/kind is not supported")
	}
	if r.Metadata.Name == "" {
		return errors.New("request metadata.name is required")
	}
	if !fullCommit.MatchString(r.Spec.Repository.Commit) {
		return errors.New("request repository commit must be a lowercase full 40-character commit")
	}
	if r.Spec.Workload.ID == "" || len(r.Spec.Workload.Command) == 0 || len(r.Spec.Workload.Command) > 240 {
		return errors.New("request workload identity or command is invalid")
	}
	if !digestPattern.MatchString(r.Spec.Workload.CommandDigest) {
		return errors.New("request commandDigest is invalid")
	}
	sum := sha256.Sum256([]byte(r.Spec.Workload.Command))
	if "sha256:"+hex.EncodeToString(sum[:]) != r.Spec.Workload.CommandDigest {
		return errors.New("request command digest does not match command")
	}
	if r.Spec.Workload.Shell != "bash" && r.Spec.Workload.Shell != "sh" {
		return errors.New("request shell is not supported by the direct runner")
	}
	if r.Spec.Observations.Network.EgressMode != "none" || r.Spec.Observations.Network.ExternalAccessApproved {
		return errors.New("network-enabled workload execution is refused")
	}
	if r.Spec.Redaction.Status != "redacted" || !r.Spec.Redaction.SecretsScanned || !r.Spec.Redaction.RawPayloadsExcluded {
		return errors.New("request redaction contract is incomplete")
	}
	approved, err := time.Parse(time.RFC3339, r.Spec.Provenance.Approval.ApprovedAt)
	if err != nil {
		return errors.New("request approval timestamp is invalid")
	}
	expires, err := time.Parse(time.RFC3339, r.Spec.Provenance.Approval.ExpiresAt)
	if err != nil || !expires.After(approved) {
		return errors.New("request approval expiry is invalid")
	}
	if time.Now().UTC().After(expires) {
		return errors.New("request approval has expired")
	}
	return nil
}

func parseCommand(command string) (commandPlan, error) {
	if strings.ContainsAny(command, "\r\n;|><$`\\") {
		return commandPlan{}, errors.New("command contains forbidden shell syntax")
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return commandPlan{}, errors.New("command is empty")
	}
	dir := "."
	if parts[0] == "cd" {
		if len(parts) < 4 || parts[2] != "&&" {
			return commandPlan{}, errors.New("only 'cd DIR && COMMAND' is supported")
		}
		dir, parts = parts[1], parts[3:]
	}
	if !safePath.MatchString(dir) || filepath.IsAbs(dir) || dir == ".." || strings.HasPrefix(filepath.Clean(dir), "../") {
		return commandPlan{}, errors.New("command directory escapes the worktree")
	}
	if len(parts) == 0 {
		return commandPlan{}, errors.New("command is empty")
	}
	for _, part := range parts {
		if !safePath.MatchString(part) {
			return commandPlan{}, errors.New("command contains unsupported argument syntax")
		}
	}
	name, args := parts[0], parts[1:]
	allowed := false
	switch name {
	case "go":
		allowed = len(args) == 2 && (args[0] == "test" || args[0] == "vet" || args[0] == "build") && args[1] == "./..."
		allowed = allowed || (len(args) == 3 && args[0] == "test" && args[1] == "-v" && args[2] == "./...")
	case "cargo":
		allowed = len(args) == 1 && args[0] == "test"
	case "npm", "pnpm", "yarn":
		allowed = len(args) == 1 && args[0] == "test"
	}
	if !allowed {
		return commandPlan{}, errors.New("command is not on the offline allowlist")
	}
	return commandPlan{Dir: dir, Name: name, Args: args}, nil
}

func verifyCheckout(repo, commit string) error {
	head, err := gitOutput(repo, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	if head != commit {
		return errors.New("checkout HEAD does not match pinned commit")
	}
	status, err := gitOutput(repo, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		return errors.New("checkout is dirty")
	}
	return nil
}

func gitOutput(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func Run(requestPath, repo string, opts options) (Evidence, error) {
	started := time.Now().UTC()
	report := Evidence{SchemaVersion: "controlled-workload/v1", Status: "BLOCKED", StartedAt: started, NetworkMode: "none", Redaction: Redaction{Status: "redacted", SecretsScanned: true, RawOutputExcluded: true}}
	r, err := loadRequest(requestPath)
	if err != nil {
		return report, err
	}
	report.RequestName, report.Commit, report.WorkloadID = r.Metadata.Name, r.Spec.Repository.Commit, r.Spec.Workload.ID
	report.RepositoryHash = hashString(repo)
	plan, err := parseCommand(r.Spec.Workload.Command)
	if err != nil {
		return report, err
	}
	report.CommandAllowlisted = true
	if err := verifyCheckout(repo, r.Spec.Repository.Commit); err != nil {
		return report, err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.Timeout > maxTimeout {
		return report, errors.New("timeout exceeds maximum")
	}
	if opts.MaxOutput <= 0 {
		opts.MaxOutput = defaultOutput
	}
	if opts.MaxOutput > maxOutput {
		return report, errors.New("output limit exceeds maximum")
	}
	tmp, err := os.MkdirTemp("", ".leo-controlled-workload-")
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(tmp)
	worktree := filepath.Join(tmp, "worktree")
	if _, err := gitOutput(repo, "worktree", "add", "--detach", worktree, r.Spec.Repository.Commit); err != nil {
		return report, fmt.Errorf("create temporary worktree: %w", err)
	}
	removed := false
	cleanup := func() { _, _ = gitOutput(repo, "worktree", "remove", "--force", worktree); removed = true }
	workDir := filepath.Join(worktree, filepath.Clean(plan.Dir))
	if !within(worktree, workDir) {
		return report, errors.New("command directory escapes temporary worktree")
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, plan.Name, plan.Args...)
	cmd.Dir = workDir
	cmd.Env = offlineEnv()
	var stdout, stderr boundedBuffer
	stdout.limit, stderr.limit = opts.MaxOutput, opts.MaxOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	cleanup()
	report.FinishedAt, report.DurationMillis = time.Now().UTC(), time.Since(started).Milliseconds()
	report.Stdout, report.Stderr = redact(stdout.String()), redact(stderr.String())
	report.OutputTruncated = stdout.truncated || stderr.truncated
	report.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	report.WorktreeRemoved = removed
	if runErr == nil {
		report.Status, report.ExitCode = "PASS", 0
		return report, nil
	}
	report.Status = "FAIL"
	if report.TimedOut {
		report.Status = "BLOCKED"
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		report.ExitCode = exitErr.ExitCode()
	} else {
		report.ExitCode = -1
	}
	return report, runErr
}

func offlineEnv() []string {
	keep := make([]string, 0)
	for _, item := range os.Environ() {
		name := item[:strings.IndexByte(item, '=')]
		upper := strings.ToUpper(name)
		if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "PRIVATE") || strings.HasPrefix(upper, "AWS_") || strings.HasPrefix(upper, "GITHUB_") {
			continue
		}
		keep = append(keep, item)
	}
	return append(keep, "GOPROXY=off", "GOSUMDB=off", "CARGO_NET_OFFLINE=true", "npm_config_offline=true", "CI=true")
}

func within(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type boundedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
func redact(value string) string { return secretPattern.ReplaceAllString(value, "[REDACTED]") }

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".controlled-workload-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
