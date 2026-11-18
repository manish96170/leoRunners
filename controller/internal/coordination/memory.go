package coordination

import (
	"context"
	"sync"
	"time"
)

type memoryEntry struct {
	lease Lease
	next  uint64
}

// MemoryStore is a deterministic fake LeaseStore suitable for unit tests and
// local single-process development. The clock is injectable for expiry tests.
type MemoryStore struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[LeaseKey]memoryEntry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{now: time.Now, items: make(map[LeaseKey]memoryEntry)}
}

// NewMemoryStoreWithClock enables deterministic time advancement in tests.
func NewMemoryStoreWithClock(now func() time.Time) *MemoryStore {
	if now == nil {
		now = time.Now
	}
	return &MemoryStore{now: now, items: make(map[LeaseKey]memoryEntry)}
}

func (s *MemoryStore) Acquire(ctx context.Context, key LeaseKey, owner OwnerID, ttl time.Duration) (Lease, error) {
	if err := contextErr(ctx); err != nil {
		return Lease{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	entry, ok := s.items[key]
	if ok && now.Before(entry.lease.ExpiresAt) {
		if entry.lease.Owner == owner {
			return entry.lease, nil
		}
		return Lease{}, ErrLeaseHeld
	}

	entry.next++
	entry.lease = Lease{Key: key, Owner: owner, Token: entry.next, ExpiresAt: now.Add(ttl)}
	s.items[key] = entry
	return entry.lease, nil
}

func (s *MemoryStore) Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error) {
	if err := contextErr(ctx); err != nil {
		return Lease{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[lease.Key]
	if !ok {
		return Lease{}, ErrNotFound
	}
	if !s.now().Before(entry.lease.ExpiresAt) {
		return Lease{}, ErrNotOwner
	}
	if entry.lease.Token != lease.Token {
		return Lease{}, ErrStaleToken
	}
	if entry.lease.Owner != lease.Owner {
		return Lease{}, ErrNotOwner
	}
	entry.lease.ExpiresAt = s.now().Add(ttl)
	s.items[lease.Key] = entry
	return entry.lease, nil
}

func (s *MemoryStore) Release(ctx context.Context, lease Lease) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[lease.Key]
	if !ok {
		return nil
	}
	if entry.lease.Token != lease.Token {
		return ErrStaleToken
	}
	if entry.lease.Owner != lease.Owner {
		return ErrNotOwner
	}
	delete(s.items, lease.Key)
	return nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
