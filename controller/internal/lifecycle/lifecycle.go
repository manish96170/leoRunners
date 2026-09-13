package lifecycle

import (
	"context"
	"fmt"
	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/state"
	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
	"time"
)

type Manager struct {
	State    state.Repository
	Provider providers.Provider
	Sink     telemetry.Sink
}

func (m *Manager) Transition(ctx context.Context, job *state.Job, to state.JobState) error {
	if job == nil || !validTransition(job.State, to) {
		return fmt.Errorf("invalid lifecycle transition")
	}
	previous := job.Revision
	job.State = to
	updated, err := m.State.SaveJob(ctx, *job, previous)
	if err != nil {
		return err
	}
	*job = updated
	if m.Sink != nil {
		_ = m.Sink.Emit(telemetry.Event{ID: fmt.Sprintf("%s-%d", job.ID, job.Revision), Type: string(to), JobID: job.ID, At: time.Now().UTC()})
	}
	return nil
}

func (m *Manager) Reap(ctx context.Context, now time.Time) error {
	_, err := (&Reconciler{State: m.State, Provider: m.Provider}).ReapExpired(ctx, now)
	return err
}

func validTransition(from, to state.JobState) bool {
	if from == to {
		return true
	}
	switch from {
	case state.JobQueued:
		return to == state.JobProvisioning || to == state.JobCancelled || to == state.JobCompleted || to == state.JobFailed
	case state.JobProvisioning:
		return to == state.JobAssigned || to == state.JobFailed || to == state.JobCancelled || to == state.JobCompleted
	case state.JobAssigned:
		return to == state.JobRunning || to == state.JobFailed || to == state.JobCancelled || to == state.JobCompleted
	case state.JobRunning:
		return to == state.JobCompleted || to == state.JobFailed || to == state.JobCancelled
	default:
		return false
	}
}
