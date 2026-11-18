package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxAttempts  = 3
	defaultInitialDelay = 100 * time.Millisecond
	defaultMaxDelay     = 2 * time.Second
	maxRetryBodySize    = 1 << 20
)

// RetryConfig bounds retries for an individual GitHub HTTP request. MaxAttempts
// includes the initial request. The default policy is deterministic and does
// not add jitter; callers can replace Sleep to observe or control delays.
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Sleep        func(context.Context, time.Duration) error
}

func normalizeRetryConfig(config RetryConfig) RetryConfig {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = defaultMaxAttempts
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = defaultInitialDelay
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = defaultMaxDelay
	}
	if config.MaxDelay < config.InitialDelay {
		config.MaxDelay = config.InitialDelay
	}
	if config.Sleep == nil {
		config.Sleep = sleepWithContext
	}
	return config
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (c *Client) doWithRetry(ctx context.Context, newRequest func(context.Context) (*http.Request, error)) (*http.Response, error) {
	policy := c.retry
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		request, err := newRequest(ctx)
		if err != nil {
			return nil, err
		}
		response, err := c.httpClient.Do(request)
		if err == nil && !retryableStatus(response.StatusCode) {
			return response, nil
		}

		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				return nil, contextError(ctx, err)
			}
			lastErr = err
		} else {
			lastErr = apiErrorFromResponse(response)
			if attempt == policy.MaxAttempts {
				return response, nil
			}
			delay := retryDelay(policy, attempt, response.Header.Get("Retry-After"), response.StatusCode)
			closeRetryResponse(response)
			if err := policy.Sleep(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}

		if attempt == policy.MaxAttempts {
			return nil, lastErr
		}
		delay := exponentialDelay(policy, attempt)
		if err := policy.Sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func contextError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func exponentialDelay(policy RetryConfig, attempt int) time.Duration {
	delay := policy.InitialDelay
	for i := 1; i < attempt && delay < policy.MaxDelay; i++ {
		if delay > policy.MaxDelay/2 {
			delay = policy.MaxDelay
		} else {
			delay *= 2
		}
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}

func retryDelay(policy RetryConfig, attempt int, header string, status int) time.Duration {
	if (status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable) && strings.TrimSpace(header) != "" {
		if delay, ok := parseRetryAfter(header, time.Now()); ok {
			if delay > policy.MaxDelay {
				return policy.MaxDelay
			}
			return delay
		}
	}
	return exponentialDelay(policy, attempt)
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds > int64(time.Duration(1<<63-1)/time.Second) {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if when.Before(now) {
		return 0, true
	}
	return when.Sub(now), true
}

func apiErrorFromResponse(response *http.Response) *APIError {
	errorBody, err := io.ReadAll(io.LimitReader(response.Body, maxJITErrorBodySize))
	if err != nil {
		return &APIError{StatusCode: response.StatusCode, Status: response.Status, RequestID: response.Header.Get("X-GitHub-Request-Id"), Body: "<unable to read response body>"}
	}
	return &APIError{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		RequestID:  response.Header.Get("X-GitHub-Request-Id"),
		Body:       strings.TrimSpace(string(errorBody)),
	}
}

func closeRetryResponse(response *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxRetryBodySize))
	_ = response.Body.Close()
}
