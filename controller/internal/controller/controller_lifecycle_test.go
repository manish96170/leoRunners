package controller

import (
	"context"
	"github.com/leo-runners/ci-platform/controller/internal/github"
	"testing"
	"time"
)

func TestCompletedEventTerminatesRunner(t *testing.T) {
	svc, provider := testService()
	queued := workflowEvent(t, `{"action":"queued","workflow_job":{"id":10,"run_id":20,"run_attempt":1,"workflow_name":"build","repository":{"full_name":"acme/repo"}}}`, "delivery-queued")
	if err := svc.HandleWorkflowJob(context.Background(), queued); err != nil {
		t.Fatal(err)
	}
	completed := workflowEvent(t, `{"action":"completed","workflow_job":{"id":10,"run_id":20,"run_attempt":1,"workflow_name":"build","conclusion":"success","repository":{"full_name":"acme/repo"}}}`, "delivery-completed")
	if err := svc.HandleWorkflowJob(context.Background(), completed); err != nil {
		t.Fatal(err)
	}
	if provider.ActiveCount() != 0 {
		t.Fatalf("active runners=%d", provider.ActiveCount())
	}
}

func workflowEvent(t *testing.T, payload, delivery string) github.WorkflowJobEvent {
	t.Helper()
	event, err := github.NormalizeWorkflowJob([]byte(payload), delivery, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return event
}
