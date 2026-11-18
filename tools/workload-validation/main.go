package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func main() {
	mode := flag.String("mode", "fake", "validation mode: fake or command")
	format := flag.String("format", "json", "report format: json or markdown")
	output := flag.String("output", "-", "report file, or - for stdout")
	id := flag.String("id", "fake-workload-0001", "stable report identifier in fake mode")
	repository := flag.String("repository", "example/repository", "repository name, or local directory in command mode")
	workflow := flag.String("workflow", "ci.yml", "workflow name or path")
	image := flag.String("image", "base-linux-x64", "runner image/profile name")
	command := flag.String("command", "", "command to run; required only in explicit command mode")
	workloadID := flag.String("workload-id", "leo-runners-ci", "reviewed workload identity")
	manifestDigest := flag.String("manifest-digest", "sha256:8fc0ae8b8dd1698e3f6d3566a7782062cb2f9272b97b68d9254f037a010a83ef", "immutable workload manifest digest")
	commit := flag.String("commit", "3d8acb1904854728572e7f45459255f7f2113858", "immutable repository commit")
	timeout := flag.Duration("timeout", 30*time.Minute, "command timeout")
	flag.Parse()

	if *format != "json" && *format != "markdown" {
		fail(errors.New("format must be json or markdown"))
	}
	var report Report
	switch *mode {
	case "fake":
		report = FakeReport(*id, *repository, *workflow, *image)
	case "command":
		if *repository == "" || *repository == "example/repository" {
			fail(errors.New("command mode requires --repository pointing to a local directory"))
		}
		absolute, err := filepath.Abs(*repository)
		if err != nil {
			fail(err)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.IsDir() {
			fail(fmt.Errorf("repository must be an existing directory: %s", *repository))
		}
		if *command == "" {
			fail(errors.New("command mode requires --command"))
		}
		report = RunCommand(context.Background(), CommandConfig{Repository: absolute, Workflow: *workflow, Command: *command, Timeout: *timeout})
		report.Image = *image
		report.Platform, report.Architecture = "linux", "unknown"
		report.Workload = defaultWorkloadEvidence("real", *workflow)
	default:
		fail(fmt.Errorf("unsupported mode %q", *mode))
	}
	report.Workload.ID, report.Workload.ManifestDigest, report.Workload.RepositoryCommit = *workloadID, *manifestDigest, *commit
	var writer io.Writer = os.Stdout
	var file *os.File
	if *output != "-" {
		var err error
		file, err = os.Create(*output)
		if err != nil {
			fail(err)
		}
		defer file.Close()
		writer = file
	}
	var err error
	if *format == "json" {
		err = WriteJSON(writer, report)
	} else {
		err = WriteMarkdown(writer, report)
	}
	if err != nil {
		fail(err)
	}
	if report.Status != "passed" {
		os.Exit(2)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "workload-validation:", err); os.Exit(1) }
