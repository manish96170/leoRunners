package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Severity string

const (
	Pass Severity = "pass"
	Warn Severity = "warn"
	Fail Severity = "fail"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Rule     string   `json:"rule"`
	Path     string   `json:"path"`
	Line     int      `json:"line"`
	Message  string   `json:"message"`
}

type Report struct {
	Version  string    `json:"version"`
	Status   Severity  `json:"status"`
	Root     string    `json:"root"`
	Scanned  int       `json:"scanned_files"`
	Findings []Finding `json:"findings"`
	Checks   []string  `json:"checks"`
	Finished time.Time `json:"finished_at"`
}

type scanner struct {
	root     string
	findings []Finding
	scanned  int
	seen     map[string]struct{}
}

var (
	awsKey       = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	githubToken  = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)
	privateKey   = regexp.MustCompile(`-----BEGIN (?:RSA |OPENSSH |EC |DSA )?PRIVATE KEY-----`)
	secretAssign = regexp.MustCompile(`(?i)\b(?:aws_secret_access_key|aws_access_key_id|github_token|github_pat|api[_-]?key|client[_-]?secret|password|secret)\s*[:=]\s*["']?([^"'\s#}]*)`)
	unsafeTrue   = regexp.MustCompile(`(?i)\b(privileged|allowPrivilegeEscalation|hostNetwork|hostPID|hostIPC)\s*:\s*["']?true\b`)
	zeroUser     = regexp.MustCompile(`(?i)\brunAsUser\s*:\s*0\b`)
	broadRBAC    = regexp.MustCompile(`(?i)(cluster-admin|system:masters|(?:verbs|resources|apiGroups)\s*:\s*\[\s*["']?\*["']?)`)
	secretFile   = regexp.MustCompile(`(?i)(^|/)(credentials(?:\.json)?|\.env(?:\..*)?|id_(?:rsa|ed25519)|[^/]+\.(?:pem|key))$`)
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", ".", "repository root to scan")
	format := fs.String("format", "text", "text or json")
	reportPath := fs.String("report", "", "optional output report path")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "format must be text or json")
		os.Exit(2)
	}
	report, err := Scan(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var output []byte
	if *format == "json" {
		output, err = json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		output = append(output, '\n')
	} else {
		output = []byte(renderText(report))
	}
	if *reportPath != "" {
		if err := writeReport(*reportPath, output); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	if _, err := os.Stdout.Write(output); err != nil {
		os.Exit(2)
	}
	if report.Status == Fail {
		os.Exit(1)
	}
}

func Scan(root string) (Report, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
		return Report{}, fmt.Errorf("scan root is not a directory: %s", root)
	}
	s := &scanner{root: absolute, seen: make(map[string]struct{})}
	err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || (path != absolute && path == filepath.Join(absolute, "tools", "security-validation")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		if !isScannable(rel) {
			return nil
		}
		return s.scanFile(path, filepath.ToSlash(rel))
	})
	if err != nil {
		return Report{}, err
	}
	sort.Slice(s.findings, func(i, j int) bool {
		a, b := s.findings[i], s.findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
	status := Pass
	for _, finding := range s.findings {
		if finding.Severity == Fail {
			status = Fail
			break
		}
		if finding.Severity == Warn {
			status = Warn
		}
	}
	return Report{
		Version: "security-validation.v1",
		Status:  status,
		Root:    filepath.ToSlash(absolute), Scanned: s.scanned,
		Findings: s.findings,
		Checks:   []string{"credential-patterns", "secret-like-material", "container-privilege", "host-isolation", "rbac-scope", "ai-default"},
		Finished: time.Now().UTC(),
	}, nil
}

func isScannable(path string) bool {
	base := filepath.Base(path)
	if base == "Dockerfile" {
		return true
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) == 0 {
		return false
	}
	scoped := parts[0] == "deploy" || parts[0] == "ami" || parts[0] == "bootstrap" || parts[0] == "intelligence" || parts[0] == "cache" || parts[0] == "network" || parts[0] == "workloads" || parts[0] == "terraform"
	if !scoped {
		return false
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".yaml", ".yml", ".json", ".tf", ".tfvars", ".hcl", ".sh":
		return true
	default:
		return false
	}
}

func (s *scanner) scanFile(path, rel string) error {
	s.scanned++
	if secretFile.MatchString(filepath.ToSlash(rel)) {
		s.add(Fail, "secret-file", rel, 1, "secret-like credential file is present")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for lineNo, line := range lines {
		lineNo++
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if awsKey.MatchString(line) || githubToken.MatchString(line) || privateKey.MatchString(line) {
			s.add(Fail, "credential-material", rel, lineNo, "credential-shaped material is present")
		}
		if m := secretAssign.FindStringSubmatch(line); len(m) == 2 && isLiteralSecret(m[1], line) {
			s.add(Fail, "secret-like-material", rel, lineNo, "secret-like field contains a literal value")
		}
		if unsafeTrue.MatchString(line) {
			s.add(Fail, "privileged-or-host-access", rel, lineNo, "privileged container or host isolation bypass is enabled")
		}
		if zeroUser.MatchString(line) {
			s.add(Fail, "root-container", rel, lineNo, "container user is explicitly root")
		}
		if strings.Contains(strings.ToLower(line), "hostpath:") || strings.Contains(line, "/var/run/docker.sock") {
			s.add(Fail, "host-mount", rel, lineNo, "host filesystem or Docker socket access is configured")
		}
		if broadRBAC.MatchString(line) {
			s.add(Fail, "broad-rbac", rel, lineNo, "Kubernetes RBAC contains a broad administrator or wildcard grant")
		}
		s.scanAI(rel, lineNo, line)
	}
	return nil
}

func isLiteralSecret(value, line string) bool {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	if value == "" || value == "null" || value == "false" || value == "true" || value == "[]" {
		return false
	}
	if strings.HasPrefix(value, "${") || strings.HasPrefix(value, "<") || strings.HasPrefix(value, "REPLACE_") || strings.HasPrefix(value, "example.") || strings.Contains(strings.ToLower(line), "secretkeyref") || strings.Contains(strings.ToLower(line), "valuefrom") {
		return false
	}
	return true
}

func (s *scanner) scanAI(rel string, lineNo int, line string) {
	lower := strings.ToLower(rel)
	if !strings.Contains(lower, "intelligence") && !strings.Contains(lower, "configmap") {
		return
	}
	if regexp.MustCompile(`(?i)\b(?:enabled|ai_enabled)\s*:\s*["']?true\b`).MatchString(line) {
		if strings.Contains(lower, "config-disabled") || strings.Contains(lower, "configmap") {
			s.add(Fail, "ai-enabled-default", rel, lineNo, "AI is enabled in a default/runtime configuration")
		} else {
			s.add(Warn, "ai-explicit-opt-in", rel, lineNo, "AI-enabled example requires explicit review and consent")
		}
	}
}

func (s *scanner) add(severity Severity, rule, path string, line int, message string) {
	key := fmt.Sprintf("%s|%s|%d|%s", severity, rule, line, path)
	if _, ok := s.seen[key]; ok {
		return
	}
	s.seen[key] = struct{}{}
	s.findings = append(s.findings, Finding{Severity: severity, Rule: rule, Path: path, Line: line, Message: message})
}

func renderText(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "security validation: %s\nscanned files: %d\n", strings.ToUpper(string(report.Status)), report.Scanned)
	for _, f := range report.Findings {
		fmt.Fprintf(&b, "%s %-24s %s:%d %s\n", strings.ToUpper(string(f.Severity)), f.Rule, f.Path, f.Line, f.Message)
	}
	if len(report.Findings) == 0 {
		b.WriteString("PASS no policy violations found\n")
	}
	return b.String()
}

func writeReport(path string, data []byte) error {
	if path == "" {
		return errors.New("empty report path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".security-validation-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func writeJSON(w io.Writer, report Report) error { return json.NewEncoder(w).Encode(report) }
