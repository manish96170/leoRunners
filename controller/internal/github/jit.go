package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAPIURL       = "https://api.github.com/"
	defaultJITTimeout   = 30 * time.Second
	githubAPIVersion    = "2022-11-28"
	maxJITErrorBodySize = 64 << 10
)

// BearerTokenSource supplies a current GitHub App installation access token.
// Implementations should return a short-lived token and honor ctx.
type BearerTokenSource interface {
	BearerToken(ctx context.Context) (string, error)
}

// StaticBearerToken is useful for tests and callers that already manage token
// rotation outside this package.
type StaticBearerToken string

func (t StaticBearerToken) BearerToken(context.Context) (string, error) {
	if strings.TrimSpace(string(t)) == "" {
		return "", errors.New("GitHub bearer token is empty")
	}
	return string(t), nil
}

// JITClientConfig configures a GitHub JIT API client.
type JITClientConfig struct {
	// BaseURL defaults to GitHub's public API URL. It is injectable for tests
	// and GitHub Enterprise Server deployments.
	BaseURL string
	// HTTPClient defaults to a client with a bounded timeout.
	HTTPClient  *http.Client
	TokenSource BearerTokenSource
	UserAgent   string
	Timeout     time.Duration
	Retry       RetryConfig
}

// JITClient generates one-time configuration for an ephemeral Actions runner.
type JITClient interface {
	GenerateJITConfig(ctx context.Context, owner, repo string, request JITConfigRequest) (JITConfigResponse, error)
}

// Client is an AWS-free GitHub REST API client.
type Client struct {
	baseURL     *url.URL
	httpClient  *http.Client
	tokenSource BearerTokenSource
	userAgent   string
	timeout     time.Duration
	retry       RetryConfig
}

// NewJITClient creates a client for GitHub's repository JIT endpoint.
func NewJITClient(config JITClientConfig) (*Client, error) {
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("GitHub API base URL must be an absolute HTTP(S) URL")
	}
	if config.TokenSource == nil {
		return nil, errors.New("GitHub bearer token source is required")
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultJITTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		userAgent = "leo-runners-controller"
	}
	return &Client{
		baseURL: parsed, httpClient: httpClient, tokenSource: config.TokenSource,
		userAgent: userAgent, timeout: timeout, retry: normalizeRetryConfig(config.Retry),
	}, nil
}

// JITConfigRequest is the request body accepted by GitHub's generate-jitconfig
// endpoint. Labels must include the labels required by the workflow job.
type JITConfigRequest struct {
	Name          string   `json:"name"`
	RunnerGroupID *int64   `json:"runner_group_id,omitempty"`
	Labels        []string `json:"labels"`
	WorkFolder    string   `json:"work_folder,omitempty"`
}

// JITConfigResponse is GitHub's one-time runner configuration response.
type JITConfigResponse struct {
	Runner           JITRunner `json:"runner"`
	EncodedJITConfig string    `json:"encoded_jit_config"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type JITRunner struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	OS     string  `json:"os"`
	Busy   bool    `json:"busy"`
	Status string  `json:"status"`
	Labels []Label `json:"labels"`
}

type Label struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// APIError describes a non-2xx GitHub response. Body is bounded and intended
// for diagnostics; callers should avoid logging it if it may contain secrets.
type APIError struct {
	StatusCode int
	Status     string
	RequestID  string
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("GitHub API returned %s", e.Status)
	}
	return fmt.Sprintf("GitHub API returned %s: %s", e.Status, e.Body)
}

// GenerateJITConfig requests a one-time configuration for a repository runner.
func (c *Client) GenerateJITConfig(ctx context.Context, owner, repo string, request JITConfigRequest) (JITConfigResponse, error) {
	var result JITConfigResponse
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return result, errors.New("GitHub repository owner and name are required")
	}
	if strings.TrimSpace(request.Name) == "" || len(request.Labels) == 0 || request.RunnerGroupID == nil || *request.RunnerGroupID <= 0 {
		return result, errors.New("GitHub JIT request requires name, labels, and a positive runner_group_id")
	}
	if strings.ContainsAny(owner, "/\\") || strings.ContainsAny(repo, "/\\") {
		return result, errors.New("GitHub repository owner and name must be single path segments")
	}

	token, err := c.tokenSource.BearerToken(ctx)
	if err != nil {
		return result, fmt.Errorf("get GitHub bearer token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return result, errors.New("GitHub bearer token is empty")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return result, fmt.Errorf("encode GitHub JIT request: %w", err)
	}
	endpoint, err := c.jitURL(owner, repo)
	if err != nil {
		return result, err
	}
	requestCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	response, err := c.doWithRetry(requestCtx, func(attemptCtx context.Context) (*http.Request, error) {
		httpRequest, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create GitHub JIT request: %w", err)
		}
		httpRequest.Header.Set("Accept", "application/vnd.github+json")
		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
		httpRequest.Header.Set("Authorization", "Bearer "+token)
		httpRequest.Header.Set("User-Agent", c.userAgent)
		return httpRequest, nil
	})
	if err != nil {
		return result, fmt.Errorf("call GitHub JIT endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, apiErrorFromResponse(response)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode GitHub JIT response: %w", err)
	}
	if strings.TrimSpace(result.EncodedJITConfig) == "" {
		return result, errors.New("GitHub JIT response has no encoded_jit_config")
	}
	return result, nil
}

func (c *Client) jitURL(owner, repo string) (*url.URL, error) {
	base := *c.baseURL
	rawPath := strings.TrimRight(base.EscapedPath(), "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/actions/runners/generate-jitconfig"
	base.RawPath = rawPath
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return nil, fmt.Errorf("escape GitHub repository path: %w", err)
	}
	base.Path = decodedPath
	return &base, nil
}
