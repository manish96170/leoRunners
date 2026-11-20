package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/dispatch"
	"github.com/leo-runners/ci-platform/controller/internal/github"
)

const testSecret = "test-webhook-secret"

const queuedPayload = `{"action":"queued","workflow_job":{"id":11,"run_id":22,"run_attempt":1,"name":"build","workflow_name":"CI","labels":["self-hosted"],"repository":{"id":1,"full_name":"example-org/example-repo","name":"example-repo"}},"repository":{"id":1,"full_name":"example-org/example-repo","name":"example-repo"}}`

func signedRequest(t *testing.T, body, secret string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-GitHub-Event", "workflow_job")
	request.Header.Set("X-GitHub-Delivery", "delivery-1")
	return request
}

func newBlockingDispatcher(t *testing.T, config dispatch.Config, release <-chan struct{}, handled *atomic.Int64) *dispatch.Dispatcher {
	t.Helper()
	d, err := dispatch.New(func(ctx context.Context, _ github.WorkflowJobEvent) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		handled.Add(1)
		return nil
	}, config)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWebhookAcknowledgesImmediatelyWhileProvisioningContinues(t *testing.T) {
	release := make(chan struct{})
	var handled atomic.Int64
	d := newBlockingDispatcher(t, dispatch.Config{Workers: 2, QueueSize: 4}, release, &handled)
	handler := newWebhookHandler(testSecret, 1<<20, d, nil)

	recorder := httptest.NewRecorder()
	started := time.Now()
	handler.ServeHTTP(recorder, signedRequest(t, queuedPayload, testSecret))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", recorder.Code)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("response took %s although processing is still blocked", elapsed)
	}
	if handled.Load() != 0 {
		t.Fatal("the handler finished before the response, so the test proves nothing")
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if handled.Load() != 1 {
		t.Fatalf("handled = %d, want the event processed after the response", handled.Load())
	}
}

func TestWebhookRejectsBadSignatureAndPayloadBeforeQueueing(t *testing.T) {
	var submitted atomic.Int64
	handler := newWebhookHandler(testSecret, 1<<20, submitFunc(func(github.WorkflowJobEvent) error {
		submitted.Add(1)
		return nil
	}), nil)

	cases := map[string]struct {
		request *http.Request
		want    int
	}{
		"wrong secret":     {signedRequest(t, queuedPayload, "other-secret"), http.StatusUnauthorized},
		"no signature":     {httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(queuedPayload)), http.StatusUnauthorized},
		"signed non json":  {signedRequest(t, "not json", testSecret), http.StatusBadRequest},
		"signed no job id": {signedRequest(t, `{"action":"queued","workflow_job":{}}`, testSecret), http.StatusBadRequest},
		"unknown action":   {signedRequest(t, `{"action":"waiting","workflow_job":{"id":1,"run_id":1}}`, testSecret), http.StatusBadRequest},
	}
	for name, tc := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tc.request)
		if recorder.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", name, recorder.Code, tc.want)
		}
	}
	if submitted.Load() != 0 {
		t.Fatalf("%d rejected deliveries were still queued", submitted.Load())
	}
}

func TestWebhookAnswers503WhenTheBacklogIsFullOrClosing(t *testing.T) {
	release := make(chan struct{})
	var handled atomic.Int64
	d := newBlockingDispatcher(t, dispatch.Config{Workers: 1, QueueSize: 1}, release, &handled)
	handler := newWebhookHandler(testSecret, 1<<20, d, nil)

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, signedRequest(t, queuedPayload, testSecret))
	other := `{"action":"queued","workflow_job":{"id":99,"run_id":22,"repository":{"full_name":"example-org/example-repo"}},"repository":{"full_name":"example-org/example-repo"}}`
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, signedRequest(t, other, testSecret))
	if first.Code != http.StatusAccepted || second.Code != http.StatusServiceUnavailable {
		t.Fatalf("statuses = %d, %d; want 202 then 503", first.Code, second.Code)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	closed := httptest.NewRecorder()
	handler.ServeHTTP(closed, signedRequest(t, queuedPayload, testSecret))
	if closed.Code != http.StatusServiceUnavailable {
		t.Fatalf("status after shutdown = %d, want 503", closed.Code)
	}
}

func TestWebhookRejectsOversizedBody(t *testing.T) {
	handler := newWebhookHandler(testSecret, 64, submitFunc(func(github.WorkflowJobEvent) error { return nil }), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, signedRequest(t, queuedPayload, testSecret))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a body over the limit", recorder.Code)
	}
}

type submitFunc func(github.WorkflowJobEvent) error

func (f submitFunc) Submit(event github.WorkflowJobEvent) error { return f(event) }
