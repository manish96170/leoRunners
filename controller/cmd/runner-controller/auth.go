package main

import (
	"context"
	"fmt"
	"log"
	"time"

	runtimeconfig "github.com/leo-runners/ci-platform/controller/internal/config"
	"github.com/leo-runners/ci-platform/controller/internal/github"
)

// newGitHubTokenSource chooses how the controller authenticates to GitHub.
// A GitHub App mints short-lived installation tokens itself; otherwise the
// operator supplies an externally rotated installation token. The two are
// mutually exclusive, which configuration validation already guarantees.
func newGitHubTokenSource(jit runtimeconfig.GitHubJITConfig) (github.BearerTokenSource, error) {
	if !jit.AppConfigured() {
		return github.StaticBearerToken(jit.InstallationToken), nil
	}
	source, err := github.NewAppTokenSource(github.AppTokenSourceConfig{
		AppID:          jit.AppID,
		InstallationID: jit.AppInstallationID,
		PrivateKeyPEM:  []byte(jit.AppPrivateKey),
		BaseURL:        jit.APIBaseURL,
		Permissions:    jit.AppTokenPermissions,
		Timeout:        jit.RequestTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure GitHub App authentication: %w", err)
	}
	return source, nil
}

// warmGitHubTokenSource requests one token in the background so a wrong App ID,
// installation ID, or key shows up in the startup log instead of at the first
// job. It never blocks startup and never logs a token.
func warmGitHubTokenSource(source github.BearerTokenSource, jit runtimeconfig.GitHubJITConfig) {
	if !jit.AppConfigured() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := source.BearerToken(ctx); err != nil {
			log.Printf("GitHub App authentication check failed (the controller keeps running and will retry per job): %v", err)
			return
		}
		log.Printf("GitHub App authentication ok: app=%s installation=%d token_permissions=%q", jit.AppID, jit.AppInstallationID, github.PermissionsString(jit.AppTokenPermissions))
	}()
}
