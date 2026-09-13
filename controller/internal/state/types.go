package state

import "time"

// JobState is the controller's durable view of a workflow job.
type JobState string

const (
	JobQueued       JobState = "queued"
	JobProvisioning JobState = "provisioning"
	JobAssigned     JobState = "assigned"
	JobRunning      JobState = "running"
	JobCompleted    JobState = "completed"
	JobCancelled    JobState = "cancelled"
	JobFailed       JobState = "failed"
)

// RunnerState describes an ephemeral runner independently of its job.
type RunnerState string

const (
	RunnerProvisioning RunnerState = "provisioning"
	RunnerReady        RunnerState = "ready"
	RunnerBusy         RunnerState = "busy"
	RunnerTerminating  RunnerState = "terminating"
	RunnerTerminated   RunnerState = "terminated"
	RunnerFailed       RunnerState = "failed"
	RunnerUnknown      RunnerState = "unknown"
)

// LeaseState is the ownership state for one job attempt and one runner.
type LeaseState string

const (
	LeasePending     LeaseState = "pending"
	LeaseActive      LeaseState = "active"
	LeaseCompleted   LeaseState = "completed"
	LeaseCancelled   LeaseState = "cancelled"
	LeaseFailed      LeaseState = "failed"
	LeaseTerminating LeaseState = "terminating"
	LeaseTerminated  LeaseState = "terminated"
)

// Job is the persisted identity and lifecycle record for one workflow attempt.
type Job struct {
	ID              string
	Repository      string
	Workflow        string
	RunID           int64
	JobID           int64
	Attempt         int
	Labels          []string
	State           JobState
	ControllerOwner string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ExpiresAt       time.Time
	Revision        int64
}

// Runner is the cloud-neutral persisted runner record.
type Runner struct {
	ID                 string
	JobID              string
	LeaseID            string
	Provider           string
	ProviderInstanceID string
	CapacityPoolID     string
	CPU                int
	MemoryGB           int
	GPU                int
	Region             string
	Labels             []string
	State              RunnerState
	ControllerOwner    string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ExpiresAt          time.Time
	Revision           int64
}

// Lease binds exactly one job attempt to exactly one ephemeral runner.
type Lease struct {
	ID              string
	JobID           string
	RunnerID        string
	State           LeaseState
	ControllerOwner string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ExpiresAt       time.Time
	Revision        int64
}

// LifecycleEvent is an immutable, idempotently inserted audit record.
type LifecycleEvent struct {
	ID             string
	IdempotencyKey string
	Type           string
	JobID          string
	LeaseID        string
	RunnerID       string
	Provider       string
	OccurredAt     time.Time
	Data           map[string]string
}
