package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/leo-runners/ci-platform/controller/internal/capacity"
	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/lifecycle"
	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/runners"
	"github.com/leo-runners/ci-platform/controller/internal/scheduler"
	"github.com/leo-runners/ci-platform/controller/internal/state"
	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	State            state.Repository
	Scheduler        *scheduler.Scheduler
	Lifecycle        *lifecycle.Manager
	Assignment       runners.AssignmentService
	Provider         providers.Provider
	Providers        map[string]providers.Provider
	Sink             telemetry.Sink
	JobTTL           time.Duration
	RunnerGroupID    *int64
	RunnerNamePrefix string
	CapacityRegistry *capacity.Registry
	ScopePolicy      *github.ScopePolicy
}

func (s *Service) HandleWorkflowJob(ctx context.Context, event github.WorkflowJobEvent) (err error) {
	if s.State == nil || s.Lifecycle == nil || (s.Scheduler == nil && s.Assignment == nil) {
		return fmt.Errorf("controller dependencies are not configured")
	}
	if s.ScopePolicy != nil {
		if err := s.ScopePolicy.ValidateEvent(event); err != nil {
			return err
		}
	}
	inserted, err := s.State.InsertLifecycleEvent(ctx, state.LifecycleEvent{ID: event.EventID, IdempotencyKey: event.DedupKey, Type: string(event.Action), JobID: event.JobKey, OccurredAt: event.ReceivedAt})
	if err != nil {
		return err
	}
	if !inserted {
		return nil
	}
	defer func() {
		if err != nil {
			_ = s.State.DeleteLifecycleEvent(context.Background(), event.DedupKey)
		}
	}()
	if s.JobTTL <= 0 {
		s.JobTTL = 2 * time.Hour
	}
	now := time.Now().UTC()
	job := state.Job{ID: event.JobKey, Repository: event.Job.Repository.FullName, Workflow: event.Job.WorkflowName, RunID: event.Job.RunID, JobID: event.Job.ID, Attempt: event.Job.RunAttempt, Labels: append([]string(nil), event.Job.Labels...), State: state.JobQueued, CreatedAt: now, ExpiresAt: now.Add(s.JobTTL)}
	switch event.Action {
	case github.ActionQueued:
		if err := s.State.CreateJob(ctx, job); err != nil {
			if err == state.ErrAlreadyExists {
				return nil
			}
			return err
		}
		job, err = s.State.GetJob(ctx, job.ID)
		if err != nil {
			return err
		}
		if err := s.Lifecycle.Transition(ctx, &job, state.JobProvisioning); err != nil {
			return err
		}
		prefix := s.RunnerNamePrefix
		if prefix == "" {
			prefix = "leo-runner"
		}
		runnerName := fmt.Sprintf("%s-%d-%d", prefix, job.JobID, job.Attempt)
		spec := providers.RunnerSpec{Labels: job.Labels, OS: "linux", Architecture: "x86_64", ExpiresAt: job.ExpiresAt, Metadata: map[string]string{"job_id": fmt.Sprint(job.JobID), "repository": job.Repository, "workflow": job.Workflow, "run_id": fmt.Sprint(job.RunID), "owner": job.Repository, "created_at": job.CreatedAt.UTC().Format(time.RFC3339), "expires_at": job.ExpiresAt.Format(time.RFC3339), "github_runner_name": runnerName, "provider_attempt_key": event.DedupKey}}
		var runner state.Runner
		var err error
		if s.Assignment != nil {
			assignmentProvider := s.Provider
			poolID := ""
			if s.Scheduler.PoolRegistry != nil {
				assignmentProvider, poolID, err = s.Scheduler.Reserve(spec)
				if err != nil {
					job.State = state.JobFailed
					_, _ = s.State.SaveJob(ctx, job, job.Revision)
					return err
				}
			}
			assignment, assignmentErr := s.Assignment.Assign(ctx, runners.AssignmentRequest{Job: job, Spec: spec, Provider: assignmentProvider, CapacityPoolID: poolID, IsFork: event.Job.Repository.IsFork})
			if assignmentErr != nil && poolID != "" {
				_ = s.Scheduler.PoolRegistry.Release(poolID, spec.CPU, spec.MemoryGB, boolInt(spec.GPU))
			}
			runner, err = assignment.Runner, assignmentErr
		} else {
			runner, err = s.Scheduler.Schedule(ctx, job, spec)
		}
		if err != nil {
			job.State = state.JobFailed
			_, _ = s.State.SaveJob(ctx, job, job.Revision)
			return err
		}
		lease := state.Lease{ID: "lease-" + job.ID, JobID: job.ID, RunnerID: runner.ID, State: state.LeaseActive, ExpiresAt: job.ExpiresAt}
		if err := s.State.CreateLease(ctx, lease); err != nil {
			_ = s.Provider.Terminate(ctx, providers.RunnerInstance{ID: runner.ID, ProviderID: runner.ProviderInstanceID, Provider: runner.Provider})
			return err
		}
		return s.settleAssigned(ctx, job.ID)
	case github.ActionInProgress:
		stored, err := s.State.GetJob(ctx, event.JobKey)
		if err != nil {
			return err
		}
		if isTerminalJobState(stored.State) {
			// A job we already gave up on, or that already ended, can still
			// report progress: GitHub starts jobs on any idle runner with
			// matching labels, not only the one created for them. Nothing to
			// record.
			return nil
		}
		return s.Lifecycle.Transition(ctx, &stored, state.JobRunning)
	case github.ActionCancelled:
		return s.finishAndRetire(ctx, event, state.JobCancelled)
	case github.ActionCompleted:
		conclusion := state.JobCompleted
		if event.Job.Conclusion == "cancelled" || event.Job.Conclusion == "skipped" {
			conclusion = state.JobCancelled
		} else if event.Job.Conclusion != "success" {
			conclusion = state.JobFailed
		}
		return s.finishAndRetire(ctx, event, conclusion)
	default:
		return nil
	}
}

// settleAssigned records that the job has a runner. The wait for the runner to
// register takes minutes, during which the job record may have moved on (the
// runner can start its job, or the job can end, before this point), so it reads
// the job again instead of trusting the copy from before the wait. A job that is
// already past provisioning is left as it is.
func (s *Service) settleAssigned(ctx context.Context, jobID string) error {
	const attempts = 5
	for i := 0; i < attempts; i++ {
		stored, err := s.State.GetJob(ctx, jobID)
		if err != nil {
			return err
		}
		if stored.State != state.JobProvisioning {
			return nil
		}
		err = s.Lifecycle.Transition(ctx, &stored, state.JobAssigned)
		if !errors.Is(err, state.ErrRevisionConflict) {
			return err
		}
	}
	return fmt.Errorf("job %s kept changing while it was being marked assigned: %w", jobID, state.ErrRevisionConflict)
}

// isTerminalJobState reports states a job never leaves. A late completed or
// cancelled event for a job already failed (for example after its runner could
// not be created) is expected, not an error.
func isTerminalJobState(s state.JobState) bool {
	return s == state.JobCompleted || s == state.JobCancelled || s == state.JobFailed
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// finishAndRetire records the end of a job and retires the runner that ran it.
func (s *Service) finishAndRetire(ctx context.Context, event github.WorkflowJobEvent, final state.JobState) error {
	if err := s.finish(ctx, event.JobKey, final); err != nil {
		return err
	}
	return s.retireRunnerThatRan(ctx, event)
}

// finish records the end of a job. It does not touch any runner: runners are
// not tied to the job they were created for (see retireRunnerThatRan).
func (s *Service) finish(ctx context.Context, jobID string, final state.JobState) error {
	job, err := s.State.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.State != final && !isTerminalJobState(job.State) {
		if err := s.Lifecycle.Transition(ctx, &job, final); err != nil {
			return err
		}
	}
	return nil
}

// retireRunnerThatRan terminates the runner GitHub reports as having run the
// job, and only that one.
//
// A runner is created for each queued job, but GitHub does not bind them: any
// idle runner whose labels match takes the next queued job. The runner created
// for job A may therefore run job B, and A's completion says nothing about it.
// Terminating "A's runner" when A ends killed B mid-run in a live test. The
// webhook names the runner that ran the job, and the runner's name carries the
// job number it was created for, which leads back to its record.
//
// A job that ends without ever running (cancelled while queued) names no runner.
// Its own runner is left alone: it stays available for the next queued job, and
// is removed by its lifetime if none comes.
func (s *Service) retireRunnerThatRan(ctx context.Context, event github.WorkflowJobEvent) error {
	jobNumber, ok := s.createdForJobNumber(event.Job.RunnerName)
	if !ok {
		return nil
	}
	lease, err := s.leaseOfRunnerCreatedFor(ctx, event.Job.Repository.FullName, jobNumber)
	if errors.Is(err, state.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	runner, err := s.State.GetRunner(ctx, lease.RunnerID)
	if err != nil {
		return err
	}
	provider := s.Provider
	if s.Providers != nil {
		var found bool
		provider, found = s.Providers[runner.Provider]
		if !found || provider == nil {
			return fmt.Errorf("provider %q is not configured for runner cleanup", runner.Provider)
		}
	}
	if err := provider.Terminate(ctx, providers.RunnerInstance{ID: runner.ID, ProviderID: runner.ProviderInstanceID, Provider: runner.Provider}); err != nil {
		return err
	}
	if s.Scheduler != nil {
		_ = s.Scheduler.Release(runner)
	}
	runner.State = state.RunnerTerminated
	if _, err := s.State.SaveRunner(ctx, runner, runner.Revision); err != nil {
		return err
	}
	lease.State = state.LeaseTerminated
	_, err = s.State.SaveLease(ctx, lease, lease.Revision)
	return err
}

// createdForJobNumber extracts the job number from one of our runner names,
// "<prefix>-<job number>-<attempt>". It reports false for any other name, so a
// runner the controller did not create is never touched.
func (s *Service) createdForJobNumber(name string) (int64, bool) {
	prefix := s.RunnerNamePrefix
	if prefix == "" {
		prefix = "leo-runner"
	}
	rest, found := strings.CutPrefix(strings.TrimSpace(name), prefix+"-")
	if !found {
		return 0, false
	}
	parts := strings.Split(rest, "-")
	if len(parts) != 2 {
		return 0, false
	}
	number, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || number <= 0 {
		return 0, false
	}
	if attempt, err := strconv.Atoi(parts[1]); err != nil || attempt <= 0 {
		return 0, false
	}
	return number, true
}

// leaseOfRunnerCreatedFor finds the lease of the job a runner was created for.
// The runner usually serves a job of the same repository, so that key is tried
// first. Organization-level runners are shared by every repository in their
// group and can serve another repository's job, so it falls back to a search by
// the job number, which GitHub makes unique.
func (s *Service) leaseOfRunnerCreatedFor(ctx context.Context, repository string, jobNumber int64) (state.Lease, error) {
	lease, err := s.State.GetLease(ctx, fmt.Sprintf("lease-%s/job/%d", repository, jobNumber))
	if !errors.Is(err, state.ErrNotFound) {
		return lease, err
	}
	leases, listErr := s.State.ListLeases(ctx)
	if listErr != nil {
		return state.Lease{}, listErr
	}
	suffix := fmt.Sprintf("/job/%d", jobNumber)
	for _, candidate := range leases {
		if strings.HasSuffix(candidate.JobID, suffix) {
			return candidate, nil
		}
	}
	return state.Lease{}, state.ErrNotFound
}
