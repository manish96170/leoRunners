package main

import "time"

const (
	defaultMaxFiles = 32
	defaultMaxBytes = 1 << 20
)

type Limits struct {
	MaxFiles int
	MaxBytes int64
}

type Report struct {
	SchemaVersion string     `json:"schema_version"`
	Status        string     `json:"status"`
	Repository    string     `json:"repository"`
	Commit        string     `json:"commit"`
	GeneratedAt   time.Time  `json:"generated_at"`
	Execution     Execution  `json:"execution"`
	Manifest      Manifest   `json:"manifest"`
	Evidence      []Evidence `json:"evidence"`
	Summary       Summary    `json:"summary"`
	Redaction     Redaction  `json:"redaction"`
}

type Execution struct {
	DirtyCheckoutRejected bool `json:"dirty_checkout_rejected"`
	WorkflowCommandsRun   bool `json:"workflow_commands_run"`
	AllowlistedFilesOnly  bool `json:"allowlisted_files_only"`
	SecretScanPassed      bool `json:"secret_scan_passed"`
}

type Manifest struct {
	APIVersion   string       `json:"api_version"`
	Kind         string       `json:"kind"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Workload     Workload     `json:"workload"`
	Runner       Runner       `json:"runner"`
	Capabilities []Capability `json:"capabilities"`
}

type Workload struct {
	ID         string `json:"id"`
	Workflow   string `json:"workflow"`
	Command    string `json:"command"`
	Provenance string `json:"provenance"`
}

type Runner struct {
	OperatingSystem string `json:"operating_system"`
	Architecture    string `json:"architecture"`
}

type Capability struct {
	ID       string   `json:"id"`
	Enabled  bool     `json:"enabled"`
	Required bool     `json:"required"`
	Evidence Evidence `json:"evidence"`
}

type Evidence struct {
	Source  string `json:"source"`
	Locator string `json:"locator"`
	Status  string `json:"status"`
}

type Summary struct {
	FilesRead int   `json:"files_read"`
	BytesRead int64 `json:"bytes_read"`
}

type Redaction struct {
	Status             string `json:"status"`
	SecretsScanned     bool   `json:"secrets_scanned"`
	RawContentExcluded bool   `json:"raw_content_excluded"`
}
