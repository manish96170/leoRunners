package main

import "time"

type request struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Repository struct {
			URL    string `json:"url"`
			Commit string `json:"commit"`
		} `json:"repository"`
		Workload struct {
			ID            string `json:"id"`
			Command       string `json:"command"`
			CommandDigest string `json:"commandDigest"`
			Shell         string `json:"shell"`
		} `json:"workload"`
		Provenance struct {
			Approval struct {
				ApprovedAt string `json:"approvedAt"`
				ExpiresAt  string `json:"expiresAt"`
			} `json:"approval"`
		} `json:"provenance"`
		Observations struct {
			Network struct {
				EgressMode             string `json:"egressMode"`
				ExternalAccessApproved bool   `json:"externalAccessApproved"`
			} `json:"network"`
		} `json:"observations"`
		Redaction struct {
			Status              string `json:"status"`
			SecretsScanned      bool   `json:"secretsScanned"`
			RawPayloadsExcluded bool   `json:"rawPayloadsExcluded"`
		} `json:"redaction"`
	} `json:"spec"`
}

type Evidence struct {
	SchemaVersion      string    `json:"schema_version"`
	Status             string    `json:"status"`
	RequestName        string    `json:"request_name"`
	RepositoryHash     string    `json:"repository_hash"`
	Commit             string    `json:"commit"`
	WorkloadID         string    `json:"workload_id"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	DurationMillis     int64     `json:"duration_ms"`
	ExitCode           int       `json:"exit_code"`
	TimedOut           bool      `json:"timed_out"`
	OutputTruncated    bool      `json:"output_truncated"`
	Stdout             string    `json:"stdout"`
	Stderr             string    `json:"stderr"`
	WorktreeRemoved    bool      `json:"worktree_removed"`
	NetworkMode        string    `json:"network_mode"`
	CommandAllowlisted bool      `json:"command_allowlisted"`
	Redaction          Redaction `json:"redaction"`
}

type Redaction struct {
	Status            string `json:"status"`
	SecretsScanned    bool   `json:"secrets_scanned"`
	RawOutputExcluded bool   `json:"raw_output_excluded"`
}
