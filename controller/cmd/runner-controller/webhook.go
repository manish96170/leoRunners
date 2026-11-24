package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/observability/prometheus"
)

// eventSubmitter accepts a verified event for background processing. It must
// not block.
type eventSubmitter interface {
	Submit(github.WorkflowJobEvent) error
}

// newWebhookHandler authenticates and validates a GitHub delivery, hands it to
// the submitter, and answers at once. Provisioning happens after the response:
// GitHub abandons a delivery after about ten seconds, and a runner takes
// longer than that to start.
//
//	401 bad or missing signature   400 unreadable or malformed workflow_job
//	200 ping                       202 accepted for processing, or ignored
//	503 backlog full or shutting down
//
// A signed event that is not a workflow_job is acknowledged and dropped. GitHub
// sends a ping when a webhook is configured and may send other events the
// controller does not act on; answering them with an error only fills the
// delivery log with failures.
func newWebhookHandler(secret string, maxBodyBytes int64, submitter eventSubmitter, metrics *prometheus.Exporter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !github.VerifyRequest(secret, r, body) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		switch name := strings.TrimSpace(r.Header.Get("X-GitHub-Event")); name {
		case "workflow_job":
		case "ping":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "pong\n")
			return
		default:
			if metrics != nil {
				_ = metrics.IncCounter("webhook_events_total", prometheus.Labels{"outcome": "ignored"})
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		event, err := github.NormalizeWorkflowJobHeaders(body, r.Header, time.Now().UTC())
		if err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		if err := submitter.Submit(event); err != nil {
			if metrics != nil {
				_ = metrics.IncCounter("webhook_events_total", prometheus.Labels{"outcome": "rejected"})
			}
			log.Printf("webhook %s for %s rejected: %v", event.EventID, event.JobKey, err)
			http.Error(w, "controller is busy, redeliver this event later", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}
