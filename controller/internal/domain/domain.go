package domain

import (
	"errors"
	"time"
)

type JobState string

const (
	JobQueued       JobState = "queued"
	JobProvisioning JobState = "provisioning"
	JobReady        JobState = "ready"
	JobRunning      JobState = "running"
	JobCompleted    JobState = "completed"
	JobFailed       JobState = "failed"
	JobCancelled    JobState = "cancelled"
	JobTerminating  JobState = "terminating"
	JobTerminated   JobState = "terminated"
)

type Job struct {
	ID, Repository, Workflow, RunID, AttemptID string
	State                                      JobState
	CreatedAt, UpdatedAt                       time.Time
	ExpiresAt                                  time.Time
	Revision                                   uint64
}

type Runner struct {
	ID, JobID, Provider, ProviderInstanceID string
	State                                   JobState
	CreatedAt, UpdatedAt, ExpiresAt         time.Time
	Revision                                uint64
}

type Lease struct {
	ID, JobID, RunnerID string
	ExpiresAt           time.Time
	CreatedAt           time.Time
}

var ErrInvalidTransition = errors.New("invalid lifecycle transition")

func CanTransition(from, to JobState) bool {
	if from == to {
		return true
	}
	switch from {
	case JobQueued:
		return to == JobProvisioning || to == JobCancelled
	case JobProvisioning:
		return to == JobReady || to == JobFailed || to == JobCancelled || to == JobTerminating
	case JobReady:
		return to == JobRunning || to == JobFailed || to == JobCancelled || to == JobTerminating
	case JobRunning:
		return to == JobCompleted || to == JobFailed || to == JobCancelled || to == JobTerminating
	case JobCompleted, JobFailed, JobCancelled:
		return to == JobTerminating
	case JobTerminating:
		return to == JobTerminated || to == JobFailed
	case JobTerminated:
		return false
	default:
		return false
	}
}

func Transition(job *Job, to JobState, now time.Time) error {
	if !CanTransition(job.State, to) {
		return ErrInvalidTransition
	}
	job.State, job.UpdatedAt, job.Revision = to, now, job.Revision+1
	return nil
}
