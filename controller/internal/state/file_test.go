package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestFileRepositoryRestoresAllRecordsAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()
	r, err := NewFileRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CreateJob(ctx, Job{ID: "job-1", State: JobQueued, Labels: []string{"linux"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateRunner(ctx, Runner{ID: "runner-1", JobID: "job-1", State: RunnerReady, Labels: []string{"linux"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateLease(ctx, Lease{ID: "lease-1", JobID: "job-1", RunnerID: "runner-1", State: LeaseActive}); err != nil {
		t.Fatal(err)
	}
	inserted, err := r.InsertLifecycleEvent(ctx, LifecycleEvent{
		ID: "event-1", IdempotencyKey: "delivery-1", Type: "JobQueued", JobID: "job-1",
		Data: map[string]string{"attempt": "1"},
	})
	if err != nil || !inserted {
		t.Fatalf("insert event = %v, %v", inserted, err)
	}

	restarted, err := NewFileRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	job, err := restarted.GetJob(ctx, "job-1")
	if err != nil || job.Revision != 1 || len(job.Labels) != 1 {
		t.Fatalf("restored job = %#v, err=%v", job, err)
	}
	runner, err := restarted.GetRunner(ctx, "runner-1")
	if err != nil || runner.JobID != "job-1" {
		t.Fatalf("restored runner = %#v, err=%v", runner, err)
	}
	lease, err := restarted.GetLease(ctx, "lease-1")
	if err != nil || lease.RunnerID != "runner-1" {
		t.Fatalf("restored lease = %#v, err=%v", lease, err)
	}
	events, err := restarted.ListLifecycleEvents(ctx, "job-1", time.Time{})
	if err != nil || len(events) != 1 || events[0].Data["attempt"] != "1" {
		t.Fatalf("restored events = %#v, err=%v", events, err)
	}
	inserted, err = restarted.InsertLifecycleEvent(ctx, LifecycleEvent{
		ID: "event-1", IdempotencyKey: "delivery-1", Type: "JobQueued", JobID: "job-1",
		Data: map[string]string{"attempt": "1"},
	})
	if err != nil || inserted {
		t.Fatalf("restored duplicate = %v, %v", inserted, err)
	}
}

func TestFileRepositoryUsesDefensiveCopies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	r, err := NewFileRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	labels := []string{"linux"}
	data := map[string]string{"key": "value"}
	if err := r.CreateJob(ctx, Job{ID: "job-1", Labels: labels}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.InsertLifecycleEvent(ctx, LifecycleEvent{ID: "event-1", Type: "test", Data: data}); err != nil {
		t.Fatal(err)
	}
	labels[0], data["key"] = "changed", "changed"
	job, _ := r.GetJob(ctx, "job-1")
	events, _ := r.ListLifecycleEvents(ctx, "", time.Time{})
	if job.Labels[0] != "linux" || events[0].Data["key"] != "value" {
		t.Fatalf("input mutation leaked: job=%#v event=%#v", job, events[0])
	}
	job.Labels[0] = "changed"
	events[0].Data["key"] = "changed"
	jobAgain, _ := r.GetJob(ctx, "job-1")
	eventsAgain, _ := r.ListLifecycleEvents(ctx, "", time.Time{})
	if jobAgain.Labels[0] != "linux" || eventsAgain[0].Data["key"] != "value" {
		t.Fatalf("output mutation leaked: job=%#v event=%#v", jobAgain, eventsAgain[0])
	}
}

func TestFileRepositoryRecoversFromCorruptPrimary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	ctx := context.Background()
	r, err := NewFileRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CreateJob(ctx, Job{ID: "job-1", State: JobQueued}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateJob(ctx, Job{ID: "job-2", State: JobProvisioning}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"jobs":`), 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewFileRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := recovered.ListJobs(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].ID != "job-1" {
		t.Fatalf("recovered jobs = %#v, err=%v", jobs, err)
	}
	if _, err := json.Marshal(jobs); err != nil {
		t.Fatal(err)
	}
}

func TestFileRepositoryAtomicConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	r, err := NewFileRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const writers = 32
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = r.CreateJob(ctx, Job{ID: "job-" + strconv.Itoa(i), State: JobQueued})
		}(i)
	}
	wg.Wait()
	restarted, err := NewFileRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := restarted.ListJobs(ctx)
	if err != nil || len(jobs) != writers {
		t.Fatalf("concurrent jobs = %d, err=%v", len(jobs), err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Base(entry.Name()) != filepath.Base(path) && filepath.Base(entry.Name()) != filepath.Base(path)+".bak" {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestFileRepositoryRejectsInvalidRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewFileRepository(path)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid recovery error = %v", err)
	}
}
