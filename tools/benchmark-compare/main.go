package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("input path cannot be empty")
	}
	*s = append(*s, value)
	return nil
}

func main() {
	var baseline, candidate stringList
	flag.Var(&baseline, "baseline", "baseline JSON report path; repeat for multiple files")
	flag.Var(&candidate, "candidate", "candidate JSON report path; repeat for multiple files")
	format := flag.String("format", "json", "output format: json or markdown")
	output := flag.String("output", "-", "output path, or - for stdout")
	thresholdPercent := flag.Float64("threshold-percent", 10, "maximum allowed median or p95 regression percentage")
	thresholdMS := flag.Float64("threshold-ms", 0, "minimum absolute regression in milliseconds")
	evidence := flag.String("evidence", "", "override input evidence: synthetic or real")
	flag.Parse()
	if len(baseline) == 0 || len(candidate) == 0 {
		fail(errors.New("at least one --baseline and one --candidate are required"))
	}
	if *format != "json" && *format != "markdown" {
		fail(errors.New("format must be json or markdown"))
	}
	var override Evidence
	if *evidence != "" {
		override = Evidence(*evidence)
		if err := validateEvidence(override); err != nil {
			fail(err)
		}
	}
	base, baseEvidence, err := readAll(baseline, override)
	if err != nil {
		fail(err)
	}
	current, currentEvidence, err := readAll(candidate, override)
	if err != nil {
		fail(err)
	}
	result, err := compare(base, current, baseEvidence, currentEvidence, *thresholdPercent, *thresholdMS, baseline, candidate)
	if err != nil {
		fail(err)
	}
	var writer io.Writer = os.Stdout
	var file *os.File
	if *output != "-" {
		file, err = os.Create(*output)
		if err != nil {
			fail(err)
		}
		defer file.Close()
		writer = file
	}
	if *format == "json" {
		err = writeJSON(writer, result)
	} else {
		err = writeMarkdown(writer, result)
	}
	if err != nil {
		fail(err)
	}
	if !result.Passed {
		os.Exit(2)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "benchmark-compare:", err); os.Exit(1) }
