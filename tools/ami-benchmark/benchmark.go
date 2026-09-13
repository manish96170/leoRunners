package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Checkpoint string

const (
	LaunchRequested   Checkpoint = "launch_requested"
	InstanceRunning   Checkpoint = "instance_running"
	BootstrapStarted  Checkpoint = "bootstrap_started"
	BootstrapComplete Checkpoint = "bootstrap_completed"
	RunnerRegistered  Checkpoint = "runner_registered"
	RunnerReady       Checkpoint = "runner_ready"
	JobStarted        Checkpoint = "job_started"
)

var checkpoints = []Checkpoint{LaunchRequested, InstanceRunning, BootstrapStarted, BootstrapComplete, RunnerRegistered, RunnerReady, JobStarted}

type Sample struct {
	ID          string               `json:"sample_id"`
	Checkpoints map[Checkpoint]int64 `json:"checkpoints_ms"`
	Durations   map[string]int64     `json:"durations_ms"`
}

type Provenance struct {
	ImageID     string `json:"image_id"`
	ImageDigest string `json:"image_digest"`
	Profile     string `json:"profile_version"`
	SourceAMI   string `json:"source_ami"`
	Manifest    string `json:"manifest"`
	Contract    string `json:"contract"`
	Redacted    bool   `json:"redacted"`
	SecretFree  bool   `json:"secret_free"`
}

type Thresholds struct {
	MaxTotalStartupMS    int64   `json:"max_total_startup_ms"`
	MaxP95TotalStartupMS int64   `json:"max_p95_total_startup_ms"`
	MaxP99TotalStartupMS int64   `json:"max_p99_total_startup_ms"`
	MaxRegressionPercent float64 `json:"max_regression_percent"`
}

type Percentiles struct {
	P50 int64 `json:"p50_ms"`
	P95 int64 `json:"p95_ms"`
	P99 int64 `json:"p99_ms"`
}

type Report struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Mode       string                 `json:"mode"`
	Status     string                 `json:"status"`
	Generated  string                 `json:"generated_at"`
	Samples    []Sample               `json:"samples"`
	Summary    map[string]Percentiles `json:"summary"`
	Timeouts   map[string]int64       `json:"timeouts"`
	Regression map[string]any         `json:"regression,omitempty"`
	Gates      map[string]bool        `json:"gates"`
	Thresholds Thresholds             `json:"thresholds"`
	Provenance Provenance             `json:"provenance"`
	Redaction  map[string]any         `json:"redaction"`
}

type FakeTimeline struct {
	ID        string
	Durations map[Checkpoint]int64
}

func DefaultDurations() map[Checkpoint]int64 {
	return map[Checkpoint]int64{
		LaunchRequested: 0, InstanceRunning: 4200, BootstrapStarted: 5200,
		BootstrapComplete: 12800, RunnerRegistered: 15700, RunnerReady: 20500, JobStarted: 22600,
	}
}

func (f FakeTimeline) Replay() (Sample, error) {
	if strings.TrimSpace(f.ID) == "" {
		return Sample{}, errors.New("sample id is required")
	}
	result := Sample{ID: f.ID, Checkpoints: map[Checkpoint]int64{}, Durations: map[string]int64{}}
	previous := int64(0)
	for _, checkpoint := range checkpoints {
		at, ok := f.Durations[checkpoint]
		if !ok || at < previous {
			return Sample{}, fmt.Errorf("invalid checkpoint %q", checkpoint)
		}
		result.Checkpoints[checkpoint] = at
		previous = at
	}
	result.Durations["launch_to_running"] = result.Checkpoints[InstanceRunning] - result.Checkpoints[LaunchRequested]
	result.Durations["running_to_bootstrap"] = result.Checkpoints[BootstrapStarted] - result.Checkpoints[InstanceRunning]
	result.Durations["bootstrap_to_complete"] = result.Checkpoints[BootstrapComplete] - result.Checkpoints[BootstrapStarted]
	result.Durations["bootstrap_to_registered"] = result.Checkpoints[RunnerRegistered] - result.Checkpoints[BootstrapStarted]
	result.Durations["registered_to_ready"] = result.Checkpoints[RunnerReady] - result.Checkpoints[RunnerRegistered]
	result.Durations["ready_to_job"] = result.Checkpoints[JobStarted] - result.Checkpoints[RunnerReady]
	result.Durations["total_startup"] = result.Checkpoints[JobStarted] - result.Checkpoints[LaunchRequested]
	return result, nil
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	// Nearest-rank is deterministic and avoids interpolating across samples.
	rank := int(math.Ceil(p * float64(len(ordered))))
	if rank < 1 {
		rank = 1
	}
	return ordered[rank-1]
}

func summarize(samples []Sample) map[string]Percentiles {
	values := map[string][]int64{}
	for _, sample := range samples {
		for name, value := range sample.Durations {
			values[name] = append(values[name], value)
		}
	}
	result := map[string]Percentiles{}
	for name, list := range values {
		result[name] = Percentiles{P50: percentile(list, .50), P95: percentile(list, .95), P99: percentile(list, .99)}
	}
	return result
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func loadProvenance(contractPath, manifestPath string) (Provenance, error) {
	var contract struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
		Spec struct {
			Source struct {
				AMIID string `json:"ami_id"`
			} `json:"source"`
			Artifact struct {
				Version string `json:"version"`
				Digest  string `json:"digest"`
			} `json:"artifact"`
		} `json:"spec"`
	}
	var manifest struct {
		Artifact struct {
			ImageID        string `json:"image_id"`
			Digest         string `json:"digest"`
			SourceAMI      string `json:"source_ami"`
			ProfileVersion string `json:"profile_version"`
		} `json:"artifact"`
	}
	if err := readJSON(contractPath, &contract); err != nil {
		return Provenance{}, err
	}
	if err := readJSON(manifestPath, &manifest); err != nil {
		return Provenance{}, err
	}
	if contract.Spec.Artifact.Digest == "" || manifest.Artifact.Digest == "" || contract.Spec.Artifact.Digest != manifest.Artifact.Digest {
		return Provenance{}, errors.New("AMI digest linkage mismatch")
	}
	if contract.Spec.Artifact.Version != "" && contract.Spec.Artifact.Version != contract.Metadata.Version {
		return Provenance{}, errors.New("AMI contract artifact version mismatch")
	}
	if manifest.Artifact.ProfileVersion != contract.Metadata.Version || manifest.Artifact.SourceAMI != contract.Spec.Source.AMIID {
		return Provenance{}, errors.New("AMI manifest provenance mismatch")
	}
	if !strings.HasPrefix(manifest.Artifact.Digest, "sha256:") {
		return Provenance{}, errors.New("AMI digest must be sha256 content address")
	}
	return Provenance{ImageID: manifest.Artifact.ImageID, ImageDigest: manifest.Artifact.Digest, Profile: manifest.Artifact.ProfileVersion, SourceAMI: manifest.Artifact.SourceAMI, Manifest: filepath.Base(manifestPath), Contract: filepath.Base(contractPath), Redacted: true, SecretFree: true}, nil
}

func reportHash(report Report) string {
	data, _ := json.Marshal(report)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func checkGates(report *Report, baseline *Report) {
	total := report.Summary["total_startup"]
	report.Gates = map[string]bool{
		"checkpoint_order":  len(report.Samples) > 0,
		"timeout":           total.P99 <= report.Thresholds.MaxTotalStartupMS,
		"p95_timeout":       total.P95 <= report.Thresholds.MaxP95TotalStartupMS,
		"p99_timeout":       total.P99 <= report.Thresholds.MaxP99TotalStartupMS,
		"provenance_linked": report.Provenance.ImageDigest != "" && report.Provenance.Redacted && report.Provenance.SecretFree,
	}
	if baseline != nil {
		base := baseline.Summary["total_startup"].P95
		current := total.P95
		change := float64(0)
		if base > 0 {
			change = (float64(current-base) / float64(base)) * 100
		}
		report.Regression = map[string]any{"baseline_p95_ms": base, "current_p95_ms": current, "change_percent": change, "allowed_percent": report.Thresholds.MaxRegressionPercent}
		report.Gates["regression"] = change <= report.Thresholds.MaxRegressionPercent
	}
	for _, passed := range report.Gates {
		if !passed {
			report.Status = "FAIL"
			return
		}
	}
	report.Status = "PASS"
}

func writeReport(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
