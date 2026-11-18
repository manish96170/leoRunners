package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RunnerSpec contains scheduler requirements and no cloud-specific settings.
type RunnerSpec struct {
	CPU             int
	MemoryGB        int
	Architecture    string
	OS              string
	DiskGB          int
	GPU             bool
	Image           string
	Labels          []string
	DockerRequired  bool
	NodeVersion     string
	SecurityProfile string
	Metadata        map[string]string
	ExpiresAt       time.Time
}

type RunnerStatus string

const (
	StatusUnknown      RunnerStatus = "unknown"
	StatusProvisioning RunnerStatus = "provisioning"
	StatusReady        RunnerStatus = "ready"
	StatusBusy         RunnerStatus = "busy"
	StatusTerminating  RunnerStatus = "terminating"
	StatusTerminated   RunnerStatus = "terminated"
	StatusFailed       RunnerStatus = "failed"
)

type RunnerInstance struct {
	ID, ProviderID string
	Provider       string
	Status         RunnerStatus
	CreatedAt      time.Time
	ExpiresAt      time.Time
	Spec           RunnerSpec
}

// Provider is the cloud-neutral lifecycle boundary used by the controller.
type Provider interface {
	Provision(context.Context, RunnerSpec) (RunnerInstance, error)
	WaitReady(context.Context, RunnerInstance) error
	Status(context.Context, RunnerInstance) (RunnerStatus, error)
	Terminate(context.Context, RunnerInstance) error
}

type RunnerProvider = Provider

var (
	ErrCapacity         = errors.New("provider capacity exhausted")
	ErrCapacityExceeded = ErrCapacity
	ErrInstanceNotFound = errors.New("provider instance not found")
)

// FailurePlan injects failures into a finite number of calls. A negative count
// fails every call until the plan is replaced; zero disables that operation.
type FailurePlan struct {
	Provision int
	WaitReady int
	Status    int
	Terminate int
	Err       error
}

type FakeConfig struct {
	Capacity       int
	ReadyAfter     time.Duration
	ReadinessDelay time.Duration
	Failures       FailurePlan
}

type FakeProvider struct {
	mu         sync.Mutex
	Capacity   int
	ReadyAfter time.Duration
	Failures   FailurePlan
	instances  map[string]RunnerInstance
	sequence   uint64
}

type Fake = FakeProvider

func NewFake(capacity int) *FakeProvider {
	return NewFakeProvider(FakeConfig{Capacity: capacity})
}

func NewFakeProvider(config FakeConfig) *FakeProvider {
	readyAfter := config.ReadyAfter
	if config.ReadinessDelay != 0 {
		readyAfter = config.ReadinessDelay
	}
	return &FakeProvider{Capacity: config.Capacity, ReadyAfter: readyAfter, Failures: config.Failures, instances: make(map[string]RunnerInstance)}
}

func (f *FakeProvider) Provision(ctx context.Context, spec RunnerSpec) (RunnerInstance, error) {
	if err := contextError(ctx); err != nil {
		return RunnerInstance{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failureLocked(&f.Failures.Provision); err != nil {
		return RunnerInstance{}, err
	}
	if f.Capacity > 0 && f.activeLocked() >= f.Capacity {
		return RunnerInstance{}, ErrCapacity
	}
	f.sequence++
	id := fmt.Sprintf("fake-runner-%d", f.sequence)
	instance := RunnerInstance{ID: id, ProviderID: id, Provider: "fake", Status: StatusProvisioning, CreatedAt: time.Now().UTC(), ExpiresAt: spec.ExpiresAt, Spec: cloneSpec(spec)}
	f.instances[id] = instance
	return cloneInstance(instance), nil
}

func (f *FakeProvider) WaitReady(ctx context.Context, instance RunnerInstance) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	if err := f.failureLocked(&f.Failures.WaitReady); err != nil {
		f.setStatusLocked(instance.ID, StatusFailed)
		f.mu.Unlock()
		return err
	}
	if _, ok := f.instances[instance.ID]; !ok {
		f.mu.Unlock()
		return ErrInstanceNotFound
	}
	delay := f.ReadyAfter
	f.mu.Unlock()
	if err := waitContext(ctx, delay); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.instances[instance.ID]
	if !ok {
		return ErrInstanceNotFound
	}
	if current.Status == StatusTerminated || current.Status == StatusTerminating {
		return nil
	}
	current.Status = StatusReady
	f.instances[instance.ID] = current
	return nil
}

func (f *FakeProvider) Status(ctx context.Context, instance RunnerInstance) (RunnerStatus, error) {
	if err := contextError(ctx); err != nil {
		return StatusUnknown, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failureLocked(&f.Failures.Status); err != nil {
		return StatusUnknown, err
	}
	current, ok := f.instances[instance.ID]
	if !ok {
		return StatusUnknown, ErrInstanceNotFound
	}
	return current.Status, nil
}

// Terminate is idempotent for an instance known to the provider. An already
// terminated instance succeeds without consuming an injected failure.
func (f *FakeProvider) Terminate(ctx context.Context, instance RunnerInstance) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.instances[instance.ID]
	if !ok || current.Status == StatusTerminated {
		return nil
	}
	if err := f.failureLocked(&f.Failures.Terminate); err != nil {
		return err
	}
	current.Status = StatusTerminated
	f.instances[instance.ID] = current
	return nil
}

func (f *FakeProvider) SetStatus(id string, status RunnerStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.instances[id]; !ok {
		return ErrInstanceNotFound
	}
	f.setStatusLocked(id, status)
	return nil
}

func (f *FakeProvider) Active() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeLocked()
}

func (f *FakeProvider) ActiveCount() int { return f.Active() }

func (f *FakeProvider) activeLocked() int {
	active := 0
	for _, instance := range f.instances {
		if instance.Status != StatusTerminated {
			active++
		}
	}
	return active
}

func (f *FakeProvider) setStatusLocked(id string, status RunnerStatus) {
	instance := f.instances[id]
	instance.Status = status
	f.instances[id] = instance
}

func (f *FakeProvider) failureLocked(count *int) error {
	if *count == 0 {
		return nil
	}
	if *count > 0 {
		(*count)--
	}
	if f.Failures.Err != nil {
		return f.Failures.Err
	}
	return errors.New("fake provider injected failure")
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func waitContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return contextError(ctx)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func cloneSpec(spec RunnerSpec) RunnerSpec {
	spec.Labels = append([]string(nil), spec.Labels...)
	if spec.Metadata != nil {
		spec.Metadata = make(map[string]string, len(spec.Metadata))
		for key, value := range spec.Metadata {
			spec.Metadata[key] = value
		}
	}
	return spec
}

func cloneInstance(instance RunnerInstance) RunnerInstance {
	instance.Spec = cloneSpec(instance.Spec)
	return instance
}
