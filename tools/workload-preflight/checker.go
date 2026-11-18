package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Status string

const (
	Pass Status = "pass"
	Warn Status = "warn"
	Fail Status = "fail"
)

type Manifest struct {
	Name           string        `json:"name"`
	Architecture   string        `json:"architecture"`
	MinDiskBytes   uint64        `json:"min_disk_bytes"`
	MinMemoryBytes uint64        `json:"min_memory_bytes"`
	Commands       []Requirement `json:"commands"`
	Docker         *DockerCheck  `json:"docker,omitempty"`
	Browsers       []Requirement `json:"browsers"`
}

type Requirement struct {
	Name        string   `json:"name"`
	MinVersion  string   `json:"min_version,omitempty"`
	VersionArgs []string `json:"version_args,omitempty"`
	Required    *bool    `json:"required,omitempty"`
}

type DockerCheck struct {
	Required   bool   `json:"required"`
	MinVersion string `json:"min_version,omitempty"`
}

type Result struct {
	Name       string  `json:"name"`
	Root       string  `json:"root"`
	StartedAt  string  `json:"started_at"`
	FinishedAt string  `json:"finished_at"`
	Summary    Summary `json:"summary"`
	Checks     []Check `json:"checks"`
}

type Summary struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

type Check struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Status   Status `json:"status"`
	Message  string `json:"message"`
	Required string `json:"required,omitempty"`
	Observed string `json:"observed,omitempty"`
}

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Architecture) == "" {
		return fmt.Errorf("architecture is required")
	}
	seen := map[string]bool{}
	for _, requirement := range append(append([]Requirement{}, m.Commands...), m.Browsers...) {
		if strings.TrimSpace(requirement.Name) == "" {
			return fmt.Errorf("command and browser names must not be empty")
		}
		if seen[requirement.Name] {
			return fmt.Errorf("duplicate requirement %q", requirement.Name)
		}
		seen[requirement.Name] = true
	}
	if m.Docker != nil && m.Docker.Required && seen["docker"] {
		return fmt.Errorf("docker must not also appear in commands or browsers")
	}
	return nil
}

func Run(manifest Manifest, root string) Result {
	started := time.Now().UTC()
	result := Result{Name: manifest.Name, StartedAt: started.Format(time.RFC3339Nano), Checks: make([]Check, 0)}
	resolved, err := resolveRoot(root)
	if err != nil {
		result.Root = root
		result.Checks = append(result.Checks, Check{ID: "root", Category: "filesystem", Status: Fail, Message: err.Error()})
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result.recount()
		return result
	}
	result.Root = resolved
	result.Checks = append(result.Checks, Check{ID: "architecture", Category: "system", Status: statusFor(manifest.Architecture == runtime.GOARCH), Message: fmt.Sprintf("host architecture is %s", runtime.GOARCH), Required: manifest.Architecture, Observed: runtime.GOARCH})
	if manifest.MinDiskBytes > 0 {
		free, diskErr := FreeBytes(resolved)
		result.Checks = append(result.Checks, quantityCheck("disk", "filesystem", manifest.MinDiskBytes, free, diskErr))
	}
	if manifest.MinMemoryBytes > 0 {
		memory, memoryErr := AvailableMemory()
		result.Checks = append(result.Checks, quantityCheck("memory", "system", manifest.MinMemoryBytes, memory, memoryErr))
	}
	for _, requirement := range manifest.Commands {
		result.Checks = append(result.Checks, checkCommand(requirement, resolved, "command"))
	}
	if manifest.Docker != nil {
		result.Checks = append(result.Checks, checkDocker(*manifest.Docker, resolved))
	}
	for _, requirement := range manifest.Browsers {
		result.Checks = append(result.Checks, checkCommand(requirement, resolved, "browser"))
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result.recount()
	return result
}

func (r *Result) recount() {
	for _, check := range r.Checks {
		switch check.Status {
		case Pass:
			r.Summary.Pass++
		case Warn:
			r.Summary.Warn++
		case Fail:
			r.Summary.Fail++
		}
	}
}

func statusFor(ok bool) Status {
	if ok {
		return Pass
	}
	return Fail
}

func quantityCheck(id, category string, required, observed uint64, err error) Check {
	check := Check{ID: id, Category: category, Required: strconv.FormatUint(required, 10), Observed: strconv.FormatUint(observed, 10)}
	if err != nil {
		check.Status = Fail
		check.Message = err.Error()
		return check
	}
	check.Status = statusFor(observed >= required)
	check.Message = fmt.Sprintf("available %d bytes; required %d bytes", observed, required)
	return check
}

func checkCommand(requirement Requirement, root, category string) Check {
	check := Check{ID: category + "." + requirement.Name, Category: category, Required: requirement.MinVersion}
	path, err := exec.LookPath(requirement.Name)
	if err != nil {
		check.Status = requirementStatus(requirement)
		check.Message = fmt.Sprintf("%s is unavailable", requirement.Name)
		return check
	}
	args := requirement.VersionArgs
	if len(args) == 0 {
		args = []string{"--version"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path, args...)
	command.Dir = root
	output, runErr := command.CombinedOutput()
	version := strings.TrimSpace(string(output))
	check.Observed = version
	if runErr != nil {
		check.Status = requirementStatus(requirement)
		check.Message = fmt.Sprintf("%s was found at %s but version probe failed: %v", requirement.Name, path, runErr)
		return check
	}
	if requirement.MinVersion != "" && CompareVersions(version, requirement.MinVersion) < 0 {
		check.Status = requirementStatus(requirement)
		check.Message = fmt.Sprintf("%s version is below minimum %s", requirement.Name, requirement.MinVersion)
		return check
	}
	check.Status = Pass
	check.Message = fmt.Sprintf("%s is available at %s", requirement.Name, path)
	return check
}

func checkDocker(requirement DockerCheck, root string) Check {
	check := checkCommand(Requirement{Name: "docker", MinVersion: requirement.MinVersion, Required: &requirement.Required}, root, "docker")
	if check.Status == Pass {
		path, _ := exec.LookPath("docker")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		probe := exec.CommandContext(ctx, path, "info", "--format", "{{.ServerVersion}}")
		probe.Dir = root
		output, err := probe.CombinedOutput()
		if err != nil {
			check.Status = requirementStatus(Requirement{Required: &requirement.Required})
			check.Message = fmt.Sprintf("Docker CLI is available but daemon is unavailable: %v", err)
		} else {
			check.Observed += "; daemon=" + strings.TrimSpace(string(output))
			check.Message = "Docker CLI and local daemon are available"
		}
	}
	return check
}

func requirementStatus(requirement Requirement) Status {
	if requirement.Required != nil && !*requirement.Required {
		return Warn
	}
	return Fail
}

func CompareVersions(observed, required string) int {
	parse := func(value string) []int {
		var parts []int
		current := ""
		for _, r := range value {
			if r >= '0' && r <= '9' {
				current += string(r)
			} else if current != "" {
				n, _ := strconv.Atoi(current)
				parts = append(parts, n)
				current = ""
			}
		}
		if current != "" {
			n, _ := strconv.Atoi(current)
			parts = append(parts, n)
		}
		return parts
	}
	a, b := parse(observed), parse(required)
	for len(a) < len(b) {
		a = append(a, 0)
	}
	for len(b) < len(a) {
		b = append(b, 0)
	}
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
