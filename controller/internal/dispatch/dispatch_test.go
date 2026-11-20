package dispatch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/github"
)

func event(job string, action github.Action) github.WorkflowJobEvent {
	return github.WorkflowJobEvent{JobKey: job, Action: action, EventID: job + "/" + string(action)}
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func shutdown(t *testing.T, d *Dispatcher) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestEventsForOneJobRunInArrivalOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	d, err := New(func(_ context.Context, e github.WorkflowJobEvent) error {
		// The first event is the slowest; ordering must still hold.
		if e.Action == github.ActionQueued {
			time.Sleep(30 * time.Millisecond)
		}
		mu.Lock()
		order = append(order, string(e.Action))
		mu.Unlock()
		return nil
	}, Config{Workers: 8})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []github.Action{github.ActionQueued, github.ActionInProgress, github.ActionCompleted} {
		if err := d.Submit(event("repo/job/1", action)); err != nil {
			t.Fatal(err)
		}
	}
	shutdown(t, d)
	if got := fmt.Sprint(order); got != "[queued in_progress completed]" {
		t.Fatalf("order = %s", got)
	}
}

func TestDifferentJobsRunConcurrentlyUpToTheWorkerLimit(t *testing.T) {
	release := make(chan struct{})
	var running, peak atomic.Int64
	d, _ := New(func(context.Context, github.WorkflowJobEvent) error {
		now := running.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		<-release
		running.Add(-1)
		return nil
	}, Config{Workers: 3, QueueSize: 20})
	for i := 0; i < 8; i++ {
		if err := d.Submit(event(fmt.Sprintf("repo/job/%d", i), github.ActionQueued)); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "three handlers running", func() bool { return running.Load() == 3 })
	time.Sleep(50 * time.Millisecond)
	if got := running.Load(); got != 3 {
		t.Fatalf("running = %d, want the worker limit 3", got)
	}
	close(release)
	shutdown(t, d)
	if peak.Load() != 3 {
		t.Fatalf("peak concurrency = %d, want 3", peak.Load())
	}
}

func TestASlowJobDoesNotDelayAnotherJob(t *testing.T) {
	block := make(chan struct{})
	var fastDone atomic.Bool
	d, _ := New(func(_ context.Context, e github.WorkflowJobEvent) error {
		if e.JobKey == "repo/job/slow" {
			<-block
			return nil
		}
		fastDone.Store(true)
		return nil
	}, Config{Workers: 4})
	_ = d.Submit(event("repo/job/slow", github.ActionQueued))
	_ = d.Submit(event("repo/job/fast", github.ActionQueued))
	waitFor(t, "fast job to finish while slow job is blocked", fastDone.Load)
	close(block)
	shutdown(t, d)
}

func TestSubmitFailsFastWhenTheBacklogIsFull(t *testing.T) {
	block := make(chan struct{})
	d, _ := New(func(context.Context, github.WorkflowJobEvent) error {
		<-block
		return nil
	}, Config{Workers: 1, QueueSize: 2})
	if err := d.Submit(event("repo/job/1", github.ActionQueued)); err != nil {
		t.Fatal(err)
	}
	if err := d.Submit(event("repo/job/2", github.ActionQueued)); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := d.Submit(event("repo/job/3", github.ActionQueued)); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("third Submit() error = %v, want ErrQueueFull", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("a full backlog must be reported immediately, not after blocking")
	}
	if d.Pending() != 2 {
		t.Fatalf("Pending() = %d, want 2", d.Pending())
	}
	close(block)
	waitFor(t, "backlog to drain", func() bool { return d.Pending() == 0 })
	if err := d.Submit(event("repo/job/3", github.ActionQueued)); err != nil {
		t.Fatalf("Submit() after drain error = %v", err)
	}
	shutdown(t, d)
}

func TestSubmitAfterShutdownIsRejected(t *testing.T) {
	d, _ := New(func(context.Context, github.WorkflowJobEvent) error { return nil }, Config{})
	shutdown(t, d)
	if err := d.Submit(event("repo/job/1", github.ActionQueued)); !errors.Is(err, ErrClosed) {
		t.Fatalf("Submit() error = %v, want ErrClosed", err)
	}
}

func TestShutdownWaitsForTheBacklog(t *testing.T) {
	var done atomic.Int64
	d, _ := New(func(context.Context, github.WorkflowJobEvent) error {
		time.Sleep(20 * time.Millisecond)
		done.Add(1)
		return nil
	}, Config{Workers: 2, QueueSize: 50})
	for i := 0; i < 10; i++ {
		_ = d.Submit(event(fmt.Sprintf("repo/job/%d", i), github.ActionQueued))
	}
	shutdown(t, d)
	if done.Load() != 10 {
		t.Fatalf("handled = %d, want all 10 before Shutdown returns", done.Load())
	}
}

func TestShutdownDeadlineCancelsRunningWorkAndDropsTheRest(t *testing.T) {
	var results sync.Map
	started := make(chan struct{})
	d, _ := New(func(ctx context.Context, e github.WorkflowJobEvent) error {
		if e.JobKey == "repo/job/1" && e.Action == github.ActionQueued {
			close(started)
		}
		<-ctx.Done()
		return ctx.Err()
	}, Config{Workers: 1, QueueSize: 10, OnResult: func(r Result) { results.Store(r.Event.EventID, r.Err) }})
	// Job 1 must hold the only worker slot before the others are submitted;
	// otherwise job 2 could win the slot and the test would wait forever.
	_ = d.Submit(event("repo/job/1", github.ActionQueued))
	<-started
	_ = d.Submit(event("repo/job/1", github.ActionCompleted)) // same chain, never starts
	_ = d.Submit(event("repo/job/2", github.ActionQueued))    // waits for the only worker slot

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := d.Shutdown(ctx)
	if !errors.Is(err, ErrAbandoned) {
		t.Fatalf("Shutdown() error = %v, want ErrAbandoned", err)
	}
	if got, _ := results.Load("repo/job/1/queued"); !errors.Is(got.(error), context.Canceled) {
		t.Fatalf("running event result = %v, want context.Canceled", got)
	}
	for _, id := range []string{"repo/job/1/completed", "repo/job/2/queued"} {
		got, ok := results.Load(id)
		if !ok || !errors.Is(got.(error), ErrAbandoned) {
			t.Fatalf("%s result = %v (reported=%v), want ErrAbandoned", id, got, ok)
		}
	}
}

func TestAPanickingHandlerDoesNotStopLaterEvents(t *testing.T) {
	var after atomic.Bool
	var panicErr atomic.Value
	d, _ := New(func(_ context.Context, e github.WorkflowJobEvent) error {
		if e.EventID == "repo/job/1/queued" {
			panic("boom")
		}
		after.Store(true)
		return nil
	}, Config{Workers: 1, OnResult: func(r Result) {
		if r.Err != nil {
			panicErr.Store(r.Err.Error())
		}
	}})
	_ = d.Submit(event("repo/job/1", github.ActionQueued))
	_ = d.Submit(event("repo/job/1", github.ActionCompleted))
	shutdown(t, d)
	if !after.Load() {
		t.Fatal("event after the panic was not handled")
	}
	if message, _ := panicErr.Load().(string); message == "" || !contains(message, "boom") {
		t.Fatalf("panic was not reported as an error: %q", message)
	}
}

func TestEachEventGetsItsOwnTimeoutIndependentOfAnyRequest(t *testing.T) {
	var gotDeadline atomic.Bool
	var timedOut atomic.Bool
	d, _ := New(func(ctx context.Context, _ github.WorkflowJobEvent) error {
		_, has := ctx.Deadline()
		gotDeadline.Store(has)
		<-ctx.Done()
		timedOut.Store(errors.Is(ctx.Err(), context.DeadlineExceeded))
		return ctx.Err()
	}, Config{EventTimeout: 40 * time.Millisecond})
	_ = d.Submit(event("repo/job/1", github.ActionQueued))
	shutdown(t, d)
	if !gotDeadline.Load() || !timedOut.Load() {
		t.Fatalf("deadline=%v timedOut=%v", gotDeadline.Load(), timedOut.Load())
	}
}

func TestNewRequiresAHandler(t *testing.T) {
	if _, err := New(nil, Config{}); err == nil {
		t.Fatal("New(nil) unexpectedly succeeded")
	}
}

func TestHeavyMixedLoadKeepsPerJobOrder(t *testing.T) {
	const jobs = 60
	var mu sync.Mutex
	seen := map[string][]string{}
	d, _ := New(func(_ context.Context, e github.WorkflowJobEvent) error {
		mu.Lock()
		seen[e.JobKey] = append(seen[e.JobKey], string(e.Action))
		mu.Unlock()
		return nil
	}, Config{Workers: 6, QueueSize: jobs * 3})
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("repo/job/%d", i)
			for _, action := range []github.Action{github.ActionQueued, github.ActionInProgress, github.ActionCompleted} {
				if err := d.Submit(event(key, action)); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	shutdown(t, d)
	for key, actions := range seen {
		if fmt.Sprint(actions) != "[queued in_progress completed]" {
			t.Fatalf("%s order = %v", key, actions)
		}
	}
	if len(seen) != jobs {
		t.Fatalf("handled %d jobs, want %d", len(seen), jobs)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
