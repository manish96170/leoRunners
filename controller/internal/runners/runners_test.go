package runners

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

type fakeJIT struct {
	config  JITConfig
	request JITRequest
	err     error
}

func (f *fakeJIT) GenerateJITConfig(_ context.Context, request JITRequest) (JITConfig, error) {
	f.request = request
	if f.err != nil {
		return JITConfig{}, f.err
	}
	return f.config, nil
}

type fakeRegistration struct {
	runner  GitHubRunner
	request RegistrationRequest
	err     error
}

type cleanupRegistration struct {
	fakeRegistration
	removed int
}

func (f *cleanupRegistration) RemoveRunner(_ context.Context, _ RegistrationRequest) error {
	f.removed++
	return nil
}

type recordingProvider struct {
	delegate *providers.FakeProvider
	spec     providers.RunnerSpec
}

func (p *recordingProvider) Provision(ctx context.Context, spec providers.RunnerSpec) (providers.RunnerInstance, error) {
	p.spec = spec
	return p.delegate.Provision(ctx, spec)
}
func (p *recordingProvider) WaitReady(ctx context.Context, instance providers.RunnerInstance) error {
	return p.delegate.WaitReady(ctx, instance)
}
func (p *recordingProvider) Status(ctx context.Context, instance providers.RunnerInstance) (providers.RunnerStatus, error) {
	return p.delegate.Status(ctx, instance)
}
func (p *recordingProvider) Terminate(ctx context.Context, instance providers.RunnerInstance) error {
	return p.delegate.Terminate(ctx, instance)
}

func (f *fakeRegistration) WaitForRegistration(_ context.Context, request RegistrationRequest) (GitHubRunner, error) {
	f.request = request
	if f.err != nil {
		return GitHubRunner{}, f.err
	}
	return f.runner, nil
}

func testService(jit *fakeJIT, registration RegistrationVerifier) (*Service, *providers.FakeProvider, *state.MemoryRepository) {
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	return &Service{State: repo, Provider: provider, JIT: jit, Registration: registration, Now: func() time.Time { return time.Unix(100, 0) }}, provider, repo
}

func testJob() state.Job {
	return state.Job{ID: "repo/job/42", Repository: "acme/widgets", JobID: 42, RunID: 7, Labels: []string{"linux"}, ExpiresAt: time.Unix(200, 0)}
}

func TestAssignPassesJITBootstrapMetadataAndPersistsIdentityWithoutConfig(t *testing.T) {
	const secret = "opaque-jit-value"
	jit := &fakeJIT{config: JITConfig{EncodedConfig: secret, RunnerID: 91, RunnerName: "ephemeral-91"}}
	registration := &fakeRegistration{runner: GitHubRunner{ID: 91, Name: "ephemeral-91"}}
	service, provider, repo := testService(jit, registration)
	originalSpec := providers.RunnerSpec{OS: "linux", ExpiresAt: time.Unix(200, 0), Metadata: map[string]string{"existing": "value"}}
	recording := &recordingProvider{delegate: provider}
	service.Provider = recording
	assignment, err := service.Assign(context.Background(), AssignmentRequest{Job: testJob(), Spec: originalSpec})
	if err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if provider.Active() != 1 || assignment.Runner.State != state.RunnerReady {
		t.Fatalf("assignment = %#v, active = %d", assignment, provider.Active())
	}
	if registration.request.RunnerID != 91 || registration.request.RunnerName != "ephemeral-91" {
		t.Fatalf("registration request = %#v", registration.request)
	}
	if recording.spec.Metadata[MetadataJITConfig] != secret || recording.spec.Metadata[MetadataGitHubRunnerID] != "91" || recording.spec.Metadata[MetadataGitHubRunnerName] != "ephemeral-91" {
		t.Fatalf("provider bootstrap metadata = %#v", recording.spec.Metadata)
	}
	if _, ok := originalSpec.Metadata[MetadataJITConfig]; ok {
		t.Fatal("Assign mutated the caller's provider spec")
	}
	if assignment.Registration.State != RegistrationReady || assignment.Registration.GitHubRunnerID != 91 {
		t.Fatalf("registration = %#v", assignment.Registration)
	}
	events, err := repo.ListLifecycleEvents(context.Background(), assignment.Runner.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	for _, event := range events {
		if strings.Contains(event.Type, secret) || strings.Contains(event.JobID, secret) || strings.Contains(event.RunnerID, secret) {
			t.Fatalf("JIT config leaked in event identity: %#v", event)
		}
		for key, value := range event.Data {
			if strings.Contains(key, "config") || strings.Contains(value, secret) {
				t.Fatalf("JIT config leaked in event data: %#v", event)
			}
		}
	}
}

func TestAssignPassesRequestedLabelsAndRunnerGroupToJIT(t *testing.T) {
	groupID := int64(42)
	jit := &fakeJIT{config: JITConfig{EncodedConfig: "opaque", RunnerID: 92, RunnerName: "ephemeral-92"}}
	registration := &fakeRegistration{runner: GitHubRunner{ID: 92, Name: "ephemeral-92"}}
	service, _, _ := testService(jit, registration)
	service.RunnerGroupID = &groupID

	job := testJob()
	job.Labels = []string{"self-hosted", "linux-x64", "ephemeral"}
	if _, err := service.Assign(context.Background(), AssignmentRequest{
		Job:  job,
		Spec: providers.RunnerSpec{ExpiresAt: time.Unix(200, 0)},
	}); err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if jit.request.RunnerGroupID == nil || *jit.request.RunnerGroupID != groupID {
		t.Fatalf("runner group = %#v, want %d", jit.request.RunnerGroupID, groupID)
	}
	if strings.Join(jit.request.Labels, ",") != strings.Join(job.Labels, ",") {
		t.Fatalf("labels = %#v, want %#v", jit.request.Labels, job.Labels)
	}
	job.Labels[0] = "mutated-after-assignment"
	if jit.request.Labels[0] == job.Labels[0] {
		t.Fatal("JIT labels alias caller-owned job labels")
	}
}

type cancellationRegistration struct {
	started chan struct{}
}

func (r cancellationRegistration) WaitForRegistration(ctx context.Context, _ RegistrationRequest) (GitHubRunner, error) {
	close(r.started)
	<-ctx.Done()
	return GitHubRunner{}, ctx.Err()
}

func TestAssignCancellationCleansUpProvisionedRunner(t *testing.T) {
	jit := &fakeJIT{config: JITConfig{EncodedConfig: "opaque", RunnerID: 93, RunnerName: "ephemeral-93"}}
	registration := cancellationRegistration{started: make(chan struct{})}
	service, provider, repo := testService(jit, registration)
	service.CleanupTimeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.Assign(ctx, AssignmentRequest{Job: testJob(), Spec: providers.RunnerSpec{ExpiresAt: time.Unix(200, 0)}})
		done <- err
	}()
	select {
	case <-registration.started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("registration verifier was not called")
	}
	if err := <-done; !errors.Is(err, ErrRegistrationFailed) {
		t.Fatalf("Assign() error = %v, want registration failure", err)
	}
	if provider.Active() != 0 {
		t.Fatalf("active provider instances = %d", provider.Active())
	}
	runners, err := repo.ListRunners(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].State != state.RunnerTerminated {
		t.Fatalf("runners after cancellation = %#v", runners)
	}
}

func TestAssignCleansUpWhenRegistrationFails(t *testing.T) {
	jit := &fakeJIT{config: JITConfig{EncodedConfig: "secret", RunnerID: 3, RunnerName: "runner-3"}}
	registration := &fakeRegistration{err: errors.New("runner never registered")}
	service, provider, repo := testService(jit, registration)
	_, err := service.Assign(context.Background(), AssignmentRequest{Job: testJob(), Spec: providers.RunnerSpec{ExpiresAt: time.Unix(200, 0)}})
	if !errors.Is(err, ErrRegistrationFailed) {
		t.Fatalf("error = %v", err)
	}
	if provider.Active() != 0 {
		t.Fatalf("active provider instances = %d", provider.Active())
	}
	runners, err := repo.ListRunners(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runners) != 1 || runners[0].State != state.RunnerTerminated {
		t.Fatalf("runners = %#v", runners)
	}
	events, err := repo.ListLifecycleEvents(context.Background(), runners[0].ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != "runner.registration.failed" {
		t.Fatalf("events = %#v", events)
	}
}

func TestAssignRejectsForkAndOutOfScopeRequestsBeforeJIT(t *testing.T) {
	jit := &fakeJIT{config: JITConfig{EncodedConfig: "opaque", RunnerID: 1, RunnerName: "runner"}}
	registration := &fakeRegistration{runner: GitHubRunner{ID: 1, Name: "runner"}}
	service, _, _ := testService(jit, registration)
	groupID := int64(42)
	service.RunnerGroupID = &groupID
	service.Policy = AssignmentPolicy{Repositories: []string{"acme/widgets"}, AllowedLabels: []string{"linux"}, RunnerGroupID: groupID}
	request := AssignmentRequest{Job: testJob(), IsFork: true, Spec: providers.RunnerSpec{ExpiresAt: time.Unix(200, 0)}}
	if err := func() error { _, err := service.Assign(context.Background(), request); return err }(); !errors.Is(err, ErrForkNotAllowed) {
		t.Fatalf("error = %v", err)
	}
	if jit.request.Repository != "" {
		t.Fatal("JIT was called for a fork")
	}
	request.IsFork = false
	request.Job.Repository = "other/widgets"
	if err := func() error { _, err := service.Assign(context.Background(), request); return err }(); err == nil {
		t.Fatal("out-of-scope repository was accepted")
	}
}

func TestAssignmentSingleUseGuardRejectsConcurrentDuplicate(t *testing.T) {
	service, _, _ := testService(&fakeJIT{}, &fakeRegistration{})
	if !service.beginAssignment("job-1") {
		t.Fatal("first assignment was rejected")
	}
	if service.beginAssignment("job-1") {
		t.Fatal("duplicate assignment was admitted")
	}
	service.endAssignment("job-1")
	if !service.beginAssignment("job-1") {
		t.Fatal("assignment was not released after completion")
	}
	service.endAssignment("job-1")
}

func TestAssignCallsRegistrationCleanupAfterRegistrationFailure(t *testing.T) {
	jit := &fakeJIT{config: JITConfig{EncodedConfig: "opaque", RunnerID: 8, RunnerName: "runner-8"}}
	registration := &cleanupRegistration{fakeRegistration: fakeRegistration{err: errors.New("registration failed")}}
	service, _, _ := testService(jit, registration)
	_, err := service.Assign(context.Background(), AssignmentRequest{Job: testJob(), Spec: providers.RunnerSpec{ExpiresAt: time.Unix(200, 0)}})
	if !errors.Is(err, ErrRegistrationFailed) || registration.removed != 1 {
		t.Fatalf("error = %v removed = %d", err, registration.removed)
	}
}

func TestAssignCleansUpWhenReadinessFails(t *testing.T) {
	jit := &fakeJIT{config: JITConfig{EncodedConfig: "secret", RunnerID: 4, RunnerName: "runner-4"}}
	registration := &fakeRegistration{runner: GitHubRunner{ID: 4, Name: "runner-4"}}
	service, provider, _ := testService(jit, registration)
	provider.Failures.WaitReady = 1
	_, err := service.Assign(context.Background(), AssignmentRequest{Job: testJob(), Spec: providers.RunnerSpec{ExpiresAt: time.Unix(200, 0)}})
	if err == nil || provider.Active() != 0 {
		t.Fatalf("error = %v, active = %d", err, provider.Active())
	}
}

func TestAssignDoesNotCallProviderWhenJITGenerationFails(t *testing.T) {
	jit := &fakeJIT{err: errors.New("GitHub unavailable")}
	registration := &fakeRegistration{}
	service, provider, _ := testService(jit, registration)
	_, err := service.Assign(context.Background(), AssignmentRequest{Job: testJob(), Spec: providers.RunnerSpec{ExpiresAt: time.Unix(200, 0)}})
	if err == nil || provider.Active() != 0 {
		t.Fatalf("error = %v, active = %d", err, provider.Active())
	}
}

// advancingRegistration stands in for the real wait: while the controller is
// waiting for GitHub, the reconciler (running every few seconds in the real
// process) may already have moved the runner record forward.
type advancingRegistration struct {
	fakeRegistration
	repo   *state.MemoryRepository
	mutate func(*state.Runner)
}

func (a *advancingRegistration) WaitForRegistration(ctx context.Context, request RegistrationRequest) (GitHubRunner, error) {
	all, err := a.repo.ListRunners(ctx)
	if err != nil || len(all) != 1 {
		return GitHubRunner{}, errors.New("test setup: expected exactly one persisted runner")
	}
	runner := all[0]
	a.mutate(&runner)
	if _, err := a.repo.SaveRunner(ctx, runner, runner.Revision); err != nil {
		return GitHubRunner{}, err
	}
	return a.fakeRegistration.WaitForRegistration(ctx, request)
}

func advancingService(t *testing.T, mutate func(*state.Runner)) (*Service, *providers.FakeProvider, *state.MemoryRepository) {
	t.Helper()
	jit := &fakeJIT{config: JITConfig{EncodedConfig: "encoded", RunnerID: 9, RunnerName: "runner-9"}}
	repo := state.NewMemoryRepository()
	provider := providers.NewFakeProvider(providers.FakeConfig{})
	registration := &advancingRegistration{fakeRegistration: fakeRegistration{runner: GitHubRunner{ID: 9, Name: "runner-9"}}, repo: repo, mutate: mutate}
	return &Service{State: repo, Provider: provider, JIT: jit, Registration: registration, Now: func() time.Time { return time.Unix(100, 0) }}, provider, repo
}

func assign(t *testing.T, s *Service) (Assignment, error) {
	t.Helper()
	return s.Assign(context.Background(), AssignmentRequest{Job: testJob(), Spec: providers.RunnerSpec{Labels: []string{"linux"}, ExpiresAt: time.Unix(200, 0)}})
}

// The reconciler marking a provisioning runner ready while the assignment is
// still waiting for GitHub must not fail the assignment: the runner is exactly
// where the assignment wanted it.
func TestAssignSucceedsWhenTheReconcilerAlreadyMarkedTheRunnerReady(t *testing.T) {
	service, provider, repo := advancingService(t, func(r *state.Runner) { r.State = state.RunnerReady })
	assignment, err := assign(t, service)
	if err != nil {
		t.Fatalf("Assign() error = %v, want success", err)
	}
	if assignment.Runner.State != state.RunnerReady {
		t.Fatalf("assignment runner state = %s, want ready", assignment.Runner.State)
	}
	stored, _ := repo.ListRunners(context.Background())
	if len(stored) != 1 || stored[0].State != state.RunnerReady {
		t.Fatalf("stored runners = %+v", stored)
	}
	if provider.Active() != 1 {
		t.Fatal("a healthy runner must not be terminated")
	}
}

// A bump that changes nothing important (for example a refreshed timestamp or
// label) still moves the revision. The assignment must reload and finish.
func TestAssignSucceedsWhenTheRecordWasOnlyTouched(t *testing.T) {
	service, _, _ := advancingService(t, func(r *state.Runner) { r.CPU = 2 })
	assignment, err := assign(t, service)
	if err != nil {
		t.Fatalf("Assign() error = %v, want success", err)
	}
	if assignment.Runner.State != state.RunnerReady {
		t.Fatalf("assignment runner state = %s, want ready", assignment.Runner.State)
	}
}

func TestAssignKeepsABusyRunnerBusy(t *testing.T) {
	service, _, repo := advancingService(t, func(r *state.Runner) { r.State = state.RunnerBusy })
	if _, err := assign(t, service); err != nil {
		t.Fatalf("Assign() error = %v, want success", err)
	}
	stored, _ := repo.ListRunners(context.Background())
	if stored[0].State != state.RunnerBusy {
		t.Fatalf("a runner already running its job was demoted to %s", stored[0].State)
	}
}

// If the runner was meanwhile declared dead, the assignment must fail and
// clean up rather than hand back a runner that is gone.
func TestAssignFailsAndCleansUpWhenTheRunnerWasTerminatedMeanwhile(t *testing.T) {
	for _, dead := range []state.RunnerState{state.RunnerTerminating, state.RunnerTerminated, state.RunnerFailed, state.RunnerUnknown} {
		t.Run(string(dead), func(t *testing.T) {
			service, provider, _ := advancingService(t, func(r *state.Runner) { r.State = dead })
			if _, err := assign(t, service); err == nil || !strings.Contains(err.Error(), string(dead)) {
				t.Fatalf("Assign() error = %v, want a failure naming %s", err, dead)
			}
			if provider.Active() != 0 {
				t.Fatal("the instance must be terminated when the assignment fails")
			}
		})
	}
}
