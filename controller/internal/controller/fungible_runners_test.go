package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

// A runner is created for each queued job, but GitHub gives a job to any idle
// runner with matching labels, so the runner created for job A can run job B.
// These tests pin the rule that ending a job retires only the runner GitHub says
// ran it.

func queuedPayload(id int) string {
	return fmt.Sprintf(`{"action":"queued","workflow_job":{"id":%d,"run_id":1,"run_attempt":1,"workflow_name":"build","repository":{"full_name":"acme/repo"}}}`, id)
}

func endedPayload(action string, id int, runner string) string {
	name := ""
	if runner != "" {
		name = fmt.Sprintf(`"runner_name":%q,`, runner)
	}
	return fmt.Sprintf(`{"action":%q,"workflow_job":{"id":%d,"run_id":1,"run_attempt":1,"workflow_name":"build","conclusion":"success",%s"repository":{"full_name":"acme/repo"}}}`, action, id, name)
}

func queueTwo(t *testing.T) (*Service, *providers.FakeProvider) {
	t.Helper()
	svc, provider := testService()
	for i, id := range []int{10, 20} {
		event := workflowEvent(t, queuedPayload(id), fmt.Sprintf("queued-%d", i))
		if err := svc.HandleWorkflowJob(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if provider.Active() != 2 {
		t.Fatalf("setup: active runners = %d, want 2", provider.Active())
	}
	return svc, provider
}

func runnerOfJob(t *testing.T, svc *Service, number int) state.Runner {
	t.Helper()
	lease, err := svc.State.GetLease(context.Background(), fmt.Sprintf("lease-acme/repo/job/%d", number))
	if err != nil {
		t.Fatal(err)
	}
	runner, err := svc.State.GetRunner(context.Background(), lease.RunnerID)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// The runner made for job 10 ran job 20. When job 20 ends, the runner made for
// job 10 is the one to retire, and the runner made for job 20 must survive: it
// may be running some other job.
func TestEndingAJobRetiresTheRunnerThatRanItNotTheOneCreatedForIt(t *testing.T) {
	svc, provider := queueTwo(t)
	ended := workflowEvent(t, endedPayload("completed", 20, "leo-runner-10-1"), "done-20")
	if err := svc.HandleWorkflowJob(context.Background(), ended); err != nil {
		t.Fatal(err)
	}
	if provider.Active() != 1 {
		t.Fatalf("active runners = %d, want exactly the runner created for job 20 to survive", provider.Active())
	}
	if got := runnerOfJob(t, svc, 10).State; got != state.RunnerTerminated {
		t.Fatalf("runner that ran the job is %s, want terminated", got)
	}
	if got := runnerOfJob(t, svc, 20).State; got == state.RunnerTerminated {
		t.Fatal("the runner created for the ended job was terminated although it did not run it")
	}
	if job, _ := svc.State.GetJob(context.Background(), "acme/repo/job/20"); job.State != state.JobCompleted {
		t.Fatalf("job 20 state = %s, want completed", job.State)
	}
}

func TestAJobThatNeverRanLeavesEveryRunnerAlone(t *testing.T) {
	svc, provider := queueTwo(t)
	cancelled := workflowEvent(t, endedPayload("cancelled", 20, ""), "cancel-20")
	if err := svc.HandleWorkflowJob(context.Background(), cancelled); err != nil {
		t.Fatal(err)
	}
	if provider.Active() != 2 {
		t.Fatalf("active runners = %d, want both left for the next queued job", provider.Active())
	}
	if job, _ := svc.State.GetJob(context.Background(), "acme/repo/job/20"); job.State != state.JobCancelled {
		t.Fatalf("job state = %s, want cancelled", job.State)
	}
}

func TestARunnerTheControllerDidNotCreateIsNeverTouched(t *testing.T) {
	for _, name := range []string{
		"blacksmith-4vcpu-ubuntu-2404",
		"some-other-runner-10-1",
		"leo-runner",
		"leo-runner-10",
		"leo-runner-ten-1",
		"leo-runner-10-one",
		"leo-runner-10-1-extra",
		"leo-runner-0-1",
		"leo-runner--5-1",
		"LEO-RUNNER-10-1",
	} {
		t.Run(name, func(t *testing.T) {
			svc, provider := queueTwo(t)
			ended := workflowEvent(t, endedPayload("completed", 20, name), "done-20")
			if err := svc.HandleWorkflowJob(context.Background(), ended); err != nil {
				t.Fatal(err)
			}
			if provider.Active() != 2 {
				t.Fatalf("runner name %q terminated something: active = %d, want 2", name, provider.Active())
			}
		})
	}
}

func TestACustomRunnerPrefixIsHonoured(t *testing.T) {
	// A name under another prefix is not ours, whatever job number it carries.
	svc, provider := queueTwo(t)
	svc.RunnerNamePrefix = "custom"
	other := workflowEvent(t, endedPayload("completed", 20, "leo-runner-10-1"), "done-other")
	if err := svc.HandleWorkflowJob(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if provider.Active() != 2 {
		t.Fatalf("active runners = %d: a name under the old prefix must not match", provider.Active())
	}

	// A name under the configured prefix is.
	svc, provider = queueTwo(t)
	svc.RunnerNamePrefix = "custom"
	mine := workflowEvent(t, endedPayload("completed", 20, "custom-10-1"), "done-mine")
	if err := svc.HandleWorkflowJob(context.Background(), mine); err != nil {
		t.Fatal(err)
	}
	if provider.Active() != 1 {
		t.Fatalf("active runners = %d, want the runner for job 10 retired", provider.Active())
	}
}

func TestEndingTheSameJobTwiceIsHarmless(t *testing.T) {
	svc, provider := queueTwo(t)
	for i := 0; i < 2; i++ {
		ended := workflowEvent(t, endedPayload("completed", 20, "leo-runner-10-1"), fmt.Sprintf("done-%d", i))
		if err := svc.HandleWorkflowJob(context.Background(), ended); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	if provider.Active() != 1 {
		t.Fatalf("active runners = %d, want 1", provider.Active())
	}
}

func TestARunnerWhoseLeaseWasNeverRecordedIsIgnored(t *testing.T) {
	svc, provider := queueTwo(t)
	ended := workflowEvent(t, endedPayload("completed", 20, "leo-runner-999-1"), "done-20")
	if err := svc.HandleWorkflowJob(context.Background(), ended); err != nil {
		t.Fatalf("an unknown runner must not fail the event: %v", err)
	}
	if provider.Active() != 2 {
		t.Fatalf("active runners = %d, want 2", provider.Active())
	}
}
