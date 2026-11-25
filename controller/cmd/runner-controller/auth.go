package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	runtimeconfig "github.com/leo-runners/ci-platform/controller/internal/config"
	"github.com/leo-runners/ci-platform/controller/internal/github"
)

// newGitHubTokenSource chooses how the controller authenticates to GitHub.
// A GitHub App mints short-lived installation tokens itself; otherwise the
// operator supplies an externally rotated installation token. The two are
// mutually exclusive, which configuration validation already guarantees.
func newGitHubTokenSource(jit runtimeconfig.GitHubJITConfig, repositories []string) (github.BearerTokenSource, error) {
	if !jit.AppConfigured() {
		return github.StaticBearerToken(jit.InstallationToken), nil
	}
	source, err := github.NewAppTokenSource(github.AppTokenSourceConfig{
		AppID:          jit.AppID,
		InstallationID: jit.AppInstallationID,
		PrivateKeyPEM:  []byte(jit.AppPrivateKey),
		BaseURL:        jit.APIBaseURL,
		Permissions:    jit.AppTokenPermissions,
		Repositories:   repositories,
		Timeout:        jit.RequestTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure GitHub App authentication: %w", err)
	}
	return source, nil
}

// tokenRepositories decides which repositories minted installation tokens are
// limited to, and says why, for the startup log.
//
// An App's permission applies to every repository it is installed on, but the
// controller only ever acts on the repositories it allows. Limiting each token to
// those means a defect or a stolen token cannot act on the others. The setting
// wins when given. Otherwise the limit is taken from the repository allowlist,
// unless an organization is allowed too (its repositories are not known up
// front) or the repositories belong to different owners (an installation
// belongs to one account).
func tokenRepositories(explicit, allowedOrgs, allowedRepos []string) ([]string, string) {
	if len(explicit) > 0 {
		return explicit, "GITHUB_APP_TOKEN_REPOSITORIES"
	}
	if len(allowedOrgs) > 0 {
		return nil, "not restricted: whole organizations are allowed"
	}
	if len(allowedRepos) == 0 {
		return nil, "not restricted: no repository allowlist"
	}
	names, err := runtimeconfig.ParseTokenRepositories(strings.Join(allowedRepos, ","))
	if err != nil {
		return nil, "not restricted: " + err.Error()
	}
	return names, "derived from GITHUB_ALLOWED_REPOSITORIES"
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
