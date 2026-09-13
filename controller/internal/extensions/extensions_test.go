package extensions

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/events"
	"github.com/leo-runners/ci-platform/controller/internal/observability"
)

type testExtension struct {
	metadata Metadata
	handle   func(context.Context, events.Event) error
	calls    atomic.Int64
}

func (e *testExtension) Metadata() Metadata { return e.metadata }
func (e *testExtension) Handle(ctx context.Context, event events.Event) error {
	e.calls.Add(1)
	return e.handle(ctx, event)
}

func newTestExtension(id string, handle func(context.Context, events.Event) error) *testExtension {
	return &testExtension{metadata: Metadata{ID: id, Name: id, Version: Version, Enabled: true}, handle: handle}
}

type testReporter struct {
	mu    sync.Mutex
	items []Execution
}

func (r *testReporter) Report(execution Execution) {
	r.mu.Lock()
	r.items = append(r.items, execution)
	r.mu.Unlock()
}

func (r *testReporter) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

func extensionEvent(t *testing.T) events.Event {
	t.Helper()
	event, err := events.New(events.Event{Kind: events.KindJob, Type: "job.queued", OccurredAt: time.Unix(10, 0)})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met")
}

func TestRegistryValidatesAndSortsMetadata(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(newTestExtension("b", func(context.Context, events.Event) error { return nil })); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(newTestExtension("a", func(context.Context, events.Event) error { return nil })); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(newTestExtension("a", func(context.Context, events.Event) error { return nil })); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	list := registry.List()
	if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("metadata not sorted: %+v", list)
	}
}

func TestDisabledExtensionReceivesNothing(t *testing.T) {
	bus := events.NewBus(events.BusConfig{Buffer: 4})
	registry := NewRegistry()
	extension := newTestExtension("disabled", func(context.Context, events.Event) error { return nil })
	extension.metadata.Enabled = false
	if err := registry.Register(extension); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(bus, registry, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(extensionEvent(t)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if extension.calls.Load() != 0 {
		t.Fatal("disabled extension received an event")
	}
	if err := dispatcher.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFailureIsolatedAndReported(t *testing.T) {
	bus := events.NewBus(events.BusConfig{Buffer: 8})
	registry := NewRegistry()
	failing := newTestExtension("failing", func(context.Context, events.Event) error { return errors.New("boom") })
	working := newTestExtension("working", func(context.Context, events.Event) error { return nil })
	if err := registry.Register(failing); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(working); err != nil {
		t.Fatal(err)
	}
	reporter := &testReporter{}
	dispatcher, err := NewDispatcher(bus, registry, Config{Reporter: reporter})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(extensionEvent(t)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return working.calls.Load() == 1 && reporter.count() == 2 })
	metrics := dispatcher.Metrics()
	if metrics.ByExtension["failing"].Failed != 1 || metrics.ByExtension["working"].Succeeded != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if err := dispatcher.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSlowExtensionTimesOutWithoutBlockingPublish(t *testing.T) {
	bus := events.NewBus(events.BusConfig{Buffer: 8})
	registry := NewRegistry()
	slow := newTestExtension("slow", func(ctx context.Context, _ events.Event) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err := registry.Register(slow); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(bus, registry, Config{Timeout: 20 * time.Millisecond, CloseTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := bus.Publish(extensionEvent(t)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("publish blocked on slow extension: %s", elapsed)
	}
	waitFor(t, func() bool { return dispatcher.Metrics().ByExtension["slow"].TimedOut == 1 })
	if err := dispatcher.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPanicIsolated(t *testing.T) {
	bus := events.NewBus(events.BusConfig{Buffer: 4})
	registry := NewRegistry()
	panicking := newTestExtension("panic", func(context.Context, events.Event) error { panic("bad extension") })
	if err := registry.Register(panicking); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(bus, registry, Config{CloseTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(extensionEvent(t)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return dispatcher.Metrics().ByExtension["panic"].Panicked == 1 })
	if err := dispatcher.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionMetricsUseBoundedDimensions(t *testing.T) {
	bus := events.NewBus(events.BusConfig{Buffer: 4})
	registry := NewRegistry()
	if err := registry.Register(newTestExtension("analytics", func(context.Context, events.Event) error { return errors.New("failed") })); err != nil {
		t.Fatal(err)
	}
	metrics := observability.NewCollector(32)
	dispatcher, err := NewDispatcher(bus, registry, Config{MetricsSink: metrics})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	event, err := events.New(events.Event{Kind: events.KindJob, Type: "job.queued", OccurredAt: time.Unix(10, 0), Provider: events.ProviderMetadata{Name: "aws", Region: "us-east-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(event); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return dispatcher.Metrics().ByExtension["analytics"].Failed == 1 })
	if err := dispatcher.Close(); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, sample := range metrics.Snapshot() {
		seen[sample.Name] = true
		if sample.Dimensions["extension"] != "analytics" || sample.Dimensions["provider"] != "aws" || sample.Dimensions["region"] != "us-east-1" {
			t.Fatalf("unexpected metric dimensions: %+v", sample)
		}
	}
	for _, name := range []string{"extension_events_received_total", "extension_events_queued_total", "extension_executions_failed_total"} {
		if !seen[name] {
			t.Fatalf("missing metric %q", name)
		}
	}
}

func TestCloseBeforeStartAndStartSemantics(t *testing.T) {
	bus := events.NewBus(events.BusConfig{})
	registry := NewRegistry()
	if err := registry.Register(newTestExtension("one", func(context.Context, events.Event) error { return nil })); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(bus, registry, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Start(); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected closed error, got %v", err)
	}
}
