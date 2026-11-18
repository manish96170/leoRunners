package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestListRunnersAuthenticatesAndFollowsPagination(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/repos/acme/widgets/actions/runners" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("accept header = %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
			t.Errorf("API version header = %q", r.Header.Get("X-GitHub-Api-Version"))
		}
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil {
			t.Errorf("page query: %v", err)
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			// A full page forces the client to request page 2.
			runners := make([]JITRunner, githubRunnerPageSize)
			for i := range runners {
				runners[i] = JITRunner{ID: int64(i + 1), Name: "runner-" + strconv.Itoa(i+1), Status: "offline"}
			}
			_ = json.NewEncoder(w).Encode(RunnerList{TotalCount: 101, Runners: runners})
			return
		}
		_ = json.NewEncoder(w).Encode(RunnerList{TotalCount: 101, Runners: []JITRunner{{ID: 101, Name: "runner-101", Status: "online"}}})
	}))
	defer server.Close()

	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL + "/api/", TokenSource: StaticBearerToken("test-token"), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	runners, err := client.ListRunners(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if len(runners) != 101 || runners[100].ID != 101 || requests.Load() != 2 {
		t.Fatalf("got %d runners across %d requests", len(runners), requests.Load())
	}
}

func TestRegistrationVerifierWaitsForOnlineRunner(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		poll := polls.Add(1)
		status := "offline"
		if poll >= 2 {
			status = "online"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RunnerList{TotalCount: 1, Runners: []JITRunner{
			{ID: 9, Name: "ephemeral-9", Status: status},
		}})
	}))
	defer server.Close()

	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewRegistrationVerifier(client, RegistrationVerifierConfig{Timeout: time.Second, Interval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := verifier.WaitForRegistration(context.Background(), "acme", "widgets", 9, "ephemeral-9")
	if err != nil {
		t.Fatal(err)
	}
	if runner.ID != 9 || runner.Name != "ephemeral-9" || !strings.EqualFold(runner.Status, "online") || polls.Load() < 2 {
		t.Fatalf("unexpected result: %+v after %d polls", runner, polls.Load())
	}
}

func TestRegistrationVerifierRequiresExactOnlineIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RunnerList{TotalCount: 2, Runners: []JITRunner{
			{ID: 7, Name: "wrong-name", Status: "online"},
			{ID: 9, Name: "ephemeral-9", Status: "offline"},
		}})
	}))
	defer server.Close()

	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewRegistrationVerifier(client, RegistrationVerifierConfig{Timeout: 25 * time.Millisecond, Interval: 2 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = verifier.WaitForRegistration(context.Background(), "acme", "widgets", 9, "ephemeral-9")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestRegistrationVerifierHonorsCallerCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RunnerList{TotalCount: 0, Runners: []JITRunner{}})
	}))
	defer server.Close()

	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewRegistrationVerifier(client, RegistrationVerifierConfig{Timeout: time.Second, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = verifier.WaitForRegistration(ctx, "acme", "widgets", 9, "ephemeral-9")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestListRunnersReturnsTypedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-GitHub-Request-Id", "req-123")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListRunners(context.Background(), "acme", "widgets")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.RequestID != "req-123" {
		t.Fatalf("error = %#v, want typed forbidden API error", err)
	}
}
