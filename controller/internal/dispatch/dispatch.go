// Package dispatch runs webhook events in the background so the HTTP handler
// can acknowledge GitHub immediately. GitHub gives up on a delivery after about
// ten seconds, while provisioning a runner takes much longer.
//
// Guarantees:
//   - Events for the same job run one at a time, in arrival order, so a
//     "completed" event can never overtake the "queued" event of its job.
//   - Events for different jobs run concurrently, up to Workers at a time, and
//     a slow job never delays another job.
//   - The backlog is bounded. When it is full Submit fails immediately so the
//     caller can answer 503 and GitHub's delivery log shows the failure.
//   - Work does not depend on the request that delivered the event, so a client
//     that hangs up cannot cancel a provision.
//
// The queue is in memory. An event acknowledged but not finished when the
// process dies is lost; the reconciler removes any runner it created, and the
// delivery can be redelivered from GitHub.
package dispatch

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/github"
)

var (
	// ErrQueueFull means the backlog is at capacity. The event was not accepted.
	ErrQueueFull = errors.New("webhook backlog is full")
	// ErrClosed means the dispatcher is shutting down and accepts no new events.
	ErrClosed = errors.New("webhook dispatcher is shutting down")
	// ErrAbandoned is returned by Shutdown when events could not finish in time.
	ErrAbandoned = errors.New("webhook events abandoned at shutdown")
)

const (
	DefaultWorkers      = 16
	DefaultQueueSize    = 1024
	DefaultEventTimeout = 30 * time.Minute
	// forcedStopGrace is how long Shutdown waits for handlers to notice a
	// cancelled context before it gives up on them.
	forcedStopGrace = 5 * time.Second
)

// Handler processes one event. It must honor ctx.
type Handler func(ctx context.Context, event github.WorkflowJobEvent) error

// Result describes one finished event, for logging and metrics.
type Result struct {
	Event    github.WorkflowJobEvent
	Err      error
	Duration time.Duration
}

// Config tunes a Dispatcher. Zero values select the defaults.
type Config struct {
	// Workers bounds how many events run at once.
	Workers int
	// QueueSize bounds how many events may be waiting or running.
	QueueSize int
	// EventTimeout bounds one event's processing time.
	EventTimeout time.Duration
	// OnResult, if set, is called after every event. It must not block.
	OnResult func(Result)
}

type chain struct {
	pending []github.WorkflowJobEvent
}

// Dispatcher is safe for concurrent use.
type Dispatcher struct {
	handler Handler
	config  Config

	baseCtx    context.Context
	cancelBase context.CancelFunc
	slots      chan struct{}

	mu     sync.Mutex
	chains map[string]*chain
	queued int
	closed bool

	wg        sync.WaitGroup
	aborted   atomic.Bool
	abandoned atomic.Int64
}

// New creates a Dispatcher. Events are processed as they are submitted; there
// is no separate start step.
func New(handler Handler, config Config) (*Dispatcher, error) {
	if handler == nil {
		return nil, errors.New("dispatch: handler is required")
	}
	if config.Workers <= 0 {
		config.Workers = DefaultWorkers
	}
	if config.QueueSize <= 0 {
		config.QueueSize = DefaultQueueSize
	}
	if config.EventTimeout <= 0 {
		config.EventTimeout = DefaultEventTimeout
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{
		handler: handler, config: config, baseCtx: ctx, cancelBase: cancel,
		slots: make(chan struct{}, config.Workers), chains: make(map[string]*chain),
	}, nil
}

// Submit queues an event without blocking. It returns ErrQueueFull when the
// backlog is at capacity and ErrClosed once Shutdown has begun.
func (d *Dispatcher) Submit(event github.WorkflowJobEvent) error {
	key := event.JobKey
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.queued >= d.config.QueueSize {
		return ErrQueueFull
	}
	d.queued++
	c, running := d.chains[key]
	if !running {
		c = &chain{}
		d.chains[key] = c
	}
	c.pending = append(c.pending, event)
	if !running {
		d.wg.Add(1)
		go d.run(key, c)
	}
	return nil
}

// Pending reports how many events are waiting or running.
func (d *Dispatcher) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queued
}

func (d *Dispatcher) run(key string, c *chain) {
	defer d.wg.Done()
	for {
		d.mu.Lock()
		if len(c.pending) == 0 {
			delete(d.chains, key)
			d.mu.Unlock()
			return
		}
		event := c.pending[0]
		c.pending = c.pending[1:]
		d.mu.Unlock()

		d.process(event)

		d.mu.Lock()
		d.queued--
		d.mu.Unlock()
	}
}

func (d *Dispatcher) process(event github.WorkflowJobEvent) {
	if d.aborted.Load() {
		d.abandoned.Add(1)
		d.report(Result{Event: event, Err: ErrAbandoned})
		return
	}
	select {
	case d.slots <- struct{}{}:
	case <-d.baseCtx.Done():
		d.abandoned.Add(1)
		d.report(Result{Event: event, Err: ErrAbandoned})
		return
	}
	defer func() { <-d.slots }()
	// Both the slot and the cancellation can be ready at once; never start new
	// work once shutdown has given up.
	if d.baseCtx.Err() != nil {
		d.abandoned.Add(1)
		d.report(Result{Event: event, Err: ErrAbandoned})
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(d.baseCtx, d.config.EventTimeout)
	defer cancel()
	err := d.safeHandle(ctx, event)
	d.report(Result{Event: event, Err: err, Duration: time.Since(start)})
}

func (d *Dispatcher) safeHandle(ctx context.Context, event github.WorkflowJobEvent) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic handling webhook event: %v\n%s", recovered, debug.Stack())
		}
	}()
	return d.handler(ctx, event)
}

func (d *Dispatcher) report(result Result) {
	if d.config.OnResult != nil {
		d.config.OnResult(result)
	}
}

// Shutdown stops accepting events and waits for the backlog to finish. If ctx
// ends first, running handlers are cancelled and events that never started are
// dropped; Shutdown then returns ErrAbandoned with the count.
func (d *Dispatcher) Shutdown(ctx context.Context) error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		d.cancelBase()
		return nil
	case <-ctx.Done():
	}

	d.aborted.Store(true)
	d.cancelBase()
	grace := time.NewTimer(forcedStopGrace)
	defer grace.Stop()
	select {
	case <-done:
	case <-grace.C:
	}
	return fmt.Errorf("%w: %d dropped before starting, %d still running", ErrAbandoned, d.abandoned.Load(), d.Pending())
}
