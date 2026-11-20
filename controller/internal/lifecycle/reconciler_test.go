package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

func TestReconcileRepairsProviderRunnerLeaseAndJobState(t *testing.T) {
	ctx := context.Background()
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(1000, 0).UTC()
	instance, err := provider.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SetStatus(instance.ID, providers.StatusReady); err != nil {
		t.Fatal(err)
	}
	job := state.Job{ID: "job-1", State: state.JobProvisioning, ExpiresAt: now.Add(time.Hour)}
	runner := state.Runner{ID: instance.ID, JobID: job.ID, Provider: "fake", ProviderInstanceID: instance.ProviderID, State: state.RunnerProvisioning, ExpiresAt: job.ExpiresAt}
	lease := state.Lease{ID: "lease-1", JobID: job.ID, RunnerID: runner.ID, State: state.LeasePending, ExpiresAt: job.ExpiresAt}
	createRecords(t, repo, job, runner, lease)

	report, err := (&Reconciler{State: repo, Provider: provider}).Reconcile(ctx, now)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.RunnersRepaired != 1 || report.LeasesRepaired != 1 || report.JobsRepaired != 1 {
		t.Fatalf("report = %#v", report)
	}
	assertRunnerState(t, repo, runner.ID, state.RunnerReady)
	assertLeaseState(t, repo, lease.ID, state.LeaseActive)
	assertJobState(t, repo, job.ID, state.JobAssigned)

	if err := provider.SetStatus(instance.ID, providers.StatusBusy); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Reconciler{State: repo, Provider: provider}).Reconcile(ctx, now); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	assertRunnerState(t, repo, runner.ID, state.RunnerBusy)
	assertJobState(t, repo, job.ID, state.JobRunning)
}

func TestReconcileTreatsMissingProviderInstanceAsTerminated(t *testing.T) {
	ctx := context.Background()
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(2000, 0).UTC()
	job := state.Job{ID: "job-missing", State: state.JobAssigned, ExpiresAt: now.Add(time.Hour)}
	runner := state.Runner{ID: "runner-missing", JobID: job.ID, Provider: "fake", ProviderInstanceID: "runner-missing", State: state.RunnerReady, ExpiresAt: job.ExpiresAt}
	lease := state.Lease{ID: "lease-missing", JobID: job.ID, RunnerID: runner.ID, State: state.LeaseActive, ExpiresAt: job.ExpiresAt}
	createRecords(t, repo, job, runner, lease)

	report, err := (&Reconciler{State: repo, Provider: provider}).Reconcile(ctx, now)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.RunnersRepaired != 1 || report.LeasesRepaired != 1 || report.JobsRepaired != 1 {
		t.Fatalf("report = %#v", report)
	}
	assertRunnerState(t, repo, runner.ID, state.RunnerTerminated)
	assertLeaseState(t, repo, lease.ID, state.LeaseTerminated)
	assertJobState(t, repo, job.ID, state.JobFailed)
}

func TestReconcileExpiresJobAndLeaseBeforeCleaningRunner(t *testing.T) {
	ctx := context.Background()
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(2500, 0).UTC()
	instance, err := provider.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	job := state.Job{ID: "job-expired", State: state.JobRunning, ExpiresAt: now.Add(-time.Minute)}
	runner := state.Runner{ID: instance.ID, JobID: job.ID, Provider: "fake", ProviderInstanceID: instance.ProviderID, State: state.RunnerReady, ExpiresAt: now.Add(time.Hour)}
	lease := state.Lease{ID: "lease-expired", JobID: job.ID, RunnerID: runner.ID, State: state.LeaseActive, ExpiresAt: now.Add(-time.Minute)}
	createRecords(t, repo, job, runner, lease)

	if _, err := (&Reconciler{State: repo, Provider: provider}).Reconcile(ctx, now); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	assertJobState(t, repo, job.ID, state.JobFailed)
	assertLeaseState(t, repo, lease.ID, state.LeaseTerminated)
	assertRunnerState(t, repo, runner.ID, state.RunnerTerminated)
	if provider.Active() != 0 {
		t.Fatalf("active provider instances = %d", provider.Active())
	}
}

func TestReconcileCleansRunnerWhoseJobRecordIsMissing(t *testing.T) {
	ctx := context.Background()
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(2750, 0).UTC()
	instance, err := provider.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	runner := state.Runner{ID: instance.ID, JobID: "deleted-job", Provider: "fake", ProviderInstanceID: instance.ProviderID, State: state.RunnerReady, ExpiresAt: now.Add(time.Hour)}
	if err := repo.CreateRunner(ctx, runner); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Reconciler{State: repo, Provider: provider}).Reconcile(ctx, now); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	assertRunnerState(t, repo, runner.ID, state.RunnerTerminated)
	if provider.Active() != 0 {
		t.Fatalf("active provider instances = %d", provider.Active())
	}
}

func TestReapExpiredIsIdempotentAndMarksTerminatingOnFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(3000, 0).UTC()

	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	instance, err := provider.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	runner := state.Runner{ID: instance.ID, Provider: "fake", ProviderInstanceID: instance.ProviderID, State: state.RunnerReady, ExpiresAt: now.Add(-time.Minute)}
	if err := repo.CreateRunner(ctx, runner); err != nil {
		t.Fatal(err)
	}
	reaper := &Reconciler{State: repo, Provider: provider}
	report, err := reaper.ReapExpired(ctx, now)
	if err != nil || report.RunnersReaped != 1 || provider.Active() != 0 {
		t.Fatalf("first reap report=%#v err=%v active=%d", report, err, provider.Active())
	}
	report, err = reaper.ReapExpired(ctx, now)
	if err != nil || report.RunnersScanned != 0 || report.RunnersReaped != 0 {
		t.Fatalf("second reap report=%#v err=%v", report, err)
	}

	failureRepo := state.NewMemoryRepository()
	failingProvider := providers.NewFakeProvider(providers.FakeConfig{Failures: providers.FailurePlan{Terminate: 1, Err: errors.New("terminate unavailable")}})
	failingInstance, err := failingProvider.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	failingRunner := state.Runner{ID: failingInstance.ID, Provider: "fake", ProviderInstanceID: failingInstance.ProviderID, State: state.RunnerReady, ExpiresAt: now.Add(-time.Minute)}
	if err := failureRepo.CreateRunner(ctx, failingRunner); err != nil {
		t.Fatal(err)
	}
	report, err = (&Reconciler{State: failureRepo, Provider: failingProvider}).ReapExpired(ctx, now)
	if err == nil || report.RunnersReaped != 0 {
		t.Fatalf("failed reap report=%#v err=%v", report, err)
	}
	assertRunnerState(t, failureRepo, failingRunner.ID, state.RunnerTerminating)
}

func TestReconcileContinuesAfterProviderStatusFailure(t *testing.T) {
	ctx := context.Background()
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{Failures: providers.FailurePlan{Status: 1, Err: errors.New("temporary status failure")}})
	now := time.Unix(4000, 0).UTC()
	first, err := provider.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SetStatus(second.ID, providers.StatusReady); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRunner(ctx, state.Runner{ID: first.ID, Provider: "fake", ProviderInstanceID: first.ProviderID, State: state.RunnerProvisioning, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRunner(ctx, state.Runner{ID: second.ID, Provider: "fake", ProviderInstanceID: second.ProviderID, State: state.RunnerProvisioning, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	report, err := (&Reconciler{State: repo, Provider: provider}).Reconcile(ctx, now)
	if err == nil || report.ProviderErrors != 1 || report.RunnersRepaired != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	assertRunnerState(t, repo, second.ID, state.RunnerReady)
	assertRunnerState(t, repo, first.ID, state.RunnerProvisioning)
}

func TestReapExpiredUsesBoundedTerminationContext(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(5000, 0).UTC()
	repo := state.NewMemoryRepository()
	base := providers.NewFakeProvider(providers.FakeConfig{})
	instance, err := base.Provision(ctx, providers.RunnerSpec{ExpiresAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	runner := state.Runner{ID: instance.ID, Provider: "fake", ProviderInstanceID: instance.ProviderID, State: state.RunnerReady, ExpiresAt: now.Add(-time.Minute)}
	if err := repo.CreateRunner(ctx, runner); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	provider := &blockingTerminateProvider{Provider: base, Started: started}
	startedAt := time.Now()
	report, err := (&Reconciler{State: repo, Provider: provider, OperationTimeout: 20 * time.Millisecond}).ReapExpired(ctx, now)
	if err == nil || report.RunnersReaped != 0 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	select {
	case <-started:
	default:
		t.Fatal("termination was not attempted")
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("bounded termination took %s", elapsed)
	}
	assertRunnerState(t, repo, runner.ID, state.RunnerTerminating)
}

type blockingTerminateProvider struct {
	providers.Provider
	Started chan<- struct{}
}

func (p *blockingTerminateProvider) Terminate(ctx context.Context, instance providers.RunnerInstance) error {
	select {
	case p.Started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func createRecords(t *testing.T, repo *state.MemoryRepository, job state.Job, runner state.Runner, lease state.Lease) {
	t.Helper()
	if err := repo.CreateJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRunner(context.Background(), runner); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateLease(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
}

func assertRunnerState(t *testing.T, repo *state.MemoryRepository, id string, want state.RunnerState) {
	t.Helper()
	runner, err := repo.GetRunner(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if runner.State != want {
		t.Fatalf("runner %s state=%s, want %s", id, runner.State, want)
	}
}

func assertLeaseState(t *testing.T, repo *state.MemoryRepository, id string, want state.LeaseState) {
	t.Helper()
	lease, err := repo.GetLease(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if lease.State != want {
		t.Fatalf("lease %s state=%s, want %s", id, lease.State, want)
	}
}

func assertJobState(t *testing.T, repo *state.MemoryRepository, id string, want state.JobState) {
	t.Helper()
	job, err := repo.GetJob(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != want {
		t.Fatalf("job %s state=%s, want %s", id, job.State, want)
	}
}

// ---- provider-side backstop ------------------------------------------------

func provisionWithExpiry(t *testing.T, provider *providers.FakeProvider, expires time.Time) providers.RunnerInstance {
	t.Helper()
	instance, err := provider.Provision(context.Background(), providers.RunnerSpec{ExpiresAt: expires})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SetStatus(instance.ID, providers.StatusReady); err != nil {
		t.Fatal(err)
	}
	return instance
}

func providerStatus(t *testing.T, provider *providers.FakeProvider, instance providers.RunnerInstance) providers.RunnerStatus {
	t.Helper()
	status, err := provider.Status(context.Background(), instance)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// The state-driven reaper cannot remove what it has no record of. Records are
// lost to crashes, table wipes and retention policies, so an instance past its
// own expiry must still be removed.
func TestReconcileReapsAnExpiredInstanceThatHasNoRecordAtAll(t *testing.T) {
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(100000, 0).UTC()
	orphan := provisionWithExpiry(t, provider, now.Add(-time.Hour))

	report, err := (&Reconciler{State: state.NewMemoryRepository(), Provider: provider}).Reconcile(context.Background(), now)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.OrphansReaped != 1 {
		t.Fatalf("OrphansReaped = %d, want 1", report.OrphansReaped)
	}
	if got := providerStatus(t, provider, orphan); got != providers.StatusTerminated {
		t.Fatalf("orphan status = %s, want terminated", got)
	}
}

func TestBackstopNeverRemovesWhatItShouldNot(t *testing.T) {
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(100000, 0).UTC()
	withinGrace := provisionWithExpiry(t, provider, now.Add(-time.Minute))       // expired, but the normal reaper owns it
	notExpired := provisionWithExpiry(t, provider, now.Add(time.Hour))           // still valid
	noMarker := provisionWithExpiry(t, provider, time.Time{})                    // expiry unknown: never grounds for removal
	justAfterGrace := provisionWithExpiry(t, provider, now.Add(-11*time.Minute)) // past the default 10 minute grace

	report, err := (&Reconciler{State: state.NewMemoryRepository(), Provider: provider}).Reconcile(context.Background(), now)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.OrphansReaped != 1 {
		t.Fatalf("OrphansReaped = %d, want exactly the one past the grace period", report.OrphansReaped)
	}
	for name, instance := range map[string]providers.RunnerInstance{"within grace": withinGrace, "not expired": notExpired, "no expiry marker": noMarker} {
		if got := providerStatus(t, provider, instance); got == providers.StatusTerminated {
			t.Errorf("%s instance was terminated", name)
		}
	}
	if got := providerStatus(t, provider, justAfterGrace); got != providers.StatusTerminated {
		t.Errorf("instance past the grace period status = %s, want terminated", got)
	}
}

func TestBackstopGraceIsConfigurable(t *testing.T) {
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(100000, 0).UTC()
	instance := provisionWithExpiry(t, provider, now.Add(-2*time.Minute))
	report, err := (&Reconciler{State: state.NewMemoryRepository(), Provider: provider, OrphanGrace: time.Minute}).Reconcile(context.Background(), now)
	if err != nil || report.OrphansReaped != 1 {
		t.Fatalf("report = %+v, error = %v", report, err)
	}
	if providerStatus(t, provider, instance) != providers.StatusTerminated {
		t.Fatal("instance past a one minute grace was not terminated")
	}
}

func TestBackstopIsIdempotentAndDoesNotDoubleCountWithTheStateReaper(t *testing.T) {
	ctx := context.Background()
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(100000, 0).UTC()
	instance := provisionWithExpiry(t, provider, now.Add(-time.Hour))
	job := state.Job{ID: "job-1", State: state.JobAssigned, ExpiresAt: now.Add(-time.Hour)}
	runner := state.Runner{ID: instance.ID, JobID: job.ID, Provider: "fake", ProviderInstanceID: instance.ProviderID, State: state.RunnerReady, ExpiresAt: job.ExpiresAt}
	createRecords(t, repo, job, runner, state.Lease{ID: "lease-1", JobID: job.ID, RunnerID: runner.ID, State: state.LeaseActive, ExpiresAt: job.ExpiresAt})

	reconciler := &Reconciler{State: repo, Provider: provider}
	first, err := reconciler.Reconcile(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if providerStatus(t, provider, instance) != providers.StatusTerminated {
		t.Fatalf("expired runner was not removed: %+v", first)
	}
	if first.OrphansReaped != 0 {
		t.Fatalf("a runner with a record was counted as an orphan: %+v", first)
	}
	second, err := reconciler.Reconcile(ctx, now)
	if err != nil || second.OrphansReaped != 0 || second.RunnersReaped != 0 {
		t.Fatalf("second pass changed things: %+v, %v", second, err)
	}
}

type listFailingProvider struct{ *providers.FakeProvider }

func (listFailingProvider) ListManaged(context.Context) ([]providers.RunnerInstance, error) {
	return nil, errors.New("list is unavailable")
}

func TestBackstopListFailureIsReportedWithoutBlockingOtherRepairs(t *testing.T) {
	ctx := context.Background()
	repo := state.NewMemoryRepository()
	fake := providers.NewFakeProvider(providers.FakeConfig{})
	now := time.Unix(100000, 0).UTC()
	instance := provisionWithExpiry(t, fake, now.Add(time.Hour))
	job := state.Job{ID: "job-1", State: state.JobProvisioning, ExpiresAt: now.Add(time.Hour)}
	runner := state.Runner{ID: instance.ID, JobID: job.ID, Provider: "fake", ProviderInstanceID: instance.ProviderID, State: state.RunnerProvisioning, ExpiresAt: job.ExpiresAt}
	createRecords(t, repo, job, runner, state.Lease{ID: "lease-1", JobID: job.ID, RunnerID: runner.ID, State: state.LeasePending, ExpiresAt: job.ExpiresAt})

	report, err := (&Reconciler{State: repo, Provider: listFailingProvider{fake}}).Reconcile(ctx, now)
	if err == nil || report.ProviderErrors == 0 {
		t.Fatalf("a failing list must be reported: %+v, %v", report, err)
	}
	assertRunnerState(t, repo, runner.ID, state.RunnerReady) // the ordinary repair still happened
}
