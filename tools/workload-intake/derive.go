package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type allowlistRule struct {
	path       string
	capability string
	locator    string
}

var rules = []allowlistRule{
	{path: ".github/workflows", capability: "github-actions", locator: "workflow definition"},
	{path: "go.mod", capability: "go", locator: "module declaration"},
	{path: "Cargo.toml", capability: "rust", locator: "package manifest"},
	{path: "package.json", capability: "javascript", locator: "package manifest"},
	{path: "pnpm-lock.yaml", capability: "pnpm", locator: "lockfile"},
	{path: "yarn.lock", capability: "yarn", locator: "lockfile"},
	{path: "package-lock.json", capability: "npm", locator: "lockfile"},
	{path: "pyproject.toml", capability: "python", locator: "project manifest"},
	{path: "requirements.txt", capability: "python", locator: "dependency manifest"},
	{path: "Dockerfile", capability: "docker", locator: "container definition"},
	{path: "Makefile", capability: "make", locator: "build target"},
	{path: "terraform", capability: "terraform", locator: "infrastructure source"},
}

var secretContent = regexp.MustCompile(`(?i)(aws_secret_access_key|aws_session_token|github_token|gh_token|private_key|password|passwd|token|secret|api[_-]?key|authorization\s*:\s*bearer)\s*["']?\s*[:=]\s*["']?[^\s,"'}]{4,}|-----begin (rsa|ec|openssh|private) key-----`)
var workflowCommand = regexp.MustCompile(`(?m)^[ \t]*(?:run|command):[ \t]*([^\r\n]+)`)

func Derive(repo, commit string, limits Limits) (Report, error) {
	if err := verifyCheckout(repo, commit); err != nil {
		return Report{}, err
	}
	if limits.MaxFiles <= 0 || limits.MaxBytes <= 0 {
		return Report{}, errors.New("inspection limits must be positive")
	}
	files, err := discover(repo)
	if err != nil {
		return Report{}, err
	}
	if len(files) > limits.MaxFiles {
		return Report{}, fmt.Errorf("allowlisted file count %d exceeds limit %d", len(files), limits.MaxFiles)
	}
	var total int64
	var evidence []Evidence
	capabilities := map[string]Evidence{}
	workflow := ""
	command := ""
	for _, path := range files {
		filePath := filepath.Join(repo, path)
		info, err := os.Stat(filePath)
		if err != nil {
			return Report{}, fmt.Errorf("stat %s: %w", path, err)
		}
		if info.Size() > limits.MaxBytes-total {
			return Report{}, fmt.Errorf("inspected bytes exceed limit %d", limits.MaxBytes)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return Report{}, fmt.Errorf("read %s: %w", path, err)
		}
		total += int64(len(data))
		if total > limits.MaxBytes {
			return Report{}, fmt.Errorf("inspected bytes exceed limit %d", limits.MaxBytes)
		}
		text := string(data)
		if secretContent.MatchString(text) {
			return Report{}, fmt.Errorf("secret-shaped content rejected in %s", path)
		}
		capability := ruleFor(path)
		if capability == "github-actions" {
			workflow = path
			if match := workflowCommand.FindStringSubmatch(text); len(match) == 2 {
				command = strings.TrimSpace(match[1])
				if strings.Contains(command, "${{") || strings.Contains(command, "secrets.") {
					return Report{}, fmt.Errorf("dynamic or secret-shaped workflow command rejected in %s", path)
				}
			}
		}
		e := Evidence{Source: path, Locator: locatorFor(path), Status: "observed"}
		evidence = append(evidence, e)
		capabilities[capability] = e
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].Source < evidence[j].Source })
	capList := make([]Capability, 0, len(capabilities))
	for id, e := range capabilities {
		capList = append(capList, Capability{ID: id, Enabled: true, Required: id == "github-actions", Evidence: e})
	}
	sort.Slice(capList, func(i, j int) bool { return capList[i].ID < capList[j].ID })
	if workflow == "" {
		workflow = ""
	}
	return Report{
		SchemaVersion: "workload-intake/v1",
		Status:        "PASS",
		Repository:    repo,
		Commit:        commit,
		GeneratedAt:   time.Now().UTC(),
		Execution:     Execution{DirtyCheckoutRejected: true, WorkflowCommandsRun: false, AllowlistedFilesOnly: true, SecretScanPassed: true},
		Manifest:      Manifest{APIVersion: "workloads.leorunners.io/v1", Kind: "WorkloadManifest", Name: "derived-workload", Version: "1.0.0", Workload: Workload{ID: workloadID(repo, commit), Workflow: workflow, Command: command, Provenance: "repository-derived"}, Runner: Runner{OperatingSystem: "linux", Architecture: "unknown"}, Capabilities: capList},
		Evidence:      evidence,
		Summary:       Summary{FilesRead: len(files), BytesRead: total},
		Redaction:     Redaction{Status: "redacted", SecretsScanned: true, RawContentExcluded: true},
	}, nil
}

func discover(repo string) ([]string, error) {
	var found []string
	for _, rule := range rules {
		root := filepath.Join(repo, rule.path)
		info, err := os.Stat(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			found = append(found, filepath.ToSlash(rule.path))
			continue
		}
		err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(repo, path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(found)
	return found, nil
}

func ruleFor(path string) string {
	for _, rule := range rules {
		if path == rule.path || strings.HasPrefix(path, strings.TrimSuffix(rule.path, "/")+"/") {
			return rule.capability
		}
	}
	return "unknown"
}

func locatorFor(path string) string {
	for _, rule := range rules {
		if path == rule.path || strings.HasPrefix(path, strings.TrimSuffix(rule.path, "/")+"/") {
			return rule.locator
		}
	}
	return "allowlisted file"
}

func workloadID(repo, commit string) string {
	sum := sha256.Sum256([]byte(repo + "\x00" + commit))
	return "derived-" + hex.EncodeToString(sum[:])[:16]
}
