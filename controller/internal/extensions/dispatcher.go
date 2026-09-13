package extensions

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/events"
	"github.com/leo-runners/ci-platform/controller/internal/observability"
)

type runtimeExtension struct {
	extension Extension
	metadata  Metadata
	queue     chan events.Event
	counters  counter
}

type Dispatcher struct {
	bus      *events.Bus
	registry *Registry
	config   Config

	mu             sync.Mutex
	started        bool
	closed         bool
	cancel         context.CancelFunc
	sub            *events.Subscription
	runtimes       []*runtimeExtension
	wg             sync.WaitGroup
	reportWG       sync.WaitGroup
	reports        chan Execution
	reportsDropped atomic.Uint64
}

func NewDispatcher(bus *events.Bus, registry *Registry, config Config) (*Dispatcher, error) {
	if bus == nil {
		return nil, fmt.Errorf("%w: nil event bus", ErrInvalidExtension)
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: nil registry", ErrInvalidExtension)
	}
	config.setDefaults()
	return &Dispatcher{bus: bus, registry: registry, config: config}, nil
}

// Start subscribes once and starts one bounded worker per enabled extension.
// Disabled extensions remain visible in Registry.List but receive no events.
func (d *Dispatcher) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.started {
		return ErrStarted
	}
	sub, err := d.bus.Subscribe()
	if err != nil {
		return err
	}
	d.sub = sub
	d.reports = make(chan Execution, d.config.ReportBuffer)
	for _, extension := range d.registry.snapshot() {
		metadata := extension.Metadata()
		if !metadata.Enabled {
			continue
		}
		runtime := &runtimeExtension{extension: extension, metadata: cloneMetadata(metadata), queue: make(chan events.Event, d.config.QueueSize)}
		d.runtimes = append(d.runtimes, runtime)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.started = true
	d.wg.Add(1)
	go d.dispatch(ctx, sub)
	for _, runtime := range d.runtimes {
		d.wg.Add(1)
		go d.worker(ctx, runtime)
	}
	if d.config.Reporter != nil {
		d.reportWG.Add(1)
		go d.reportLoop(ctx)
	}
	return nil
}

func (d *Dispatcher) dispatch(ctx context.Context, sub *events.Subscription) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sub.C():
			if !ok {
				return
			}
			for _, runtime := range d.runtimes {
				runtime.counters.received.Add(1)
				d.recordMetric("extension_events_received_total", runtime.metadata.ID, event, "received")
				select {
				case runtime.queue <- event:
					runtime.counters.queued.Add(1)
					d.recordMetric("extension_events_queued_total", runtime.metadata.ID, event, "queued")
				default:
					runtime.counters.dropped.Add(1)
					d.recordMetric("extension_events_dropped_total", runtime.metadata.ID, event, "dropped")
				}
			}
		}
	}
}

func (d *Dispatcher) worker(ctx context.Context, runtime *runtimeExtension) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-runtime.queue:
			d.execute(ctx, runtime, event)
		}
	}
}

func (d *Dispatcher) execute(parent context.Context, runtime *runtimeExtension, event events.Event) {
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(parent, d.config.Timeout)
	err, panicked := callSafely(ctx, runtime.extension, event)
	cancel()
	execution := Execution{ExtensionID: runtime.metadata.ID, EventID: event.ID, StartedAt: started, Duration: time.Since(started), Outcome: "success"}
	if panicked {
		runtime.counters.panicked.Add(1)
		runtime.counters.failed.Add(1)
		execution.Outcome = "panic"
		execution.Error = "extension panicked"
		d.recordMetric("extension_executions_panicked_total", runtime.metadata.ID, event, "panic")
	} else if err != nil {
		runtime.counters.failed.Add(1)
		if ctx.Err() == context.DeadlineExceeded {
			runtime.counters.timedOut.Add(1)
			execution.Outcome = "timeout"
		} else {
			execution.Outcome = "error"
		}
		execution.Error = err.Error()
		d.recordMetric("extension_executions_failed_total", runtime.metadata.ID, event, execution.Outcome)
		if execution.Outcome == "timeout" {
			d.recordMetric("extension_executions_timed_out_total", runtime.metadata.ID, event, "timeout")
		}
	} else {
		runtime.counters.succeeded.Add(1)
		d.recordMetric("extension_executions_succeeded_total", runtime.metadata.ID, event, "success")
	}
	d.enqueueReport(execution)
}

func callSafely(ctx context.Context, extension Extension, event events.Event) (err error, panicked bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("extension panic: %v", recovered)
			panicked = true
			_ = debug.Stack()
		}
	}()
	err = extension.Handle(ctx, event)
	return err, false
}

func (d *Dispatcher) enqueueReport(execution Execution) {
	if d.config.Reporter == nil {
		return
	}
	select {
	case d.reports <- execution:
	default:
		d.reportsDropped.Add(1)
		d.recordMetric("extension_reports_dropped_total", execution.ExtensionID, events.Event{ID: execution.EventID, OccurredAt: execution.StartedAt}, "dropped")
	}
}

func (d *Dispatcher) recordMetric(name, extensionID string, event events.Event, outcome string) {
	if d.config.MetricsSink == nil {
		return
	}
	dims := observability.Dimensions{"extension": boundedMetricValue(extensionID)}
	if value := boundedMetricValue(event.Provider.Name); value != "" {
		dims["provider"] = value
	}
	if value := boundedMetricValue(event.Provider.Region); value != "" {
		dims["region"] = value
	}
	if value := boundedMetricValue(outcome); value != "" {
		dims["outcome"] = value
	}
	_ = d.config.MetricsSink.Record(observability.MetricSample{Name: name, Kind: observability.MetricCounter, Unit: observability.UnitCount, Value: 1, Timestamp: time.Now().UTC(), Dimensions: dims})
}

func boundedMetricValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 64 {
		return value[:64]
	}
	return value
}

func (d *Dispatcher) reportLoop(ctx context.Context) {
	defer d.reportWG.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case execution := <-d.reports:
			d.config.Reporter.Report(execution)
		}
	}
}

// Close cancels dispatch and workers, waits up to CloseTimeout, and then
// closes optional extension resources. It is safe to call more than once.
func (d *Dispatcher) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	if !d.started {
		d.mu.Unlock()
		return nil
	}
	cancel, sub := d.cancel, d.sub
	d.mu.Unlock()
	cancel()
	_ = sub.Close()
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		d.reportWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d.config.CloseTimeout):
		return ErrCloseTimeout
	}
	for _, runtime := range d.runtimes {
		if closer, ok := runtime.extension.(Closer); ok {
			if err := closeWithTimeout(closer, d.config.CloseTimeout); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Dispatcher) Metrics() Metrics {
	d.mu.Lock()
	runtimes := append([]*runtimeExtension(nil), d.runtimes...)
	d.mu.Unlock()
	result := Metrics{ByExtension: make(map[string]Counters, len(runtimes)), ReportsDropped: d.reportsDropped.Load()}
	for _, runtime := range runtimes {
		result.ByExtension[runtime.metadata.ID] = runtime.counters.snapshot()
	}
	return result
}

func closeWithTimeout(closer Closer, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- closer.Close() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return ErrCloseTimeout
	}
}
