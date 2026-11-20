package aws

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const userDataTemplatePath = "../../../../bootstrap/runner/user-data.sh"

// ec2UserDataLimit is EC2's limit for raw (not base64) user data.
const ec2UserDataLimit = 16 * 1024

func readUserDataTemplate(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(userDataTemplatePath))
	if err != nil {
		t.Fatalf("read shipped user-data template: %v", err)
	}
	return string(raw)
}

func TestShippedUserDataTemplateRendersWithTheProviderRenderer(t *testing.T) {
	template := readUserDataTemplate(t)
	if got := strings.Count(template, UserDataJITPlaceholder); got != 1 {
		t.Fatalf("template has %d placeholders, want exactly 1", got)
	}
	if len(template) >= ec2UserDataLimit {
		t.Fatalf("template is %d bytes; EC2 allows %d", len(template), ec2UserDataLimit)
	}

	const jit = "opaque+encoded/jit_config=="
	encoded, err := renderUserData(template, jit)
	if err != nil {
		t.Fatalf("renderUserData() error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(decoded)
	if !strings.Contains(rendered, "JIT_CONFIG_B64='"+jit+"'") {
		t.Fatal("the JIT value was not assigned as a single-quoted shell string")
	}
	if strings.Contains(rendered, UserDataJITPlaceholder) {
		t.Fatal("the placeholder survived rendering")
	}

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available to syntax-check the rendered script")
	}
	command := exec.Command(bash, "-n")
	command.Stdin = strings.NewReader(rendered)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("rendered user data is not valid bash: %v\n%s", err, output)
	}
}

func TestShippedUserDataTemplateKeepsItsSecurityProperties(t *testing.T) {
	template := readUserDataTemplate(t)
	hash := regexp.MustCompile(`(?m)^RUNNER_SHA256_(X64|ARM64)="[0-9a-f]{64}"$`)
	if got := len(hash.FindAllString(template, -1)); got != 2 {
		t.Fatalf("found %d pinned runner checksums, want one per architecture", got)
	}
	if !regexp.MustCompile(`(?m)^RUNNER_VERSION="[0-9]+\.[0-9]+\.[0-9]+"$`).MatchString(template) {
		t.Fatal("the runner version is not pinned to an exact release")
	}
	for _, forbidden := range []string{"set -x", "set -o xtrace", "latest/download"} {
		if strings.Contains(template, forbidden) {
			t.Fatalf("template must not contain %q", forbidden)
		}
	}
	if pipeToShell := regexp.MustCompile(`\|\s*(sudo\s+)?(ba|z)?sh\b`); pipeToShell.MatchString(template) {
		t.Fatal("template must not pipe downloaded content into a shell")
	}
	if !regexp.MustCompile(`\|\s*sha256sum\b`).MatchString(template) {
		t.Fatal("expected the checksum pipeline to be present")
	}
	for _, required := range []string{"sha256sum -c", "set -euo pipefail", "umask 077", "runuser -u", "rm -f \"$2\""} {
		if !strings.Contains(template, required) {
			t.Fatalf("template must contain %q", required)
		}
	}
	// The checksum must be verified before anything is extracted.
	if strings.Index(template, "sha256sum -c") > strings.Index(template, "tar -xzf") {
		t.Fatal("the archive is extracted before its checksum is verified")
	}
}
