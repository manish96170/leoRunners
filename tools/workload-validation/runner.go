package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type CommandConfig struct {
	Repository string
	Workflow   string
	Command    string
	Timeout    time.Duration
}

func RunCommand(ctx context.Context, cfg CommandConfig) Report {
	started := time.Now().UTC()
	report := baseReport("command", cfg.Repository, cfg.Workflow)
	report.StartedAt = started
	report.Command = RedactSecrets(cfg.Command)
	if strings.TrimSpace(cfg.Command) == "" {
		report.Status, report.ExitCode, report.Error = "invalid", -1, "command is required"
		report.FinishedAt = time.Now().UTC()
		report.Duration = report.FinishedAt.Sub(started)
		return report
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Minute
	}
	commandCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, "sh", "-c", cfg.Command)
	command.Dir = cfg.Repository
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	report.FinishedAt = time.Now().UTC()
	report.Duration = report.FinishedAt.Sub(started)
	report.Log = RedactSecrets(output.String())
	report.ExitCode = 0
	report.Status = "passed"
	if err != nil {
		report.Status = "failed"
		report.ExitCode = exitCode(err)
		report.Error = RedactSecrets(err.Error())
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			report.Status, report.TimedOut, report.ExitCode = "timed_out", true, -1
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			report.Status, report.ExitCode = "cancelled", -1
		}
	}
	return report
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func baseReport(mode, repository, workflow string) Report {
	return Report{SchemaVersion: "1", ID: fmt.Sprintf("workload-%d", time.Now().UnixNano()), Mode: mode, Repository: repository, Workflow: workflow, Image: "unknown", Platform: "unknown", Architecture: "unknown", ExitCode: -1}
}
