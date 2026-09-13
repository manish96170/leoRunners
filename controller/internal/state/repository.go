package state

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("state record not found")
	ErrAlreadyExists    = errors.New("state record already exists")
	ErrRevisionConflict = errors.New("state revision conflict")
	ErrInvalidRecord    = errors.New("invalid state record")
	ErrEventConflict    = errors.New("event idempotency key conflicts with existing event")
)

// Repository is the persistence boundary used by reconciliation and reaping.
// Save methods are conditional: expectedRevision must be zero for creation, or
// equal to the stored revision for an update. They return the new record.
type Repository interface {
	CreateJob(context.Context, Job) error
	GetJob(context.Context, string) (Job, error)
	SaveJob(context.Context, Job, int64) (Job, error)
	ListJobs(context.Context, ...JobState) ([]Job, error)
	ListExpiredJobs(context.Context, time.Time) ([]Job, error)

	CreateRunner(context.Context, Runner) error
	GetRunner(context.Context, string) (Runner, error)
	SaveRunner(context.Context, Runner, int64) (Runner, error)
	ListRunners(context.Context, ...RunnerState) ([]Runner, error)
	ListExpiredRunners(context.Context, time.Time) ([]Runner, error)

	CreateLease(context.Context, Lease) error
	GetLease(context.Context, string) (Lease, error)
	SaveLease(context.Context, Lease, int64) (Lease, error)
	ListLeases(context.Context, ...LeaseState) ([]Lease, error)
	ListExpiredLeases(context.Context, time.Time) ([]Lease, error)

	InsertLifecycleEvent(context.Context, LifecycleEvent) (inserted bool, err error)
	DeleteLifecycleEvent(context.Context, string) error
	ListLifecycleEvents(context.Context, string, time.Time) ([]LifecycleEvent, error)
}
