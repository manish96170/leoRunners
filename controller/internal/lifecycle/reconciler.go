package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

const (
	defaultOperationTimeout = 30 * time.Second
)

// Reconciler compares durable lifecycle records with provider truth and
// repairs only transitions that are safe to infer. It is intentionally
// stateless so a new process can run it after a controller restart.
type Reconciler struct {
	State            state.Repository
	Provider         providers.Provider
	OperationTimeout time.Duration
	MaxRecords       int
	Now              func() time.Time
}

// Report describes one reconciliation pass. Errors are also returned joined
// so callers can alert while still benefiting from independent repairs.
type Report struct {
	JobsScanned       int
	RunnersScanned    int
	LeasesScanned     int
	RunnersRepaired   int
	JobsRepaired      int
	LeasesRepaired    int
	RunnersReaped     int
	ProviderErrors    int
	PersistenceErrors int
	Errors            []error
}

// Reconcile performs one bounded pass over all durable records. A provider or
// record failure does not prevent unrelated records from being reconciled.
func (r *Reconciler) Reconcile(ctx context.Context, now time.Time) (Report, error) {
	var report Report
	if r.State == nil || r.Provider == nil {
		return report, errors.New("reconciler dependencies are not configured")
	}
	if err := contextErr(ctx); err != nil {
		return report, err
	}
	if now.IsZero() {
		now = r.NowTime()
	}

	listCtx, cancel := r.operationContext(ctx)
	jobs, err := r.State.ListJobs(listCtx)
	cancel()
	if err != nil {
		return report, fmt.Errorf("list jobs: %w", err)
	}
	listCtx, cancel = r.operationContext(ctx)
	runners, err := r.State.ListRunners(listCtx)
	cancel()
	if err != nil {
		return report, fmt.Errorf("list runners: %w", err)
	}
	listCtx, cancel = r.operationContext(ctx)
	leases, err := r.State.ListLeases(listCtx)
	cancel()
	if err != nil {
		return report, fmt.Errorf("list leases: %w", err)
	}
	jobs = limitJobs(jobs, r.maxRecords())
	runners = limitRunners(runners, r.maxRecords())
	leases = limitLeases(leases, r.maxRecords())
	report.JobsScanned, report.RunnersScanned, report.LeasesScanned = len(jobs), len(runners), len(leases)

	jobByID := make(map[string]state.Job, len(jobs))
	for _, job := range jobs {
		jobByID[job.ID] = job
	}
	runnerByID := make(map[string]state.Runner, len(runners))
	for _, runner := range runners {
		runnerByID[runner.ID] = runner
	}
	leaseByJob := make(map[string]state.Lease, len(leases))
	for _, lease := range leases {
		leaseByJob[lease.JobID] = lease
	}

	// Provider truth is authoritative for non-terminal runner records.
	runnerIDs := make([]string, 0, len(runnerByID))
	for id := range runnerByID {
		runnerIDs = append(runnerIDs, id)
	}
	sort.Strings(runnerIDs)
	for _, id := range runnerIDs {
		runner := runnerByID[id]
		if runner.State == state.RunnerTerminated {
			continue
		}
		status, statusErr := r.providerStatus(ctx, runner)
		if statusErr != nil {
			if errors.Is(statusErr, providers.ErrInstanceNotFound) {
				status = providers.StatusTerminated
			} else {
				report.ProviderErrors++
				report.addError(fmt.Errorf("status runner %s: %w", id, statusErr))
				continue
			}
		}
		target := runnerState(status)
		if target == runner.State {
			continue
		}
		runner.State = target
		updated, saveErr := r.saveRunner(ctx, runner)
		if saveErr != nil {
			report.PersistenceErrors++
			report.addError(fmt.Errorf("save runner %s: %w", id, saveErr))
			continue
		}
		runnerByID[id] = updated
		report.RunnersRepaired++
	}

	// Repair lease ownership and job state from the now-current runner view.
	for id, lease := range leaseByJob {
		job, jobExists := jobByID[lease.JobID]
		runner, runnerExists := runnerByID[lease.RunnerID]
		target := lease.State
		switch {
		case !jobExists || !runnerExists:
			if target != state.LeaseTerminated && target != state.LeaseFailed {
				target = state.LeaseFailed
			}
		case !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(now):
			target = state.LeaseTerminated
		case isTerminalJob(job.State):
			target = state.LeaseTerminated
		case runner.State == state.RunnerTerminated:
			target = state.LeaseTerminated
		case runner.State == state.RunnerFailed || runner.State == state.RunnerUnknown:
			target = state.LeaseFailed
		case target == state.LeasePending:
			target = state.LeaseActive
		}
		if target != lease.State {
			lease.State = target
			updated, saveErr := r.saveLease(ctx, lease)
			if saveErr != nil {
				report.PersistenceErrors++
				report.addError(fmt.Errorf("save lease %s: %w", id, saveErr))
			} else {
				leaseByJob[id] = updated
				report.LeasesRepaired++
			}
		}
	}

	for id, job := range jobByID {
		lease, hasLease := leaseByJob[id]
		target := job.State
		if !isTerminalJob(job.State) && !job.ExpiresAt.IsZero() && !job.ExpiresAt.After(now) {
			target = state.JobFailed
		}
		if target != job.State {
			job.State = target
			updated, saveErr := r.saveJob(ctx, job)
			if saveErr != nil {
				report.PersistenceErrors++
				report.addError(fmt.Errorf("save expired job %s: %w", id, saveErr))
				continue
			}
			job = updated
			jobByID[id] = updated
			report.JobsRepaired++
		}
		if isTerminalJob(job.State) {
			// Terminal jobs must not retain live provider resources. Cleanup is
			// bounded and retried on the next pass if it fails.
			var runner state.Runner
			var hasRunner bool
			if hasLease {
				runner, hasRunner = runnerByID[lease.RunnerID]
			} else {
				for _, candidate := range runnerByID {
					if candidate.JobID == id {
						runner, hasRunner = candidate, true
						break
					}
				}
			}
			if hasRunner && runner.State != state.RunnerTerminated {
				cleaned, cleanErr := r.terminateRunner(ctx, runner)
				if cleanErr != nil {
					report.ProviderErrors++
					report.addError(fmt.Errorf("cleanup terminal job %s runner %s: %w", id, runner.ID, cleanErr))
				} else {
					runnerByID[runner.ID] = cleaned
				}
			}
			continue
		}
		if !hasLease && (job.State == state.JobAssigned || job.State == state.JobRunning) {
			target = state.JobFailed
		} else if hasLease {
			runner, ok := runnerByID[lease.RunnerID]
			if lease.State == state.LeaseTerminated || lease.State == state.LeaseFailed || lease.State == state.LeaseCancelled || lease.State == state.LeaseCompleted {
				target = state.JobFailed
			} else if !ok || runner.State == state.RunnerTerminated || runner.State == state.RunnerFailed || runner.State == state.RunnerUnknown {
				target = state.JobFailed
			} else if runner.State == state.RunnerBusy && (job.State == state.JobQueued || job.State == state.JobProvisioning || job.State == state.JobAssigned) {
				target = state.JobRunning
			} else if runner.State == state.RunnerReady && (job.State == state.JobQueued || job.State == state.JobProvisioning) {
				target = state.JobAssigned
			}
		}
		if target != job.State {
			job.State = target
			updated, saveErr := r.saveJob(ctx, job)
			if saveErr != nil {
				report.PersistenceErrors++
				report.addError(fmt.Errorf("save job %s: %w", id, saveErr))
			} else {
				jobByID[id] = updated
				report.JobsRepaired++
			}
		}
	}

	// A crash can leave a provider runner after both its lease and job record
	// have been removed. The durable JobID is sufficient to identify this as
	// an orphan; cleanup remains idempotent and bounded.
	for id, runner := range runnerByID {
		if runner.JobID == "" || runner.State == state.RunnerTerminated {
			continue
		}
		if _, exists := jobByID[runner.JobID]; exists {
			continue
		}
		cleaned, cleanErr := r.terminateRunner(ctx, runner)
		if cleanErr != nil {
			report.ProviderErrors++
			report.addError(fmt.Errorf("cleanup orphan runner %s: %w", id, cleanErr))
		} else {
			runnerByID[id] = cleaned
		}
	}

	_, reapErr := r.reapExpired(ctx, now, &report, runnerByID)
	_ = reapErr
	return report, report.err()
}

// ReapExpired is the narrow compatibility operation used by the existing
// Manager. It marks each expired runner terminating before attempting cleanup,
// and marks it terminated only after the provider confirms termination.
func (r *Reconciler) ReapExpired(ctx context.Context, now time.Time) (Report, error) {
	var report Report
	if r.State == nil || r.Provider == nil {
		return report, errors.New("reaper dependencies are not configured")
	}
	listCtx, cancel := r.operationContext(ctx)
	runners, err := r.State.ListExpiredRunners(listCtx, now)
	cancel()
	if err != nil {
		return report, fmt.Errorf("list expired runners: %w", err)
	}
	runners = limitRunners(runners, r.maxRecords())
	report.RunnersScanned = len(runners)
	byID := make(map[string]state.Runner, len(runners))
	for _, runner := range runners {
		byID[runner.ID] = runner
	}
	_, err = r.reapExpired(ctx, now, &report, byID)
	_ = err
	return report, report.err()
}

func (r *Reconciler) reapExpired(ctx context.Context, now time.Time, report *Report, runners map[string]state.Runner) (map[string]state.Runner, error) {
	var errs []error
	for id, runner := range runners {
		if err := contextErr(ctx); err != nil {
			return runners, errors.Join(append(errs, err)...)
		}
		if runner.ExpiresAt.IsZero() || runner.ExpiresAt.After(now) || runner.State == state.RunnerTerminated {
			continue
		}
		if runner.State != state.RunnerTerminating {
			runner.State = state.RunnerTerminating
			updated, err := r.saveRunner(ctx, runner)
			if err != nil {
				report.PersistenceErrors++
				err = fmt.Errorf("mark expired runner %s terminating: %w", id, err)
				report.addError(err)
				errs = append(errs, err)
				continue
			}
			runner = updated
			runners[id] = runner
			report.RunnersRepaired++
		}
		if err := r.terminate(ctx, runner); err != nil {
			report.ProviderErrors++
			err = fmt.Errorf("terminate expired runner %s: %w", id, err)
			report.addError(err)
			errs = append(errs, err)
			continue
		}
		runner.State = state.RunnerTerminated
		updated, err := r.saveRunner(ctx, runner)
		if err != nil {
			report.PersistenceErrors++
			err = fmt.Errorf("mark expired runner %s terminated: %w", id, err)
			report.addError(err)
			errs = append(errs, err)
			continue
		}
		runners[id] = updated
		report.RunnersReaped++
	}
	return runners, errors.Join(errs...)
}

func (r *Reconciler) terminateRunner(ctx context.Context, runner state.Runner) (state.Runner, error) {
	if runner.State != state.RunnerTerminating {
		runner.State = state.RunnerTerminating
		updated, err := r.saveRunner(ctx, runner)
		if err != nil {
			return runner, err
		}
		runner = updated
	}
	if err := r.terminate(ctx, runner); err != nil {
		return runner, err
	}
	runner.State = state.RunnerTerminated
	return r.saveRunner(ctx, runner)
}

func (r *Reconciler) providerStatus(ctx context.Context, runner state.Runner) (providers.RunnerStatus, error) {
	callCtx, cancel := r.operationContext(ctx)
	defer cancel()
	return r.Provider.Status(callCtx, providers.RunnerInstance{ID: runner.ID, ProviderID: runner.ProviderInstanceID, Provider: runner.Provider})
}

func (r *Reconciler) terminate(ctx context.Context, runner state.Runner) error {
	callCtx, cancel := r.operationContext(ctx)
	defer cancel()
	return r.Provider.Terminate(callCtx, providers.RunnerInstance{ID: runner.ID, ProviderID: runner.ProviderInstanceID, Provider: runner.Provider})
}

func (r *Reconciler) saveRunner(ctx context.Context, runner state.Runner) (state.Runner, error) {
	callCtx, cancel := r.operationContext(ctx)
	defer cancel()
	return r.State.SaveRunner(callCtx, runner, runner.Revision)
}

func (r *Reconciler) saveJob(ctx context.Context, job state.Job) (state.Job, error) {
	callCtx, cancel := r.operationContext(ctx)
	defer cancel()
	return r.State.SaveJob(callCtx, job, job.Revision)
}

func (r *Reconciler) saveLease(ctx context.Context, lease state.Lease) (state.Lease, error) {
	callCtx, cancel := r.operationContext(ctx)
	defer cancel()
	return r.State.SaveLease(callCtx, lease, lease.Revision)
}

func (r *Reconciler) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, r.operationTimeout())
}

func (r *Reconciler) operationTimeout() time.Duration {
	if r.OperationTimeout > 0 {
		return r.OperationTimeout
	}
	return defaultOperationTimeout
}

func (r *Reconciler) maxRecords() int {
	return r.MaxRecords
}

func (r *Report) addError(err error) {
	if err != nil {
		r.Errors = append(r.Errors, err)
	}
}

func (r *Reconciler) NowTime() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func runnerState(status providers.RunnerStatus) state.RunnerState {
	switch status {
	case providers.StatusProvisioning:
		return state.RunnerProvisioning
	case providers.StatusReady:
		return state.RunnerReady
	case providers.StatusBusy:
		return state.RunnerBusy
	case providers.StatusTerminating:
		return state.RunnerTerminating
	case providers.StatusTerminated:
		return state.RunnerTerminated
	case providers.StatusFailed:
		return state.RunnerFailed
	default:
		return state.RunnerUnknown
	}
}

func isTerminalJob(s state.JobState) bool {
	return s == state.JobCompleted || s == state.JobCancelled || s == state.JobFailed
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (r Report) err() error { return errors.Join(r.Errors...) }

func limitJobs(v []state.Job, n int) []state.Job {
	if n > 0 && len(v) > n {
		return v[:n]
	}
	return v
}
func limitRunners(v []state.Runner, n int) []state.Runner {
	if n > 0 && len(v) > n {
		return v[:n]
	}
	return v
}
func limitLeases(v []state.Lease, n int) []state.Lease {
	if n > 0 && len(v) > n {
		return v[:n]
	}
	return v
}
