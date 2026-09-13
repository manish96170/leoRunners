// Package runners coordinates GitHub JIT registration with provider
// provisioning and durable runner lifecycle state.
package runners

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

const (
	MetadataJITConfig        = "github_jit_config"
	MetadataGitHubRunnerID   = "github_runner_id"
	MetadataGitHubRunnerName = "github_runner_name"

	RegistrationPending RegistrationState = "pending"
	RegistrationReady   RegistrationState = "ready"
	RegistrationFailed  RegistrationState = "failed"
	RegistrationCleaned RegistrationState = "cleaned"
)

var (
	ErrInvalidRequest       = errors.New("invalid runner assignment request")
	ErrNotConfigured        = errors.New("runner assignment service is not configured")
	ErrInvalidJITConfig     = errors.New("JIT response has no runner identity or configuration")
	ErrRegistrationFailed   = errors.New("GitHub runner registration failed")
	ErrAssignmentInProgress = errors.New("GitHub runner assignment is already in progress")
	ErrForkNotAllowed       = errors.New("fork workload is not allowed for ephemeral runner assignment")
)

// JITRequest contains the public inputs to GitHub's generate-jitconfig API.
// It intentionally contains no provider or cloud credentials.
type JITRequest struct {
	Repository    string
	Name          string
	Labels        []string
	RunnerGroup   string
	RunnerGroupID *int64
}

// JITConfig contains the one-time bootstrap value returned by GitHub. The
// EncodedConfig field must never be logged or written to state.
type JITConfig struct {
	EncodedConfig string
	RunnerID      int64
	RunnerName    string
}

// GitHubRunner is the identity observed after the runner process registers.
type GitHubRunner struct {
	ID   int64
	Name string
}

// JITClient obtains a short-lived JIT configuration. The implementation may
// use a GitHub App installation token or another short-lived credential.
type JITClient interface {
	GenerateJITConfig(context.Context, JITRequest) (JITConfig, error)
}

// RegistrationVerifier observes registration from the runner host. A real
// implementation can poll GitHub's runner list; tests can use a deterministic
// fake. The encoded JIT config is not part of this request.
type RegistrationVerifier interface {
	WaitForRegistration(context.Context, RegistrationRequest) (GitHubRunner, error)
}

// RegistrationCleanup is optional because GitHub JIT runners normally clean
// themselves up on exit. Implementations that can observe a runner after a
// failed assignment should remove it explicitly and remain idempotent.
type RegistrationCleanup interface {
	RemoveRunner(context.Context, RegistrationRequest) error
}

type RegistrationRequest struct {
	Repository string
	RunnerID   int64
	RunnerName string
}

type RegistrationState string

// Registration is the durable GitHub identity associated with a provider
// runner. It is stored as lifecycle-event data by this package because the
// existing state.Runner schema is cloud-neutral and intentionally has no
// GitHub-specific fields.
type Registration struct {
	RunnerID         string
	JobID            string
	GitHubRunnerID   int64
	GitHubRunnerName string
	State            RegistrationState
}

type AssignmentRequest struct {
	Job            state.Job
	Spec           providers.RunnerSpec
	Provider       providers.Provider
	CapacityPoolID string
	IsFork         bool
}

type Assignment struct {
	Runner       state.Runner
	Registration Registration
}

type Service struct {
	State          state.Repository
	Provider       providers.Provider
	JIT            JITClient
	Registration   RegistrationVerifier
	RunnerGroupID  *int64
	CleanupTimeout time.Duration
	Now            func() time.Time
	Policy         AssignmentPolicy
	mu             sync.Mutex
	inflight       map[string]struct{}
}

// AssignmentPolicy is the assignment-side counterpart to github.ScopePolicy.
// It prevents a caller from bypassing webhook admission with a direct request.
type AssignmentPolicy struct {
	Repositories  []string
	AllowedLabels []string
	RunnerGroupID int64
	AllowForks    bool
}

func (p AssignmentPolicy) validate(request AssignmentRequest, configuredGroupID *int64) error {
	if len(p.Repositories) == 0 || len(p.AllowedLabels) == 0 {
		return fmt.Errorf("%w: assignment repository and label allowlists are required", ErrInvalidRequest)
	}
	if p.RunnerGroupID <= 0 || configuredGroupID == nil || *configuredGroupID != p.RunnerGroupID {
		return fmt.Errorf("%w: configured runner group does not match the approved policy", ErrInvalidRequest)
	}
	if request.IsFork && !p.AllowForks {
		return ErrForkNotAllowed
	}
	if !containsFold(p.Repositories, request.Job.Repository) {
		return fmt.Errorf("%w: repository %q is not approved", ErrInvalidRequest, request.Job.Repository)
	}
	for _, label := range request.Job.Labels {
		if !containsFold(p.AllowedLabels, label) {
			return fmt.Errorf("%w: label %q is not approved", ErrInvalidRequest, label)
		}
	}
	return nil
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// AssignmentService is the contract consumed by scheduling/controller code.
type AssignmentService interface {
	Assign(context.Context, AssignmentRequest) (Assignment, error)
}

func (s *Service) Assign(ctx context.Context, request AssignmentRequest) (Assignment, error) {
	if s.State == nil || (s.Provider == nil && request.Provider == nil) || s.JIT == nil || s.Registration == nil {
		return Assignment{}, ErrNotConfigured
	}
	if request.Job.ID == "" || request.Job.Repository == "" || request.Spec.ExpiresAt.IsZero() {
		return Assignment{}, ErrInvalidRequest
	}
	if len(s.Policy.Repositories) > 0 || len(s.Policy.AllowedLabels) > 0 {
		if err := s.Policy.validate(request, s.RunnerGroupID); err != nil {
			return Assignment{}, err
		}
	}
	if !s.beginAssignment(request.Job.ID) {
		return Assignment{}, ErrAssignmentInProgress
	}
	defer s.endAssignment(request.Job.ID)
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}

	jit, err := s.JIT.GenerateJITConfig(ctx, JITRequest{
		Repository:    request.Job.Repository,
		Name:          request.Spec.Metadata[MetadataGitHubRunnerName],
		Labels:        append([]string(nil), request.Job.Labels...),
		RunnerGroupID: s.RunnerGroupID,
	})
	if err != nil {
		return Assignment{}, fmt.Errorf("generate GitHub JIT config: %w", err)
	}
	if jit.EncodedConfig == "" || jit.RunnerID <= 0 || strings.TrimSpace(jit.RunnerName) == "" {
		return Assignment{}, ErrInvalidJITConfig
	}

	spec := cloneSpec(request.Spec)
	if spec.Metadata == nil {
		spec.Metadata = make(map[string]string)
	}
	// This metadata is consumed by the provider bootstrap. It is deliberately
	// never copied into state.Runner or lifecycle-event data.
	spec.Metadata[MetadataJITConfig] = jit.EncodedConfig
	spec.Metadata[MetadataGitHubRunnerID] = strconv.FormatInt(jit.RunnerID, 10)
	spec.Metadata[MetadataGitHubRunnerName] = jit.RunnerName
	spec.Metadata["github_repository"] = request.Job.Repository

	provider := s.Provider
	if request.Provider != nil {
		provider = request.Provider
	}
	instance, err := provider.Provision(ctx, spec)
	if err != nil {
		return Assignment{}, fmt.Errorf("provision runner: %w", err)
	}
	var runner state.Runner
	runnerPersisted := false
	cleanup := func(cause error) (Assignment, error) {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), s.cleanupTimeout())
		defer cancel()
		if runnerPersisted {
			current, getErr := s.State.GetRunner(cleanupCtx, runner.ID)
			if getErr != nil {
				return Assignment{}, fmt.Errorf("%w; load cleanup state: %v", cause, getErr)
			}
			current.State = state.RunnerTerminating
			updated, saveErr := s.State.SaveRunner(cleanupCtx, current, current.Revision)
			if saveErr != nil {
				return Assignment{}, fmt.Errorf("%w; mark runner terminating: %v", cause, saveErr)
			}
			runner = updated
		}
		if terminateErr := provider.Terminate(cleanupCtx, instance); terminateErr != nil {
			return Assignment{}, fmt.Errorf("%w; terminate provisioned runner: %v", cause, terminateErr)
		}
		if cleanupRegistration, ok := s.Registration.(RegistrationCleanup); ok {
			if removeErr := cleanupRegistration.RemoveRunner(cleanupCtx, RegistrationRequest{Repository: request.Job.Repository, RunnerID: jit.RunnerID, RunnerName: jit.RunnerName}); removeErr != nil {
				return Assignment{}, fmt.Errorf("%w; remove GitHub runner: %v", cause, removeErr)
			}
		}
		if runnerPersisted {
			runner.State = state.RunnerTerminated
			if _, saveErr := s.State.SaveRunner(cleanupCtx, runner, runner.Revision); saveErr != nil {
				return Assignment{}, fmt.Errorf("%w; mark runner terminated: %v", cause, saveErr)
			}
		}
		return Assignment{}, cause
	}

	if err = provider.WaitReady(ctx, instance); err != nil {
		return cleanup(fmt.Errorf("wait for runner readiness: %w", err))
	}

	runner = state.Runner{
		ID: instance.ID, JobID: request.Job.ID, Provider: instance.Provider,
		ProviderInstanceID: instance.ProviderID, Labels: append([]string(nil), request.Job.Labels...),
		CapacityPoolID: request.CapacityPoolID, CPU: spec.CPU, MemoryGB: spec.MemoryGB, GPU: boolInt(spec.GPU),
		State: state.RunnerProvisioning, ControllerOwner: request.Job.ControllerOwner,
		CreatedAt: instance.CreatedAt, ExpiresAt: request.Job.ExpiresAt,
	}
	if err = s.State.CreateRunner(ctx, runner); err != nil {
		return cleanup(fmt.Errorf("persist provisioned runner: %w", err))
	}
	runnerPersisted = true
	// CreateRunner intentionally has no return value. Reload to obtain the
	// repository-assigned revision before any conditional update.
	runner, err = s.State.GetRunner(ctx, runner.ID)
	if err != nil {
		return cleanup(fmt.Errorf("reload provisioned runner: %w", err))
	}
	pending := Registration{RunnerID: runner.ID, JobID: runner.JobID, GitHubRunnerID: jit.RunnerID, GitHubRunnerName: jit.RunnerName, State: RegistrationPending}
	if err = s.persistRegistration(ctx, pending, now().UTC()); err != nil {
		return cleanup(fmt.Errorf("persist pending registration: %w", err))
	}

	registered, err := s.Registration.WaitForRegistration(ctx, RegistrationRequest{
		Repository: request.Job.Repository, RunnerID: jit.RunnerID, RunnerName: jit.RunnerName,
	})
	if err != nil {
		failed := pending
		failed.State = RegistrationFailed
		failureCtx, cancel := context.WithTimeout(context.Background(), s.cleanupTimeout())
		persistErr := s.persistRegistration(failureCtx, failed, now().UTC())
		cancel()
		if persistErr != nil {
			err = fmt.Errorf("%w; persist registration failure: %v", err, persistErr)
		}
		return cleanup(fmt.Errorf("%w: %v", ErrRegistrationFailed, err))
	}
	if registered.ID != jit.RunnerID || strings.TrimSpace(registered.Name) != jit.RunnerName {
		failed := pending
		failed.State = RegistrationFailed
		failureCtx, cancel := context.WithTimeout(context.Background(), s.cleanupTimeout())
		persistErr := s.persistRegistration(failureCtx, failed, now().UTC())
		cancel()
		if persistErr != nil {
			return cleanup(fmt.Errorf("%w; persist registration failure: %v", ErrRegistrationFailed, persistErr))
		}
		return cleanup(fmt.Errorf("%w: verifier returned unexpected runner identity", ErrRegistrationFailed))
	}

	runner.State = state.RunnerReady
	if _, err = s.State.SaveRunner(ctx, runner, runner.Revision); err != nil {
		return cleanup(fmt.Errorf("persist ready runner: %w", err))
	}
	ready := pending
	ready.State = RegistrationReady
	if err = s.persistRegistration(ctx, ready, now().UTC()); err != nil {
		return cleanup(fmt.Errorf("persist registered runner: %w", err))
	}
	return Assignment{Runner: runner, Registration: ready}, nil
}

func (s *Service) beginAssignment(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight == nil {
		s.inflight = make(map[string]struct{})
	}
	if _, exists := s.inflight[jobID]; exists {
		return false
	}
	s.inflight[jobID] = struct{}{}
	return true
}

func (s *Service) endAssignment(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inflight, jobID)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Service) cleanupTimeout() time.Duration {
	if s.CleanupTimeout > 0 {
		return s.CleanupTimeout
	}
	return 30 * time.Second
}

func (s *Service) persistRegistration(ctx context.Context, registration Registration, occurredAt time.Time) error {
	return s.StateInsertEvent(ctx, state.LifecycleEvent{
		ID:             "runner-registration/" + registration.RunnerID + "/" + string(registration.State),
		IdempotencyKey: "runner-registration/" + registration.RunnerID + "/" + string(registration.State),
		Type:           "runner.registration." + string(registration.State), JobID: registration.JobID,
		RunnerID: registration.RunnerID, Provider: "github",
		OccurredAt: occurredAt,
		Data:       map[string]string{"github_runner_id": strconv.FormatInt(registration.GitHubRunnerID, 10), "github_runner_name": registration.GitHubRunnerName, "registration_state": string(registration.State)},
	})
}

func (s *Service) StateInsertEvent(ctx context.Context, event state.LifecycleEvent) error {
	_, err := s.State.InsertLifecycleEvent(ctx, event)
	return err
}

func cloneSpec(spec providers.RunnerSpec) providers.RunnerSpec {
	spec.Labels = append([]string(nil), spec.Labels...)
	if spec.Metadata != nil {
		spec.Metadata = make(map[string]string, len(spec.Metadata))
		for key, value := range spec.Metadata {
			spec.Metadata[key] = value
		}
	}
	return spec
}
