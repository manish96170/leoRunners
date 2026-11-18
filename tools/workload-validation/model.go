package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PlatformTimings are the runner lifecycle fields shared with image benchmarks.
type PlatformTimings struct {
	Queue        time.Duration `json:"queue"`
	Launch       time.Duration `json:"launch"`
	Boot         time.Duration `json:"boot"`
	Registration time.Duration `json:"registration"`
	Ready        time.Duration `json:"ready"`
	JobStart     time.Duration `json:"job_start"`
	Cleanup      time.Duration `json:"cleanup"`
}

func (p PlatformTimings) Total() time.Duration {
	return p.Queue + p.Launch + p.Boot + p.Registration + p.Ready + p.JobStart + p.Cleanup
}

// WorkloadTimings are observed command stages. Zero is valid for an omitted stage.
type WorkloadTimings struct {
	DependencyInstall time.Duration `json:"dependency_install"`
	Build             time.Duration `json:"build"`
	Tests             time.Duration `json:"tests"`
	Cypress           time.Duration `json:"cypress"`
	Docker            time.Duration `json:"docker"`
}

// WorkloadEvidence binds a report to the reviewed repository contract. Logs
// are deliberately represented only by redaction attestations, never raw
// payloads or credentials.
type WorkloadEvidence struct {
	ID                  string `json:"workload_id"`
	ManifestVersion     string `json:"manifest_version"`
	ManifestDigest      string `json:"manifest_digest"`
	RepositoryCommit    string `json:"repository_commit"`
	ManifestWorkflow    string `json:"manifest_workflow"`
	CommandDigest       string `json:"command_digest"`
	Provenance          string `json:"provenance"`
	RedactionStatus     string `json:"redaction_status"`
	SecretsScanned      bool   `json:"secrets_scanned"`
	RawPayloadsExcluded bool   `json:"raw_payloads_excluded"`
}

func (w WorkloadTimings) Total() time.Duration {
	return w.DependencyInstall + w.Build + w.Tests + w.Cypress + w.Docker
}

type Report struct {
	SchemaVersion string           `json:"schema_version"`
	ID            string           `json:"id"`
	Mode          string           `json:"mode"`
	Repository    string           `json:"repository"`
	Workflow      string           `json:"workflow"`
	Image         string           `json:"image"`
	Platform      string           `json:"platform"`
	Architecture  string           `json:"architecture"`
	Commit        string           `json:"commit,omitempty"`
	Command       string           `json:"command,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	FinishedAt    time.Time        `json:"finished_at"`
	Duration      time.Duration    `json:"duration"`
	ExitCode      int              `json:"exit_code"`
	Status        string           `json:"status"`
	TimedOut      bool             `json:"timed_out"`
	PlatformTimes PlatformTimings  `json:"platform_timings"`
	WorkloadTimes WorkloadTimings  `json:"workload_timings"`
	Workload      WorkloadEvidence `json:"workload"`
	Log           string           `json:"log,omitempty"`
	Error         string           `json:"error,omitempty"`
}

func (r Report) Validate() error {
	for name, value := range map[string]string{"id": r.ID, "repository": r.Repository, "workflow": r.Workflow, "image": r.Image, "platform": r.Platform, "architecture": r.Architecture, "mode": r.Mode, "status": r.Status} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if r.SchemaVersion == "" {
		return errors.New("schema_version is required")
	}
	if strings.TrimSpace(r.Workload.ID) == "" || strings.TrimSpace(r.Workload.ManifestVersion) == "" ||
		strings.TrimSpace(r.Workload.ManifestDigest) == "" || strings.TrimSpace(r.Workload.RepositoryCommit) == "" ||
		strings.TrimSpace(r.Workload.ManifestWorkflow) == "" || strings.TrimSpace(r.Workload.CommandDigest) == "" {
		return errors.New("workload evidence identity is required")
	}
	if r.Workload.Provenance != "synthetic" && r.Workload.Provenance != "real" {
		return errors.New("workload provenance must be synthetic or real")
	}
	if r.Workload.RedactionStatus != "redacted" || !r.Workload.SecretsScanned || !r.Workload.RawPayloadsExcluded {
		return errors.New("workload evidence must be redacted and secret-scanned")
	}
	if r.ExitCode < -1 {
		return errors.New("exit_code must be -1 or non-negative")
	}
	if r.Duration < 0 || r.PlatformTimes.Total() < 0 || r.WorkloadTimes.Total() < 0 {
		return errors.New("durations cannot be negative")
	}
	return nil
}
