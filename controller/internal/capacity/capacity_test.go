package capacity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/coordination"
)

func testPool(id string, ownership Ownership) Pool {
	return Pool{ID: id, Ownership: ownership, Provider: "aws", Region: "us-east-1", SecurityProfile: "isolated", Availability: Available, Labels: []string{"linux", "x64"}, Capacity: Capacity{MaxRunners: 2, MaxCPU: 8, MaxMemoryGB: 16}, Startup: Startup{ExpectedSeconds: 20, P95Seconds: 40}, Pricing: Pricing{Currency: "USD", PerRunnerHour: 0.1}}
}

func TestRegisterValidatesAndCopies(t *testing.T) {
	r := NewRegistry()
	p := testPool("managed-a", Managed)
	p.Metadata = map[string]string{"team": "ci"}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	p.Labels[0] = "mutated"
	p.Metadata["team"] = "mutated"
	got, err := r.Get("managed-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels[0] != "linux" || got.Metadata["team"] != "ci" {
		t.Fatal("registry retained caller-owned mutable data")
	}
	if err := r.Register(testPool("managed-a", Managed)); !errors.Is(err, ErrPoolExists) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestSelectOwnershipAndDeterminism(t *testing.T) {
	r := NewRegistry()
	for _, p := range []Pool{testPool("z-managed", Managed), testPool("a-customer", CustomerOwned), testPool("b-customer", CustomerOwned)} {
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	got, ok := r.Select(Request{Mode: CustomerFirst, Labels: []string{"linux"}, CPU: 2, MemoryGB: 2})
	if !ok || got.ID != "a-customer" {
		t.Fatalf("selected = %#v, %v", got, ok)
	}
	got, ok = r.Select(Request{Mode: ManagedOnly, Labels: []string{"linux"}})
	if !ok || got.ID != "z-managed" {
		t.Fatalf("managed selected = %#v, %v", got, ok)
	}
	if _, ok := r.Select(Request{Mode: CustomerOnly, Labels: []string{"windows"}}); ok {
		t.Fatal("selected pool with missing label")
	}
}

func TestReserveReleaseIsAtomic(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(testPool("p", Managed)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	reserved := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r.Reserve("p", 1, 1, 0) == nil {
				mu.Lock()
				reserved++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if reserved != 2 {
		t.Fatalf("reserved %d runners, want 2", reserved)
	}
	p, _ := r.Get("p")
	if p.Capacity.UsedRunners != 2 || p.Capacity.UsedCPU != 2 {
		t.Fatalf("usage = %#v", p.Capacity)
	}
	if err := r.Release("p", 1, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := r.Release("p", 2, 1, 0); !errors.Is(err, ErrReservation) {
		t.Fatalf("over-release error = %v", err)
	}
}

func TestInvalidPool(t *testing.T) {
	for _, p := range []Pool{{}, testPool("p", Managed)} {
		if p.ID == "p" {
			p.Capacity.UsedRunners = p.Capacity.MaxRunners + 1
		}
		if err := p.Validate(); !errors.Is(err, ErrInvalidPool) {
			t.Fatalf("validation error = %v", err)
		}
	}
}

func TestTwoReplicaFailoverRejectsStaleOwnerAndConservesCapacity(t *testing.T) {
	clock := newCapacityTestClock(time.Unix(500, 0))
	leases := coordination.NewMemoryStoreWithClock(clock.Now)
	coordinator, err := coordination.New(leases)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	pool := testPool("shared", Managed)
	pool.Capacity.MaxRunners = 3
	if err := registry.Register(pool); err != nil {
		t.Fatal(err)
	}

	firstLease, err := coordinator.Acquire(context.Background(), "capacity/shared", "controller-a", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewReplica(registry, string(firstLease.Owner), firstLease.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ReserveFenced("job-a", "shared", 2, 4, 0); err != nil {
		t.Fatal(err)
	}
	if err := first.ReserveFenced("job-a", "shared", 2, 4, 0); err != nil {
		t.Fatalf("retry was not idempotent: %v", err)
	}

	clock.Advance(11 * time.Second)
	secondLease, err := coordinator.Acquire(context.Background(), "capacity/shared", "controller-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewReplica(registry, string(secondLease.Owner), secondLease.Token)
	if err != nil {
		t.Fatal(err)
	}
	if secondLease.Token <= firstLease.Token {
		t.Fatalf("failover token %d did not advance past %d", secondLease.Token, firstLease.Token)
	}

	if err := first.ReserveFenced("job-stale", "shared", 1, 1, 0); !errors.Is(err, ErrStaleReplica) {
		t.Fatalf("stale reserve error = %v, want ErrStaleReplica", err)
	}
	if err := first.ReleaseFenced("job-a"); !errors.Is(err, ErrStaleReplica) {
		t.Fatalf("stale release error = %v, want ErrStaleReplica", err)
	}
	if err := second.ReserveFenced("job-b", "shared", 1, 2, 0); err != nil {
		t.Fatal(err)
	}
	if err := second.ReserveFenced("job-c", "shared", 1, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := second.ReserveFenced("job-d", "shared", 1, 1, 0); !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("over-capacity reserve error = %v, want ErrNoCapacity", err)
	}

	evidence := registry.Evidence()
	if evidence.ActiveOwner != "controller-b" || evidence.ActiveToken != secondLease.Token || len(evidence.Reservations) != 3 {
		t.Fatalf("recovery evidence = %+v", evidence)
	}
	if evidence.Reservations[0].ID != "job-a" || evidence.Reservations[1].ID != "job-b" || evidence.Reservations[2].ID != "job-c" {
		t.Fatalf("reservations are not deterministic: %+v", evidence.Reservations)
	}
	got, err := registry.Get("shared")
	if err != nil {
		t.Fatal(err)
	}
	if got.Capacity.UsedRunners != 3 || got.Capacity.UsedCPU != 4 || got.Capacity.UsedMemoryGB != 7 {
		t.Fatalf("capacity conservation failed: %+v", got.Capacity)
	}
	if err := second.ReleaseFenced("job-a"); err != nil {
		t.Fatal(err)
	}
	if err := second.ReleaseFenced("job-b"); err != nil {
		t.Fatal(err)
	}
	if err := second.ReleaseFenced("job-c"); err != nil {
		t.Fatal(err)
	}
	got, _ = registry.Get("shared")
	if got.Capacity.UsedRunners != 0 || got.Capacity.UsedCPU != 0 || got.Capacity.UsedMemoryGB != 0 {
		t.Fatalf("capacity after partial release = %+v", got.Capacity)
	}
}

type capacityTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newCapacityTestClock(now time.Time) *capacityTestClock { return &capacityTestClock{now: now} }

func (c *capacityTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *capacityTestClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}
