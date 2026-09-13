package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRegistrationTimeout  = 10 * time.Minute
	defaultRegistrationInterval = 2 * time.Second
	githubRunnerPageSize        = 100
)

// RunnerList is the repository-scoped response returned by GitHub's list
// self-hosted runners endpoint.
type RunnerList struct {
	TotalCount int         `json:"total_count"`
	Runners    []JITRunner `json:"runners"`
}

// ListRunners returns all self-hosted runners visible in a repository. It
// follows GitHub pagination so callers do not need to reason about page size.
func (c *Client) ListRunners(ctx context.Context, owner, repo string) ([]JITRunner, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateRepository(owner, repo); err != nil {
		return nil, err
	}

	token, err := c.tokenSource.BearerToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get GitHub bearer token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("GitHub bearer token is empty")
	}

	var runners []JITRunner
	for page := 1; ; page++ {
		endpoint, err := c.runnersURL(owner, repo, page)
		if err != nil {
			return nil, err
		}
		var result RunnerList
		if err := c.getJSON(ctx, endpoint, token, &result); err != nil {
			return nil, fmt.Errorf("list GitHub self-hosted runners: %w", err)
		}
		runners = append(runners, result.Runners...)
		if len(result.Runners) < githubRunnerPageSize {
			return runners, nil
		}
	}
}

// RegistrationVerifierConfig bounds polling for a runner becoming online.
// Zero values select conservative defaults.
type RegistrationVerifierConfig struct {
	Timeout  time.Duration
	Interval time.Duration
}

// RegistrationWaiter is the provider-neutral boundary for callers that need
// to wait for a particular GitHub runner. Implementations must honor ctx.
type RegistrationWaiter interface {
	WaitForRegistration(context.Context, string, string, int64, string) (JITRunner, error)
}

// RegistrationVerifier polls GitHub until the expected runner is registered
// and online. It is safe to use concurrently.
type RegistrationVerifier struct {
	client   *Client
	timeout  time.Duration
	interval time.Duration
}

var _ RegistrationWaiter = (*RegistrationVerifier)(nil)

// NewRegistrationVerifier creates a bounded GitHub runner registration
// verifier using the supplied client.
func NewRegistrationVerifier(client *Client, config RegistrationVerifierConfig) (*RegistrationVerifier, error) {
	if client == nil {
		return nil, errors.New("GitHub client is required")
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultRegistrationTimeout
	}
	interval := config.Interval
	if interval <= 0 {
		interval = defaultRegistrationInterval
	}
	return &RegistrationVerifier{client: client, timeout: timeout, interval: interval}, nil
}

// WaitForRegistration waits for an exact runner ID/name match with status
// online. The caller's context always takes precedence over the verifier
// deadline.
func (v *RegistrationVerifier) WaitForRegistration(ctx context.Context, owner, repo string, runnerID int64, runnerName string) (JITRunner, error) {
	var empty JITRunner
	if v == nil || v.client == nil {
		return empty, errors.New("GitHub registration verifier is not configured")
	}
	if err := validateRepository(owner, repo); err != nil {
		return empty, err
	}
	if runnerID <= 0 || strings.TrimSpace(runnerName) == "" {
		return empty, errors.New("GitHub runner ID and name are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pollCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	for {
		runners, err := v.client.ListRunners(pollCtx, owner, repo)
		if err != nil {
			if pollCtx.Err() != nil {
				return empty, pollCtx.Err()
			}
			return empty, err
		}
		for _, runner := range runners {
			if runner.ID == runnerID && runner.Name == runnerName && strings.EqualFold(runner.Status, "online") {
				return runner, nil
			}
		}

		timer := time.NewTimer(v.interval)
		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return empty, ctx.Err()
			}
			return empty, pollCtx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) runnersURL(owner, repo string, page int) (*url.URL, error) {
	endpoint, err := c.jitURL(owner, repo)
	if err != nil {
		return nil, err
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/actions/runners/generate-jitconfig") + "/actions/runners"
	endpoint.RawPath = ""
	query := endpoint.Query()
	query.Set("per_page", strconv.Itoa(githubRunnerPageSize))
	query.Set("page", strconv.Itoa(page))
	endpoint.RawQuery = query.Encode()
	return endpoint, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint *url.URL, token string, destination any) error {
	requestCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	response, err := c.doWithRetry(requestCtx, func(attemptCtx context.Context) (*http.Request, error) {
		request, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create GitHub runner request: %w", err)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("User-Agent", c.userAgent)
		return request, nil
	})
	if err != nil {
		return fmt.Errorf("call GitHub runner endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return apiErrorFromResponse(response)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		return fmt.Errorf("decode GitHub runner response: %w", err)
	}
	return nil
}

func validateRepository(owner, repo string) error {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return errors.New("GitHub repository owner and name are required")
	}
	if strings.ContainsAny(owner, "/\\") || strings.ContainsAny(repo, "/\\") {
		return errors.New("GitHub repository owner and name must be single path segments")
	}
	return nil
}
