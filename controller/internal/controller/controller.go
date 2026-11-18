package controller

import (
	"context"
	"fmt"
	"github.com/leo-runners/ci-platform/controller/internal/capacity"
	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/lifecycle"
	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/runners"
	"github.com/leo-runners/ci-platform/controller/internal/scheduler"
	"github.com/leo-runners/ci-platform/controller/internal/state"
	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
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
		return s.Lifecycle.Transition(ctx, &job, state.JobAssigned)
	case github.ActionInProgress:
		stored, err := s.State.GetJob(ctx, event.JobKey)
		if err != nil {
			return err
		}
		return s.Lifecycle.Transition(ctx, &stored, state.JobRunning)
	case github.ActionCancelled:
		return s.finish(ctx, event.JobKey, state.JobCancelled)
	case github.ActionCompleted:
		conclusion := state.JobCompleted
		if event.Job.Conclusion == "cancelled" || event.Job.Conclusion == "skipped" {
			conclusion = state.JobCancelled
		} else if event.Job.Conclusion != "success" {
			conclusion = state.JobFailed
		}
		return s.finish(ctx, event.JobKey, conclusion)
	default:
		return nil
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Service) finish(ctx context.Context, jobID string, final state.JobState) error {
	job, err := s.State.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.State != final {
		if err := s.Lifecycle.Transition(ctx, &job, final); err != nil {
			return err
		}
	}
	lease, err := s.State.GetLease(ctx, "lease-"+jobID)
	if err != nil {
		return err
	}
	runner, err := s.State.GetRunner(ctx, lease.RunnerID)
	if err != nil {
		return err
	}
	provider := s.Provider
	if s.Providers != nil {
		var ok bool
		provider, ok = s.Providers[runner.Provider]
		if !ok || provider == nil {
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
