package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	manifestPath := flag.String("manifest", "", "path to a JSON workload manifest")
	root := flag.String("root", ".", "filesystem root used for disk checks and command working directory")
	flag.Parse()
	if *manifestPath == "" {
		fail(errors.New("--manifest is required"))
	}
	manifest, err := LoadManifest(*manifestPath)
	if err != nil {
		fail(err)
	}
	result := Run(manifest, *root)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fail(err)
	}
	if result.Summary.Fail > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "workload-preflight:", err)
	os.Exit(2)
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %q: %w", path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %q: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}
	return manifest, nil
}

func resolveRoot(root string) (string, error) {
	if root == "" {
		return "", errors.New("root must not be empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("stat root %q: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root %q is not a directory", absolute)
	}
	return absolute, nil
}
