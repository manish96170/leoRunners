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

func testService(jit *fakeJIT, registration *fakeRegistration) (*Service, *providers.FakeProvider, *state.MemoryRepository) {
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
