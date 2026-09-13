package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func WriteJSON(w io.Writer, report Report) error {
	if err := report.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteMarkdown(w io.Writer, report Report) error {
	if err := report.Validate(); err != nil {
		return err
	}
	log := report.Log
	if log == "" {
		log = "(no command output)"
	}
	_, err := fmt.Fprintf(w, "# Workload Validation\n\n| Field | Value |\n|---|---|\n| Schema | `%s` |\n| ID | `%s` |\n| Mode | `%s` |\n| Repository | `%s` |\n| Workflow | `%s` |\n| Image | `%s` |\n| Platform | `%s/%s` |\n| Status | `%s` |\n| Exit code | `%d` |\n| Duration | `%s` |\n\n## Platform Timings\n\n| Stage | Duration |\n|---|---:|\n| Queue | %s |\n| Launch | %s |\n| Boot | %s |\n| Registration | %s |\n| Ready | %s |\n| Job start | %s |\n| Cleanup | %s |\n| **Platform total** | **%s** |\n\n## Workload Timings\n\n| Stage | Duration |\n|---|---:|\n| Dependency install | %s |\n| Build | %s |\n| Tests | %s |\n| Cypress | %s |\n| Docker | %s |\n| **Workload total** | **%s** |\n\n## Redacted Log\n\n```text\n%s\n```\n", report.SchemaVersion, report.ID, report.Mode, escapeCell(report.Repository), escapeCell(report.Workflow), escapeCell(report.Image), report.Platform, report.Architecture, report.Status, report.ExitCode, report.Duration, report.PlatformTimes.Queue, report.PlatformTimes.Launch, report.PlatformTimes.Boot, report.PlatformTimes.Registration, report.PlatformTimes.Ready, report.PlatformTimes.JobStart, report.PlatformTimes.Cleanup, report.PlatformTimes.Total(), report.WorkloadTimes.DependencyInstall, report.WorkloadTimes.Build, report.WorkloadTimes.Tests, report.WorkloadTimes.Cypress, report.WorkloadTimes.Docker, report.WorkloadTimes.Total(), log)
	return err
}

func escapeCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}
