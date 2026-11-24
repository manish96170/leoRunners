package main

import (
	"testing"
	"time"

	runtimeconfig "github.com/leo-runners/ci-platform/controller/internal/config"
	"github.com/leo-runners/ci-platform/controller/internal/github"
)

func TestConfiguredGitHubTimeoutsReachTheClientAndVerifier(t *testing.T) {
	jit := runtimeconfig.GitHubJITConfig{
		APIBaseURL:          "https://api.github.com",
		RequestTimeout:      45 * time.Second,
		RegistrationTimeout: 40 * time.Minute,
		PollInterval:        7 * time.Second,
	}
	client := jitClientConfig(jit, github.StaticBearerToken("t"))
	if client.Timeout != 45*time.Second || client.BaseURL != "https://api.github.com" {
		t.Fatalf("client config = %+v", client)
	}
	verifier := registrationVerifierConfig(jit)
	if verifier.Timeout != 40*time.Minute || verifier.Interval != 7*time.Second {
		t.Fatalf("verifier config = %+v", verifier)
	}
}

func TestDefaultGitHubTimeoutsAreTheDocumentedOnes(t *testing.T) {
	jit := runtimeconfig.Defaults().GitHubJIT
	if got := registrationVerifierConfig(jit); got.Timeout != 10*time.Minute || got.Interval != 5*time.Second {
		t.Fatalf("default verifier config = %+v, want 10m and 5s", got)
	}
	if got := jitClientConfig(jit, github.StaticBearerToken("t")); got.Timeout != 30*time.Second {
		t.Fatalf("default request timeout = %s, want 30s", got.Timeout)
	}
}
