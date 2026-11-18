package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	requestPath := flag.String("request", "", "validated workload-intake request JSON")
	repository := flag.String("repository", "", "clean local repository checkout")
	output := flag.String("output", "", "atomic evidence report path")
	timeout := flag.Duration("timeout", defaultTimeout, "maximum command duration")
	maxOutput := flag.Int("max-output", defaultOutput, "maximum captured bytes per stream")
	networkEnabled := flag.Bool("network-enabled", false, "unsupported compatibility flag; must remain false")
	flag.Parse()
	if *requestPath == "" || *repository == "" || *output == "" {
		fail("--request, --repository, and --output are required")
	}
	if *networkEnabled {
		fail("network-enabled mode is refused")
	}
	fail("host execution is refused; invoke this runner only through an external sandbox implementation")
	repo, err := filepath.Abs(*repository)
	if err != nil {
		fail(err.Error())
	}
	report, runErr := Run(*requestPath, repo, options{Timeout: *timeout, MaxOutput: *maxOutput})
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fail(err.Error())
	}
	data = append(data, '\n')
	if err := writeAtomic(*output, data); err != nil {
		fail(err.Error())
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "controlled-workload:", runErr)
		os.Exit(1)
	}
}

func fail(message string) { fmt.Fprintln(os.Stderr, "controlled-workload:", message); os.Exit(2) }
