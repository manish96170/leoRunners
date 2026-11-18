package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	mode := flag.String("mode", "fake", "benchmark mode: fake or real-ec2")
	count := flag.Int("count", 20, "number of deterministic fake samples")
	output := flag.String("output", "-", "report path, or - for stdout")
	contract := flag.String("contract", "ami/image-contract.v1.json", "AMI contract path")
	manifest := flag.String("manifest", "ami/manifest.example.v1.json", "AMI manifest path")
	baselinePath := flag.String("baseline", "", "optional prior report for regression comparison")
	maxTotal := flag.Int64("max-total-startup-ms", 30000, "maximum p99 total startup time")
	maxP95 := flag.Int64("max-p95-startup-ms", 26000, "maximum p95 total startup time")
	maxP99 := flag.Int64("max-p99-startup-ms", 30000, "maximum p99 total startup time")
	maxRegression := flag.Float64("max-regression-percent", 15, "maximum allowed p95 regression")
	allowReal := flag.Bool("allow-real-ec2", false, "acknowledge that real mode may create ephemeral EC2 resources")
	confirm := flag.String("confirm", "", "exact real-mode confirmation token")
	flag.Parse()

	if *mode == "real-ec2" {
		fail(errors.New("real-ec2 procedure is intentionally not executable by this offline benchmark; use tools/ami-benchmark/real-ec2.sh after reviewing the runbook"))
	}
	if *mode != "fake" {
		fail(fmt.Errorf("unsupported mode %q", *mode))
	}
	if *allowReal || *confirm != "" {
		fail(errors.New("real EC2 flags are unavailable in fake mode"))
	}
	if *count < 1 || *count > 10000 {
		fail(errors.New("count must be between 1 and 10000"))
	}
	if *maxTotal <= 0 || *maxP95 <= 0 || *maxP99 <= 0 || *maxRegression < 0 {
		fail(errors.New("thresholds must be positive and regression must be non-negative"))
	}

	provenance, err := loadProvenance(*contract, *manifest)
	if err != nil {
		fail(err)
	}
	samples := make([]Sample, 0, *count)
	for i := 0; i < *count; i++ {
		durations := DefaultDurations()
		// Stable variation exercises percentile gates without using wall-clock time.
		variation := int64(i % 5)
		durations[InstanceRunning] += variation * 100
		durations[BootstrapStarted] += variation * 100
		durations[BootstrapComplete] += variation * 150
		durations[RunnerRegistered] += variation * 200
		durations[RunnerReady] += variation * 250
		durations[JobStarted] += variation * 300
		sample, err := (FakeTimeline{ID: fmt.Sprintf("fake-%04d", i+1), Durations: durations}).Replay()
		if err != nil {
			fail(err)
		}
		samples = append(samples, sample)
	}
	report := Report{APIVersion: "ami.leorunners.io/v1", Kind: "AMIStartupBenchmarkReport", Mode: "fake", Generated: nowUTC(), Samples: samples, Summary: summarize(samples), Thresholds: Thresholds{MaxTotalStartupMS: *maxTotal, MaxP95TotalStartupMS: *maxP95, MaxP99TotalStartupMS: *maxP99, MaxRegressionPercent: *maxRegression}, Provenance: provenance, Redaction: map[string]any{"applied": true, "removed_fields": []string{"aws_account_id", "instance_id", "private_ip", "public_ip", "user_data", "credentials", "jit_config"}}}
	var baseline *Report
	if strings.TrimSpace(*baselinePath) != "" {
		baseline = &Report{}
		if err := readJSON(*baselinePath, baseline); err != nil {
			fail(err)
		}
	}
	checkGates(&report, baseline)
	if report.Status == "" {
		fail(errors.New("benchmark report did not produce a status"))
	}
	if *output == "-" {
		if err := writeReport(os.Stdout, report); err != nil {
			fail(err)
		}
	} else {
		tmp := *output + fmt.Sprintf(".tmp.%d", os.Getpid())
		file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fail(err)
		}
		err = writeReport(file, report)
		closeErr := file.Close()
		if err != nil {
			fail(err)
		}
		if closeErr != nil {
			fail(closeErr)
		}
		if err := os.Rename(tmp, *output); err != nil {
			fail(err)
		}
	}
	if report.Status != "PASS" {
		os.Exit(1)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "ami-benchmark:", err); os.Exit(2) }
