package state

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestEventIdempotencyAndExpiredJobQuery(t *testing.T) {
	r := NewMemoryRepository()
	e := LifecycleEvent{ID: "event-1", IdempotencyKey: "delivery-1", Type: "JobQueued"}
	inserted, err := r.InsertLifecycleEvent(context.Background(), e)
	if err != nil || !inserted {
		t.Fatalf("first event insert = %v, %v", inserted, err)
	}
	inserted, err = r.InsertLifecycleEvent(context.Background(), e)
	if err != nil || inserted {
		t.Fatalf("duplicate event insert = %v, %v", inserted, err)
	}
	now := time.Now().UTC()
	if err := r.CreateJob(context.Background(), Job{ID: "job-1", State: JobQueued, ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	jobs, err := r.ListJobs(context.Background(), JobQueued)
	if err != nil || len(jobs) != 1 || jobs[0].ID != "job-1" {
		t.Fatalf("queued jobs = %#v, err=%v", jobs, err)
	}
}

func TestConditionalUpdateRejectsStaleRevision(t *testing.T) {
	r := NewMemoryRepository()
	ctx := context.Background()
	if err := r.CreateJob(ctx, Job{ID: "job-1", State: JobQueued}); err != nil {
		t.Fatal(err)
	}
	j, err := r.GetJob(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	j.State = JobRunning
	saved, err := r.SaveJob(ctx, j, 1)
	if err != nil || saved.Revision != 2 {
		t.Fatalf("save = %#v, err=%v", saved, err)
	}
	if _, err := r.SaveJob(ctx, j, 1); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale update error = %v", err)
	}
}

func TestExpiryQueriesExcludeTerminalRecords(t *testing.T) {
	r := NewMemoryRepository()
	ctx := context.Background()
	now := time.Unix(100, 0)
	if err := r.CreateJob(ctx, Job{ID: "expired-job", State: JobProvisioning, ExpiresAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateLease(ctx, Lease{ID: "expired-lease", State: LeaseActive, ExpiresAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateRunner(ctx, Runner{ID: "expired-runner", State: RunnerReady, ExpiresAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateRunner(ctx, Runner{ID: "done-runner", State: RunnerTerminated, ExpiresAt: now}); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ListExpiredJobs(ctx, now); err != nil || len(got) != 1 {
		t.Fatalf("jobs = %#v, err=%v", got, err)
	}
	if got, err := r.ListExpiredLeases(ctx, now); err != nil || len(got) != 1 {
		t.Fatalf("leases = %#v, err=%v", got, err)
	}
	if got, err := r.ListExpiredRunners(ctx, now); err != nil || len(got) != 1 {
		t.Fatalf("runners = %#v, err=%v", got, err)
	}
}

func TestConcurrentEventInsertionStoresOneEvent(t *testing.T) {
	r := NewMemoryRepository()
	e := LifecycleEvent{ID: "event-1", IdempotencyKey: "delivery-1", Type: "JobQueued"}
	const workers = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	inserted := 0
	failures := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := r.InsertLifecycleEvent(context.Background(), e)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures++
			}
			if ok {
				inserted++
			}
		}()
	}
	wg.Wait()
	if failures != 0 || inserted != 1 {
		t.Fatalf("inserted=%d failures=%d", inserted, failures)
	}
	if got, err := r.ListLifecycleEvents(context.Background(), "", time.Time{}); err != nil || len(got) != 1 {
		t.Fatalf("events = %#v, err=%v", got, err)
	}
}
