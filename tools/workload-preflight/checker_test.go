package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	if CompareVersions("tool 1.2.3", "1.2.0") <= 0 {
		t.Fatal("expected newer version")
	}
	if CompareVersions("v1.2", "1.2.0") != 0 {
		t.Fatal("expected equivalent version")
	}
	if CompareVersions("1.9", "2.0") >= 0 {
		t.Fatal("expected older version")
	}
}

func TestRunFixtureProducesStructuredChecks(t *testing.T) {
	root := t.TempDir()
	manifest := Manifest{Name: "fixture", Architecture: "not-host", MinDiskBytes: 1, MinMemoryBytes: 1, Commands: []Requirement{{Name: "go", MinVersion: "1.0"}}, Browsers: []Requirement{{Name: "definitely-missing-browser", Required: boolPtr(false)}}}
	result := Run(manifest, root)
	if result.Summary.Fail == 0 {
		t.Fatal("expected architecture failure")
	}
	if result.Summary.Warn == 0 {
		t.Fatal("expected optional browser warning")
	}
	if len(result.Checks) != 5 {
		t.Fatalf("got %d checks, want 5", len(result.Checks))
	}
	for _, check := range result.Checks {
		if check.ID == "browser.definitely-missing-browser" && check.Status != Warn {
			t.Fatalf("browser status = %s", check.Status)
		}
	}
}

func TestLoadManifestRejectsDuplicateNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"name":"x","architecture":"amd64","commands":[{"name":"go"},{"name":"go"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func boolPtr(value bool) *bool { return &value }
