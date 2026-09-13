package capacity

import (
	"errors"
	"sync"
	"testing"
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
