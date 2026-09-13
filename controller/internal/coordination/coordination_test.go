package coordination

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTwoControllersOnlyOneAcquires(t *testing.T) {
	store := NewMemoryStore()
	coord, err := New(store)
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		lease Lease
		err   error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, owner := range []OwnerID{"controller-a", "controller-b"} {
		wg.Add(1)
		go func(owner OwnerID) {
			defer wg.Done()
			lease, err := coord.Acquire(context.Background(), "scheduler", owner, time.Minute)
			results <- result{lease: lease, err: err}
		}(owner)
	}
	wg.Wait()
	close(results)

	var acquired int
	for result := range results {
		if result.err == nil {
			acquired++
			if result.lease.Token != 1 {
				t.Fatalf("first token = %d, want 1", result.lease.Token)
			}
		} else if !errors.Is(result.err, ErrLeaseHeld) {
			t.Fatalf("unexpected contention error: %v", result.err)
		}
	}
	if acquired != 1 {
		t.Fatalf("acquired by %d controllers, want 1", acquired)
	}
}

func TestExpiredLeaseCanBeTakenOverWithNewFencingToken(t *testing.T) {
	clock := newTestClock(time.Unix(100, 0))
	store := NewMemoryStoreWithClock(clock.Now)
	coord, _ := New(store)

	first, err := coord.Acquire(context.Background(), "reaper", "controller-a", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(11 * time.Second)
	second, err := coord.Acquire(context.Background(), "reaper", "controller-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token <= first.Token {
		t.Fatalf("takeover token %d did not advance past %d", second.Token, first.Token)
	}
	if _, err := coord.Renew(context.Background(), first, time.Minute); !errors.Is(err, ErrStaleToken) && !errors.Is(err, ErrNotOwner) {
		t.Fatalf("stale renew error = %v, want stale-token or not-owner", err)
	}
	if err := coord.Release(context.Background(), first); !errors.Is(err, ErrStaleToken) {
		t.Fatalf("stale release error = %v, want stale-token", err)
	}
	if err := coord.Release(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if err := coord.Release(context.Background(), second); err != nil {
		t.Fatalf("release should be idempotent: %v", err)
	}
}

func TestRenewKeepsFencingTokenAndExtendsTTL(t *testing.T) {
	clock := newTestClock(time.Unix(200, 0))
	store := NewMemoryStoreWithClock(clock.Now)
	coord, _ := New(store)
	lease, err := coord.Acquire(context.Background(), "worker", "controller-a", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(5 * time.Second)
	renewed, err := coord.Renew(context.Background(), lease, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Token != lease.Token {
		t.Fatalf("renew changed token from %d to %d", lease.Token, renewed.Token)
	}
	clock.Advance(15 * time.Second)
	if _, err := coord.Acquire(context.Background(), "worker", "controller-b", time.Second); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("renewed lease was not held: %v", err)
	}
}

func TestAcquireByCurrentOwnerIsIdempotent(t *testing.T) {
	clock := newTestClock(time.Unix(300, 0))
	store := NewMemoryStoreWithClock(clock.Now)
	coord, _ := New(store)
	first, err := coord.Acquire(context.Background(), "worker", "controller-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	second, err := coord.Acquire(context.Background(), "worker", "controller-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("idempotent acquire changed lease: first=%+v second=%+v", first, second)
	}
}

func TestCancellationAndDeadlineArePassedThrough(t *testing.T) {
	store := NewMemoryStore()
	coord, _ := New(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coord.Acquire(ctx, "key", "owner", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire error = %v, want context canceled", err)
	}
	deadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := coord.Release(deadline, Lease{Key: "key", Owner: "owner", Token: 1}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("release error = %v, want deadline exceeded", err)
	}
}

func TestValidation(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) should fail")
	}
	store := NewMemoryStore()
	coord, _ := New(store)
	if _, err := coord.Acquire(context.Background(), "", "owner", time.Second); !errors.Is(err, ErrInvalidKey) {
		t.Fatal(err)
	}
	if _, err := coord.Acquire(context.Background(), "key", "", time.Second); !errors.Is(err, ErrInvalidOwner) {
		t.Fatal(err)
	}
	if _, err := coord.Acquire(context.Background(), "key", "owner", 0); !errors.Is(err, ErrInvalidTTL) {
		t.Fatal(err)
	}
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(now time.Time) *testClock { return &testClock{now: now} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}
