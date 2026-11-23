package controller

import (
	"context"
	"fmt"
	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/runners"
	"strings"
)

// GitHubJITAdapter adapts the owner/repository-neutral controller contract to
// the GitHub REST client's owner and repository path parameters. With
// Organization set it registers the runner at organization level, in the
// configured organization runner group, instead of on the repository.
type GitHubJITAdapter struct {
	Client       *github.Client
	Organization bool
}

func (a GitHubJITAdapter) GenerateJITConfig(ctx context.Context, request runners.JITRequest) (runners.JITConfig, error) {
	if a.Client == nil {
		return runners.JITConfig{}, fmt.Errorf("GitHub JIT client is nil")
	}
	parts := strings.SplitN(strings.TrimSpace(request.Repository), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return runners.JITConfig{}, fmt.Errorf("invalid GitHub repository %q", request.Repository)
	}
	body := github.JITConfigRequest{Name: request.Name, Labels: request.Labels, RunnerGroupID: request.RunnerGroupID, WorkFolder: "_work"}
	var response github.JITConfigResponse
	var err error
	if a.Organization {
		response, err = a.Client.GenerateOrganizationJITConfig(ctx, parts[0], body)
	} else {
		response, err = a.Client.GenerateJITConfig(ctx, parts[0], parts[1], body)
	}
	if err != nil {
		return runners.JITConfig{}, err
	}
	return runners.JITConfig{EncodedConfig: response.EncodedJITConfig, RunnerID: response.Runner.ID, RunnerName: response.Runner.Name}, nil
}

var _ runners.JITClient = GitHubJITAdapter{}

// GitHubRegistrationAdapter adapts GitHub's repository runner poller to the
// provider-neutral registration verifier used by the assignment service.
type GitHubRegistrationAdapter struct {
	Verifier     *github.RegistrationVerifier
	Organization bool
}

func (a GitHubRegistrationAdapter) WaitForRegistration(ctx context.Context, request runners.RegistrationRequest) (runners.GitHubRunner, error) {
	if a.Verifier == nil {
		return runners.GitHubRunner{}, fmt.Errorf("GitHub registration verifier is nil")
	}
	parts := strings.SplitN(strings.TrimSpace(request.Repository), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return runners.GitHubRunner{}, fmt.Errorf("invalid GitHub repository %q", request.Repository)
	}
	var result github.JITRunner
	var err error
	if a.Organization {
		result, err = a.Verifier.WaitForOrganizationRegistration(ctx, parts[0], request.RunnerID, request.RunnerName)
	} else {
		result, err = a.Verifier.WaitForRegistration(ctx, parts[0], parts[1], request.RunnerID, request.RunnerName)
	}
	if err != nil {
		return runners.GitHubRunner{}, err
	}
	return runners.GitHubRunner{ID: result.ID, Name: result.Name}, nil
}

var _ runners.RegistrationVerifier = GitHubRegistrationAdapter{}
