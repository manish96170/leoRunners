package cache

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
)

func testKey(t *testing.T) Key {
	t.Helper()
	key, err := NewKey(KeyInput{Repository: "acme/project", Workflow: "build", Lockfile: "sha256:abc", Profile: "linux-x64-v1"})
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestNewKeyIsOpaqueAndContentAddressed(t *testing.T) {
	a, err := NewKey(KeyInput{Repository: "a/b", Workflow: "build", Lockfile: "lock-content", Profile: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewKey(KeyInput{Repository: "a/b", Workflow: "build", Lockfile: "lock-content", Profile: "p1"})
	if err != nil || a != b {
		t.Fatalf("keys are not deterministic: %q %q %v", a, b, err)
	}
	if len(a.String()) != len("sha256:")+64 || contains(a.String(), "lock-content") {
		t.Fatalf("key exposes content: %q", a)
	}
	if _, err := NewKey(KeyInput{Repository: "a/b", Workflow: "build", Lockfile: "", Profile: "p1"}); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected invalid key, got %v", err)
	}
	aTenant, _ := NewKey(KeyInput{TenantID: "tenant-a", Namespace: "go", Repository: "a/b", Workflow: "build", Lockfile: "lock-content", Profile: "p1"})
	bTenant, _ := NewKey(KeyInput{TenantID: "tenant-b", Namespace: "go", Repository: "a/b", Workflow: "build", Lockfile: "lock-content", Profile: "p1"})
	if aTenant == bTenant {
		t.Fatal("tenant scopes must not share cache identity")
	}
	if err := ValidateScope("tenant/a", "go"); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("unsafe tenant accepted: %v", err)
	}
	if err := ValidateScope("tenant", strings.Repeat("x", 65)); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("unbounded namespace accepted: %v", err)
	}
}

func TestSummaryCollectorAggregatesBoundedWorkloadTelemetry(t *testing.T) {
	collector := NewSummaryCollector(1)
	for _, event := range []telemetry.Event{
		{Type: EventHit, Metadata: map[string]string{"tenant_id": "tenant-a", "namespace": "go"}},
		{Type: EventMiss, Metadata: map[string]string{"tenant_id": "tenant-a", "namespace": "go"}},
		{Type: EventSave, Metadata: map[string]string{"tenant_id": "tenant-a", "namespace": "go"}},
		{Type: EventRestore, Metadata: map[string]string{"tenant_id": "tenant-a", "namespace": "go"}},
		{Type: EventDelete, Metadata: map[string]string{"tenant_id": "tenant-a", "namespace": "go"}},
		{Type: EventHit, Metadata: map[string]string{"tenant_id": "tenant-b", "namespace": "go"}},
	} {
		_ = collector.Emit(event)
	}
	got := collector.Snapshot()
	if len(got) != 1 || got[0].TenantID != "tenant-a" || got[0].Hits != 1 || got[0].Misses != 1 || got[0].Requests != 2 || got[0].HitRate != 0.5 || got[0].Saves != 1 || got[0].Restores != 1 || got[0].Invalidations != 1 {
		t.Fatalf("summary = %+v", got)
	}
	if collector.Dropped() != 1 {
		t.Fatalf("dropped = %d, want 1", collector.Dropped())
	}
}

func TestMemoryEmitsRealWorkloadSummaryByScope(t *testing.T) {
	collector := NewSummaryCollector(4)
	key := testKey(t)
	c, err := NewMemory(Config{Mode: ModeEnabled, TenantID: "tenant-a", Namespace: "go", Telemetry: collector})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(context.Background(), key, []byte("payload"), Metadata{}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := c.Get(context.Background(), key); err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	got := collector.Snapshot()
	if len(got) != 1 || got[0].TenantID != "tenant-a" || got[0].Namespace != "go" || got[0].Hits != 1 || got[0].Misses != 0 || got[0].Saves != 1 || got[0].Restores != 1 {
		t.Fatalf("real workload summary = %+v", got)
	}
}

func TestMemoryEnabledLifecycleAndDefensiveCopies(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	sink := &telemetry.MemorySink{}
	c, err := NewMemory(Config{Mode: ModeEnabled, DefaultTTL: time.Hour, MaxEntryBytes: 10, MaxEntries: 2, Clock: func() time.Time { return now }, Telemetry: sink})
	if err != nil {
		t.Fatal(err)
	}
	key := testKey(t)
	value := []byte("payload")
	entry, err := c.Put(context.Background(), key, value, Metadata{ContentType: "application/octet-stream"})
	if err != nil {
		t.Fatal(err)
	}
	value[0] = 'X'
	got, ok, err := c.Get(context.Background(), key)
	if err != nil || !ok || string(got.Value) != "payload" {
		t.Fatalf("get: %+v %v %v", got, ok, err)
	}
	got.Value[0] = 'Y'
	gotAgain, _, _ := c.Get(context.Background(), key)
	if string(gotAgain.Value) != "payload" || entry.Checksum == "" {
		t.Fatalf("entry was not protected: %+v", gotAgain)
	}
	if err := c.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(context.Background(), key); ok {
		t.Fatal("deleted entry was returned")
	}
	events := sink.Snapshot()
	if len(events) != 7 || events[0].Type != EventSave || events[1].Type != EventHit || events[2].Type != EventRestore || events[3].Type != EventHit || events[4].Type != EventRestore || events[5].Type != EventDelete || events[6].Type != EventMiss {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestMemoryModesAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	key := testKey(t)
	for _, mode := range []Mode{ModeDisabled, ModeObserveOnly} {
		c, err := NewMemory(Config{Mode: mode, DefaultTTL: time.Minute, Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Put(context.Background(), key, []byte("x"), Metadata{}); err != nil {
			t.Fatal(err)
		}
		if c.Len() != 0 {
			t.Fatalf("mode %s wrote entries", mode)
		}
	}
	var current = now
	c, err := NewMemory(Config{Mode: ModeEnabled, DefaultTTL: time.Minute, Clock: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(context.Background(), key, []byte("x"), Metadata{}); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	if _, ok, err := c.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("expired entry: ok=%v err=%v", ok, err)
	}
}

func TestMemoryLimitsCancellationAndConcurrency(t *testing.T) {
	key := testKey(t)
	c, err := NewMemory(Config{Mode: ModeEnabled, MaxEntryBytes: 2, MaxEntries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(context.Background(), key, []byte("too-long"), Metadata{}); !errors.Is(err, ErrEntryTooLarge) {
		t.Fatalf("size error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.Get(ctx, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("get cancellation: %v", err)
	}
	if _, err := c.Put(context.Background(), key, []byte("ok"), Metadata{}); err != nil {
		t.Fatal(err)
	}
	other, _ := NewKey(KeyInput{Repository: "a/b", Workflow: "test", Lockfile: "l", Profile: "p"})
	if _, err := c.Put(context.Background(), other, []byte("ok"), Metadata{}); !errors.Is(err, ErrCacheFull) {
		t.Fatalf("capacity error: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, _ = c.Get(context.Background(), key) }()
	}
	wg.Wait()
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
