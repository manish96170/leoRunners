package controller

import (
	"context"
	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/lifecycle"
	"github.com/leo-runners/ci-platform/controller/internal/policy"
	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/scheduler"
	"github.com/leo-runners/ci-platform/controller/internal/state"
	"testing"
	"time"
)

func TestQueuedEventIsIdempotentAndCreatesLease(t *testing.T) {
	svc, provider := testService()
	payload := []byte(`{"action":"queued","workflow_job":{"id":1,"run_id":2,"run_attempt":1,"workflow_name":"build","repository":{"full_name":"acme/repo"}}}`)
	event, err := github.NormalizeWorkflowJob(payload, "delivery", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.HandleWorkflowJob(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err = svc.HandleWorkflowJob(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if provider.ActiveCount() != 1 {
		t.Fatalf("active runners=%d", provider.ActiveCount())
	}
}

func testService() (*Service, *providers.FakeProvider) {
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{Capacity: 2})
	svc := &Service{State: repo, Provider: provider, JobTTL: time.Hour}
	svc.Scheduler = &scheduler.Scheduler{State: repo, Mode: policy.Fallback, Capacities: []scheduler.Capacity{{Pool: policy.Pool{ID: "fake", Available: true}, Provider: provider}}}
	svc.Lifecycle = &lifecycle.Manager{State: repo, Provider: provider}
	return svc, provider
}
