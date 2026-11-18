package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	repository := flag.String("repository", "", "local repository checkout to inspect")
	commit := flag.String("commit", "", "full 40-character commit that must equal HEAD")
	output := flag.String("output", "-", "JSON report path, or - for stdout")
	maxFiles := flag.Int("max-files", defaultMaxFiles, "maximum allowlisted files to inspect")
	maxBytes := flag.Int64("max-bytes", defaultMaxBytes, "maximum total bytes to inspect")
	flag.Parse()

	if strings.TrimSpace(*repository) == "" {
		fail(errors.New("--repository is required"))
	}
	if err := validateCommit(*commit); err != nil {
		fail(err)
	}
	if *maxFiles <= 0 || *maxFiles > 128 || *maxBytes <= 0 || *maxBytes > 8<<20 {
		fail(errors.New("bounds must be max-files 1..128 and max-bytes 1..8388608"))
	}
	repo, err := filepath.Abs(*repository)
	if err != nil {
		fail(err)
	}
	report, err := Derive(repo, *commit, Limits{MaxFiles: *maxFiles, MaxBytes: *maxBytes})
	if err != nil {
		fail(err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fail(err)
	}
	data = append(data, '\n')
	if *output == "-" {
		_, err = os.Stdout.Write(data)
	} else {
		err = writeAtomic(*output, data)
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "workload-intake:", err)
	os.Exit(1)
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".workload-intake-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validateCommit(commit string) error {
	if !fullCommit.MatchString(commit) {
		return errors.New("--commit must be a lowercase full 40-character commit")
	}
	return nil
}
