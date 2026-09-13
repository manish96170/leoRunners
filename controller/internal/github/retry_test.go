package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func retryTestConfig(sleep func(context.Context, time.Duration) error) RetryConfig {
	return RetryConfig{MaxAttempts: 3, InitialDelay: 10 * time.Millisecond, MaxDelay: time.Second, Sleep: sleep}
}

func TestGenerateJITConfigRetriesTransientStatusAndPreservesAuth(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("authorization = %q", got)
		}
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"message":"try again"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"encoded_jit_config":"secret-value"}`)
	}))
	defer server.Close()

	var delays []time.Duration
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Retry: retryTestConfig(func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GenerateJITConfig(context.Background(), "acme", "widgets", JITConfigRequest{Name: "runner", RunnerGroupID: int64Ptr(1), Labels: []string{"self-hosted"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.EncodedJITConfig != "secret-value" || attempts.Load() != 3 {
		t.Fatalf("result=%+v attempts=%d", got, attempts.Load())
	}
	if len(delays) != 2 || delays[0] != 10*time.Millisecond || delays[1] != 20*time.Millisecond {
		t.Fatalf("delays = %v", delays)
	}
}

func TestListRunnersRetriesRetryAfterForRateLimit(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"runners":[{"id":9,"name":"runner","status":"online"}]}`)
	}))
	defer server.Close()

	var delays []time.Duration
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Retry: RetryConfig{MaxAttempts: 2, InitialDelay: time.Millisecond, MaxDelay: 5 * time.Second, Sleep: func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	runners, err := client.ListRunners(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].ID != 9 || len(delays) != 1 || delays[0] != 3*time.Second {
		t.Fatalf("runners=%+v delays=%v", runners, delays)
	}
}

func TestRetryAfterIsBoundedAndInvalidValuesUseBackoff(t *testing.T) {
	policy := RetryConfig{InitialDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond}
	if got := retryDelay(policy, 1, "999", http.StatusTooManyRequests); got != 50*time.Millisecond {
		t.Fatalf("bounded Retry-After delay = %s", got)
	}
	if got := retryDelay(policy, 2, "not-a-delay", http.StatusTooManyRequests); got != 20*time.Millisecond {
		t.Fatalf("invalid Retry-After fallback = %s", got)
	}
	if _, ok := parseRetryAfter("9223372036854775807", time.Now()); ok {
		t.Fatal("accepted Retry-After duration that overflows time.Duration")
	}
}

func TestGitHubRetryRetriesNetworkError(t *testing.T) {
	var attempts atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("temporary connection reset")
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}")), Request: request}, nil
	})
	client, err := NewJITClient(JITClientConfig{BaseURL: "https://github.example/", TokenSource: StaticBearerToken("token"), HTTPClient: &http.Client{Transport: transport}, Retry: retryTestConfig(func(context.Context, time.Duration) error { return nil })})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.doWithRetry(context.Background(), func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL.String(), nil)
	})
	if err != nil || response == nil || attempts.Load() != 2 {
		t.Fatalf("response=%v err=%v attempts=%d", response, err, attempts.Load())
	}
	_ = response.Body.Close()
}

func TestGitHubRetryStopsOnNonTransientStatus(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Retry: retryTestConfig(func(context.Context, time.Duration) error { t.Fatal("unexpected retry"); return nil })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListRunners(context.Background(), "acme", "widgets")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || attempts.Load() != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts.Load())
	}
}

func TestGitHubRetryHonorsCancellationDuringBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	var slept atomic.Int32
	client, err := NewJITClient(JITClientConfig{BaseURL: server.URL, TokenSource: StaticBearerToken("token"), Retry: retryTestConfig(func(ctx context.Context, _ time.Duration) error {
		slept.Add(1)
		<-ctx.Done()
		return ctx.Err()
	})})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, requestErr := client.ListRunners(ctx, "acme", "widgets"); done <- requestErr }()
	for slept.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
