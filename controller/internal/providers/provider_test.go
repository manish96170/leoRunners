package providers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFakeProviderCapacityAndIdempotentTerminate(t *testing.T) {
	p := NewFake(1)
	first, err := p.Provision(context.Background(), RunnerSpec{Labels: []string{"linux"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Provision(context.Background(), RunnerSpec{}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("second provision error = %v, want capacity error", err)
	}
	if err := p.Terminate(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := p.Terminate(context.Background(), first); err != nil {
		t.Fatalf("repeated terminate error = %v", err)
	}
	if got := p.Active(); got != 0 {
		t.Fatalf("active count = %d, want 0", got)
	}
}

func TestFakeProviderReadinessDelayAndStatusChange(t *testing.T) {
	p := NewFakeProvider(FakeConfig{ReadinessDelay: 10 * time.Millisecond})
	instance, err := p.Provision(context.Background(), RunnerSpec{CPU: 4, Labels: []string{"docker"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Status(context.Background(), instance); got != StatusProvisioning {
		t.Fatalf("initial status = %q, want provisioning", got)
	}
	if err := p.WaitReady(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Status(context.Background(), instance); got != StatusReady {
		t.Fatalf("ready status = %q, want ready", got)
	}
	if err := p.SetStatus(instance.ID, StatusBusy); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Status(context.Background(), instance); got != StatusBusy {
		t.Fatalf("changed status = %q, want busy", got)
	}
}

func TestFakeProviderFailuresAndCancellation(t *testing.T) {
	injected := errors.New("provision unavailable")
	p := NewFakeProvider(FakeConfig{Failures: FailurePlan{Provision: 1, Err: injected}})
	if _, err := p.Provision(context.Background(), RunnerSpec{}); !errors.Is(err, injected) {
		t.Fatalf("injected error = %v", err)
	}
	instance, err := p.Provision(context.Background(), RunnerSpec{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.WaitReady(ctx, instance); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error = %v", err)
	}

	p = NewFakeProvider(FakeConfig{ReadinessDelay: time.Hour})
	instance, err = p.Provision(context.Background(), RunnerSpec{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := p.WaitReady(ctx, instance); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline wait error = %v", err)
	}
}

func TestFakeProviderStatusAndTerminateFailures(t *testing.T) {
	injected := errors.New("provider unavailable")
	p := NewFakeProvider(FakeConfig{Failures: FailurePlan{Status: 1, Terminate: 1, Err: injected}})
	instance, err := p.Provision(context.Background(), RunnerSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Status(context.Background(), instance); !errors.Is(err, injected) {
		t.Fatalf("status error = %v", err)
	}
	if err := p.Terminate(context.Background(), instance); !errors.Is(err, injected) {
		t.Fatalf("terminate error = %v", err)
	}
	if err := p.Terminate(context.Background(), instance); err != nil {
		t.Fatalf("second terminate error = %v", err)
	}
}
