package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateJITConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/repos/acme/widgets/actions/runners/generate-jitconfig" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer installation-token" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("accept = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubAPIVersion {
			t.Errorf("API version = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"name":"runner-1","runner_group_id":7,"labels":["self-hosted","linux"],"work_folder":"_work"}`
		if string(body) != want {
			t.Errorf("body = %s, want %s", body, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"runner":{"id":11,"name":"runner-1","os":"linux","busy":false,"status":"offline","labels":[{"id":1,"name":"self-hosted"}]},"encoded_jit_config":"encoded-value","expires_at":"2026-09-13T12:00:00Z"}`)
	}))
	defer server.Close()

	client, err := NewJITClient(JITClientConfig{
		BaseURL:     server.URL + "/api/",
		TokenSource: StaticBearerToken("installation-token"),
		UserAgent:   "test-client",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GenerateJITConfig(context.Background(), "acme", "widgets", JITConfigRequest{
		Name: "runner-1", RunnerGroupID: int64Ptr(7), Labels: []string{"self-hosted", "linux"}, WorkFolder: "_work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Runner.ID != 11 || got.EncodedJITConfig != "encoded-value" || got.ExpiresAt.IsZero() {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func int64Ptr(value int64) *int64 { return &value }

func TestGenerateJITConfigEscapesPathSegments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/repos/acme%20inc/widgets%20one/actions/runners/generate-jitconfig" {
			t.Errorf("escaped path = %q", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"encoded_jit_config":"value"}`)
	}))
	defer server.Close()
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GenerateJITConfig(context.Background(), "acme inc", "widgets one", JITConfigRequest{Name: "runner", RunnerGroupID: int64Ptr(1), Labels: []string{"self-hosted"}}); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateJITConfigReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-GitHub-Request-Id", "request-123")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Resource not accessible by integration"}`)
	}))
	defer server.Close()
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GenerateJITConfig(context.Background(), "acme", "widgets", JITConfigRequest{Name: "runner", RunnerGroupID: int64Ptr(1), Labels: []string{"self-hosted"}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.RequestID != "request-123" || !strings.Contains(apiErr.Body, "not accessible") {
		t.Fatalf("unexpected API error: %+v", apiErr)
	}
}

func TestGenerateJITConfigHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.GenerateJITConfig(ctx, "acme", "widgets", JITConfigRequest{Name: "runner", RunnerGroupID: int64Ptr(1), Labels: []string{"self-hosted"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestGenerateJITConfigRejectsMalformedSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GenerateJITConfig(context.Background(), "acme", "widgets", JITConfigRequest{Name: "runner", RunnerGroupID: int64Ptr(1), Labels: []string{"self-hosted"}})
	if err == nil || !strings.Contains(err.Error(), "encoded_jit_config") {
		t.Fatalf("error = %v, want missing encoded config", err)
	}
}

func TestNewJITClientValidatesConfiguration(t *testing.T) {
	for name, config := range map[string]JITClientConfig{
		"missing token source": {BaseURL: "https://api.github.com"},
		"relative URL":         {BaseURL: "/api", TokenSource: StaticBearerToken("token")},
		"unsupported scheme":   {BaseURL: "ftp://github.example", TokenSource: StaticBearerToken("token")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewJITClient(config); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}

func TestJITURLPreservesBasePathAndEscaping(t *testing.T) {
	client, err := NewJITClient(JITClientConfig{
		BaseURL:     "https://github.example/api/v3/",
		TokenSource: StaticBearerToken("token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.jitURL("owner name", "repo name")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(got.String())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.EscapedPath() != "/api/v3/repos/owner%20name/repo%20name/actions/runners/generate-jitconfig" {
		t.Fatalf("escaped URL path = %q", parsed.EscapedPath())
	}
}
