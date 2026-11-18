package controller

import (
	"context"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/lifecycle"
	"github.com/leo-runners/ci-platform/controller/internal/state"
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

func TestCancelledEventTerminatesRunnerAndUpdatesState(t *testing.T) {
	svc, provider := testService()
	queued := workflowEvent(t, `{"action":"queued","workflow_job":{"id":11,"run_id":21,"run_attempt":1,"workflow_name":"build","repository":{"full_name":"acme/repo"}}}`, "cancel-queued")
	if err := svc.HandleWorkflowJob(context.Background(), queued); err != nil {
		t.Fatal(err)
	}
	cancelled := workflowEvent(t, `{"action":"cancelled","workflow_job":{"id":11,"run_id":21,"run_attempt":1,"workflow_name":"build","repository":{"full_name":"acme/repo"}}}`, "cancel-event")
	if err := svc.HandleWorkflowJob(context.Background(), cancelled); err != nil {
		t.Fatal(err)
	}
	if provider.ActiveCount() != 0 {
		t.Fatalf("active runners=%d", provider.ActiveCount())
	}
	job, err := svc.State.GetJob(context.Background(), queued.JobKey)
	if err != nil || job.State != state.JobCancelled {
		t.Fatalf("job=%+v err=%v", job, err)
	}
	lease, err := svc.State.GetLease(context.Background(), "lease-"+queued.JobKey)
	if err != nil || lease.State != state.LeaseTerminated {
		t.Fatalf("lease=%+v err=%v", lease, err)
	}
}

func TestControllerReaperTerminatesExpiredRunner(t *testing.T) {
	svc, provider := testService()
	queued := workflowEvent(t, `{"action":"queued","workflow_job":{"id":12,"run_id":22,"run_attempt":1,"workflow_name":"build","repository":{"full_name":"acme/repo"}}}`, "reap-queued")
	if err := svc.HandleWorkflowJob(context.Background(), queued); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	job, err := svc.State.GetJob(ctx, queued.JobKey)
	if err != nil {
		t.Fatal(err)
	}
	job.ExpiresAt = time.Unix(100, 0).UTC()
	if _, err := svc.State.SaveJob(ctx, job, job.Revision); err != nil {
		t.Fatal(err)
	}
	lease, err := svc.State.GetLease(ctx, "lease-"+queued.JobKey)
	if err != nil {
		t.Fatal(err)
	}
	lease.ExpiresAt = job.ExpiresAt
	if _, err := svc.State.SaveLease(ctx, lease, lease.Revision); err != nil {
		t.Fatal(err)
	}
	runner, err := svc.State.GetRunner(ctx, lease.RunnerID)
	if err != nil {
		t.Fatal(err)
	}
	runner.ExpiresAt = job.ExpiresAt
	if _, err := svc.State.SaveRunner(ctx, runner, runner.Revision); err != nil {
		t.Fatal(err)
	}
	reaper := &lifecycle.Reconciler{State: svc.State, Provider: provider, OperationTimeout: time.Second}
	report, err := reaper.ReapExpired(ctx, time.Unix(200, 0).UTC())
	if err != nil || report.RunnersReaped != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if provider.ActiveCount() != 0 {
		t.Fatalf("active runners=%d", provider.ActiveCount())
	}
	runner, err = svc.State.GetRunner(ctx, runner.ID)
	if err != nil || runner.State != state.RunnerTerminated {
		t.Fatalf("runner=%+v err=%v", runner, err)
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
