package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type benchmarkSample struct {
	ID        string                   `json:"id"`
	Durations map[string]time.Duration `json:"durations"`
}

type workloadReport struct {
	ID              string                   `json:"id"`
	Mode            string                   `json:"mode"`
	Duration        time.Duration            `json:"duration"`
	PlatformTimings map[string]time.Duration `json:"platform_timings"`
	WorkloadTimings map[string]time.Duration `json:"workload_timings"`
}

type inputWrapper struct {
	Evidence Evidence        `json:"evidence"`
	Samples  json.RawMessage `json:"samples"`
	Reports  json.RawMessage `json:"reports"`
}

func readObservations(path string, override Evidence) ([]Observation, Evidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, "", fmt.Errorf("%s is empty", path)
	}
	evidence := override
	var wrapper inputWrapper
	if data[0] == '{' && json.Unmarshal(data, &wrapper) == nil && (len(wrapper.Samples) > 0 || len(wrapper.Reports) > 0) {
		if evidence == "" {
			evidence = wrapper.Evidence
		}
		if len(wrapper.Samples) > 0 {
			data = wrapper.Samples
		} else {
			data = wrapper.Reports
		}
	}
	var observations []Observation
	if data[0] == '[' {
		var samples []benchmarkSample
		if err := json.Unmarshal(data, &samples); err != nil {
			return nil, "", fmt.Errorf("parse benchmark report %s: %w", path, err)
		}
		if len(samples) == 0 {
			return nil, "", fmt.Errorf("%s contains no benchmark samples", path)
		}
		if evidence == "" {
			evidence = inferBenchmarkEvidence(samples)
		}
		if err := validateEvidence(evidence); err != nil {
			return nil, "", err
		}
		for _, sample := range samples {
			if strings.TrimSpace(sample.ID) == "" {
				return nil, "", fmt.Errorf("%s contains a sample without id", path)
			}
			for metric, duration := range sample.Durations {
				if duration < 0 {
					return nil, "", fmt.Errorf("%s sample %s has negative %s", path, sample.ID, metric)
				}
				observations = append(observations, Observation{Metric: metric, Duration: duration, Evidence: evidence, Source: path, SampleID: sample.ID})
			}
			total := time.Duration(0)
			for _, duration := range sample.Durations {
				total += duration
			}
			observations = append(observations, Observation{Metric: "total", Duration: total, Evidence: evidence, Source: path, SampleID: sample.ID})
		}
		return observations, evidence, nil
	}
	var report workloadReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, "", fmt.Errorf("parse workload report %s: %w", path, err)
	}
	if strings.TrimSpace(report.ID) == "" {
		return nil, "", fmt.Errorf("%s workload report has no id", path)
	}
	if evidence == "" {
		evidence = inferWorkloadEvidence(report.Mode)
	}
	if err := validateEvidence(evidence); err != nil {
		return nil, "", err
	}
	if report.Duration < 0 {
		return nil, "", fmt.Errorf("%s has negative duration", path)
	}
	for metric, duration := range report.PlatformTimings {
		if err := addWorkloadObservation(&observations, path, report.ID, "platform."+metric, duration, evidence); err != nil {
			return nil, "", err
		}
	}
	for metric, duration := range report.WorkloadTimings {
		if err := addWorkloadObservation(&observations, path, report.ID, "workload."+metric, duration, evidence); err != nil {
			return nil, "", err
		}
	}
	observations = append(observations, Observation{Metric: "total", Duration: report.Duration, Evidence: evidence, Source: path, SampleID: report.ID})
	return observations, evidence, nil
}

func addWorkloadObservation(out *[]Observation, path, id, metric string, duration time.Duration, evidence Evidence) error {
	if duration < 0 {
		return fmt.Errorf("%s report %s has negative %s", path, id, metric)
	}
	*out = append(*out, Observation{Metric: metric, Duration: duration, Evidence: evidence, Source: path, SampleID: id})
	return nil
}

func inferBenchmarkEvidence(samples []benchmarkSample) Evidence {
	for _, sample := range samples {
		if !strings.HasPrefix(strings.ToLower(sample.ID), "fake-") {
			return EvidenceReal
		}
	}
	return EvidenceSynthetic
}

func inferWorkloadEvidence(mode string) Evidence {
	if strings.EqualFold(mode, "fake") || mode == "" {
		return EvidenceSynthetic
	}
	return EvidenceReal
}

func validateEvidence(evidence Evidence) error {
	if evidence != EvidenceSynthetic && evidence != EvidenceReal {
		return fmt.Errorf("evidence must be %q or %q", EvidenceSynthetic, EvidenceReal)
	}
	return nil
}

func readAll(paths []string, override Evidence) ([]Observation, Evidence, error) {
	if len(paths) == 0 {
		return nil, "", fmt.Errorf("at least one input file is required")
	}
	var all []Observation
	var evidence Evidence
	for _, path := range paths {
		observations, fileEvidence, err := readObservations(path, override)
		if err != nil {
			return nil, "", err
		}
		if evidence == "" {
			evidence = fileEvidence
		} else if evidence != fileEvidence {
			return nil, "", fmt.Errorf("mixed evidence labels in inputs (%s and %s); use --evidence to override", evidence, fileEvidence)
		}
		all = append(all, observations...)
	}
	if len(all) == 0 {
		return nil, "", fmt.Errorf("inputs contain no observations")
	}
	return all, evidence, nil
}
