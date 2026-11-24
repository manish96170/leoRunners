package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/lifecycle"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

// The runner wait takes minutes and the reconciler runs during it, so the job
// record the controller holds when the wait ends can be stale.

func jobIn(t *testing.T, repo *state.MemoryRepository, id string, s state.JobState) {
	t.Helper()
	job := state.Job{ID: id, Repository: "acme/repo", JobID: 1, State: state.JobQueued, ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.CreateJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetJob(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if s != state.JobQueued {
		stored.State = s
		if _, err := repo.SaveJob(context.Background(), stored, stored.Revision); err != nil {
			t.Fatal(err)
		}
	}
}

func settleService(repo state.Repository) *Service {
	return &Service{State: repo, Lifecycle: &lifecycle.Manager{State: repo}}
}

func TestSettleAssignedMovesAProvisioningJobToAssigned(t *testing.T) {
	repo := state.NewMemoryRepository()
	jobIn(t, repo, "job-1", state.JobProvisioning)
	if err := settleService(repo).settleAssigned(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	if job, _ := repo.GetJob(context.Background(), "job-1"); job.State != state.JobAssigned {
		t.Fatalf("job state = %s, want assigned", job.State)
	}
}

func TestSettleAssignedLeavesAJobThatMovedOnAlone(t *testing.T) {
	for _, past := range []state.JobState{state.JobAssigned, state.JobRunning, state.JobCompleted, state.JobCancelled, state.JobFailed} {
		t.Run(string(past), func(t *testing.T) {
			repo := state.NewMemoryRepository()
			jobIn(t, repo, "job-1", past)
			if err := settleService(repo).settleAssigned(context.Background(), "job-1"); err != nil {
				t.Fatalf("settleAssigned() error = %v, want nil", err)
			}
			if job, _ := repo.GetJob(context.Background(), "job-1"); job.State != past {
				t.Fatalf("job state = %s, want it left at %s", job.State, past)
			}
		})
	}
}

// touchOnce saves a competing write just before the first SaveJob, so that call
// finds a stale revision, as it would if the reconciler had written meanwhile.
type touchOnce struct {
	state.Repository
	touched bool
}

func (r *touchOnce) SaveJob(ctx context.Context, job state.Job, expected int64) (state.Job, error) {
	if !r.touched {
		r.touched = true
		current, err := r.Repository.GetJob(ctx, job.ID)
		if err != nil {
			return state.Job{}, err
		}
		if _, err := r.Repository.SaveJob(ctx, current, current.Revision); err != nil {
			return state.Job{}, err
		}
	}
	return r.Repository.SaveJob(ctx, job, expected)
}

func TestSettleAssignedRetriesAfterARevisionConflict(t *testing.T) {
	inner := state.NewMemoryRepository()
	jobIn(t, inner, "job-1", state.JobProvisioning)
	repo := &touchOnce{Repository: inner}
	if err := settleService(repo).settleAssigned(context.Background(), "job-1"); err != nil {
		t.Fatalf("settleAssigned() error = %v, want it to retry", err)
	}
	if !repo.touched {
		t.Fatal("test setup: the competing write never happened")
	}
	if job, _ := inner.GetJob(context.Background(), "job-1"); job.State != state.JobAssigned {
		t.Fatalf("job state = %s, want assigned", job.State)
	}
}

type alwaysConflicts struct{ state.Repository }

func (alwaysConflicts) SaveJob(context.Context, state.Job, int64) (state.Job, error) {
	return state.Job{}, state.ErrRevisionConflict
}

func TestSettleAssignedGivesUpAfterRepeatedConflicts(t *testing.T) {
	inner := state.NewMemoryRepository()
	jobIn(t, inner, "job-1", state.JobProvisioning)
	err := settleService(alwaysConflicts{inner}).settleAssigned(context.Background(), "job-1")
	if !errors.Is(err, state.ErrRevisionConflict) {
		t.Fatalf("error = %v, want a revision conflict after bounded retries", err)
	}
}

// A job can finish while its runner is still being created, before any lease
// exists. That is not an error, and the job must still be marked finished.
func TestFinishWithoutALeaseMarksTheJobAndSucceeds(t *testing.T) {
	repo := state.NewMemoryRepository()
	jobIn(t, repo, "job-1", state.JobProvisioning)
	svc := settleService(repo)
	if err := svc.finish(context.Background(), "job-1", state.JobCancelled); err != nil {
		t.Fatalf("finish() error = %v, want nil when no lease exists yet", err)
	}
	if job, _ := repo.GetJob(context.Background(), "job-1"); job.State != state.JobCancelled {
		t.Fatalf("job state = %s, want cancelled", job.State)
	}
}

func TestFinishStillReportsAMissingJob(t *testing.T) {
	svc := settleService(state.NewMemoryRepository())
	if err := svc.finish(context.Background(), "nope", state.JobCompleted); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestLateEndEventForAnAlreadyFailedJobIsNotAnError(t *testing.T) {
	for _, final := range []state.JobState{state.JobCompleted, state.JobCancelled} {
		t.Run(string(final), func(t *testing.T) {
			repo := state.NewMemoryRepository()
			jobIn(t, repo, "job-1", state.JobFailed)
			if err := settleService(repo).finish(context.Background(), "job-1", final); err != nil {
				t.Fatalf("finish() error = %v, want nil for a job that already failed", err)
			}
			if job, _ := repo.GetJob(context.Background(), "job-1"); job.State != state.JobFailed {
				t.Fatalf("job state = %s, a failed job must stay failed", job.State)
			}
		})
	}
}
