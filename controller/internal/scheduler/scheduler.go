package scheduler

import (
	"context"
	"errors"
	"github.com/leo-runners/ci-platform/controller/internal/capacity"
	"github.com/leo-runners/ci-platform/controller/internal/policy"
	"github.com/leo-runners/ci-platform/controller/internal/providers"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

var ErrNoCapacity = errors.New("no eligible capacity")

type Capacity struct {
	Pool     policy.Pool
	Provider providers.Provider
}
type Scheduler struct {
	State         state.Repository
	Capacities    []Capacity
	Mode          policy.Mode
	PoolRegistry  *capacity.Registry
	PoolProviders map[string]providers.Provider
}

// Reserve selects and reserves a registry-backed pool before a separate
// assignment service performs provider/JIT work.
func (s *Scheduler) Reserve(spec providers.RunnerSpec) (providers.Provider, string, error) {
	if s.PoolRegistry == nil {
		return nil, "", ErrNoCapacity
	}
	selected, ok := s.PoolRegistry.Select(capacity.Request{Mode: capacity.SelectionMode(s.Mode), Labels: spec.Labels, CPU: spec.CPU, MemoryGB: spec.MemoryGB, GPU: boolInt(spec.GPU)})
	if !ok {
		return nil, "", ErrNoCapacity
	}
	provider := s.PoolProviders[selected.ID]
	if provider == nil {
		return nil, "", errors.New("capacity pool has no provider")
	}
	if err := s.PoolRegistry.Reserve(selected.ID, spec.CPU, spec.MemoryGB, boolInt(spec.GPU)); err != nil {
		return nil, "", err
	}
	return provider, selected.ID, nil
}

func (s *Scheduler) Schedule(ctx context.Context, job state.Job, spec providers.RunnerSpec) (state.Runner, error) {
	if s.PoolRegistry != nil {
		selected, ok := s.PoolRegistry.Select(capacity.Request{Mode: capacity.SelectionMode(s.Mode), Labels: spec.Labels, CPU: spec.CPU, MemoryGB: spec.MemoryGB, GPU: boolInt(spec.GPU)})
		if !ok {
			return state.Runner{}, ErrNoCapacity
		}
		provider := s.PoolProviders[selected.ID]
		if provider == nil {
			return state.Runner{}, errors.New("capacity pool has no provider")
		}
		if err := s.PoolRegistry.Reserve(selected.ID, spec.CPU, spec.MemoryGB, boolInt(spec.GPU)); err != nil {
			return state.Runner{}, err
		}
		instance, err := provider.Provision(ctx, spec)
		if err != nil {
			_ = s.PoolRegistry.Release(selected.ID, spec.CPU, spec.MemoryGB, boolInt(spec.GPU))
			return state.Runner{}, err
		}
		if err = provider.WaitReady(ctx, instance); err != nil {
			_ = provider.Terminate(context.Background(), instance)
			_ = s.PoolRegistry.Release(selected.ID, spec.CPU, spec.MemoryGB, boolInt(spec.GPU))
			return state.Runner{}, err
		}
		runner := state.Runner{ID: instance.ID, JobID: job.ID, Provider: instance.Provider, ProviderInstanceID: instance.ProviderID, CapacityPoolID: selected.ID, CPU: spec.CPU, MemoryGB: spec.MemoryGB, GPU: boolInt(spec.GPU), State: state.RunnerReady, ExpiresAt: job.ExpiresAt}
		if err = s.State.CreateRunner(ctx, runner); err != nil {
			_ = provider.Terminate(context.Background(), instance)
			_ = s.PoolRegistry.Release(selected.ID, spec.CPU, spec.MemoryGB, boolInt(spec.GPU))
			return state.Runner{}, err
		}
		return runner, nil
	}
	pools := make([]policy.Pool, len(s.Capacities))
	for i, c := range s.Capacities {
		pools[i] = c.Pool
	}
	selected, ok := policy.Select(s.Mode, pools)
	if !ok {
		return state.Runner{}, ErrNoCapacity
	}
	var p providers.Provider
	for _, c := range s.Capacities {
		if c.Pool.ID == selected.ID {
			p = c.Provider
			break
		}
	}
	instance, err := p.Provision(ctx, spec)
	if err != nil {
		return state.Runner{}, err
	}
	if err = p.WaitReady(ctx, instance); err != nil {
		_ = p.Terminate(context.Background(), instance)
		return state.Runner{}, err
	}
	r := state.Runner{ID: instance.ID, JobID: job.ID, Provider: instance.Provider, ProviderInstanceID: instance.ProviderID, State: state.RunnerReady, ExpiresAt: job.ExpiresAt}
	if err = s.State.CreateRunner(ctx, r); err != nil {
		_ = p.Terminate(context.Background(), instance)
		return state.Runner{}, err
	}
	return r, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Scheduler) Release(runner state.Runner) error {
	if s.PoolRegistry == nil || runner.CapacityPoolID == "" {
		return nil
	}
	return s.PoolRegistry.Release(runner.CapacityPoolID, runner.CPU, runner.MemoryGB, runner.GPU)
}
