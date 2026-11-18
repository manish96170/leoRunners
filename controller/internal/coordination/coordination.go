// Package coordination provides fenced, TTL-based leadership leases for
// multiple controller instances.
package coordination

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidOwner = errors.New("coordination: owner ID is required")
	ErrInvalidKey   = errors.New("coordination: lease key is required")
	ErrInvalidTTL   = errors.New("coordination: TTL must be positive")
	ErrLeaseHeld    = errors.New("coordination: lease is held by another owner")
	ErrNotFound     = errors.New("coordination: lease does not exist")
	ErrNotOwner     = errors.New("coordination: lease is not owned by this owner")
	ErrStaleToken   = errors.New("coordination: fencing token is stale")
)

// OwnerID identifies one controller process. It must be stable for the
// process lifetime and unique among concurrently running controllers.
type OwnerID string

// LeaseKey identifies a mutually exclusive coordination resource.
type LeaseKey string

// Lease is a snapshot of a held lease. Token must be attached to every
// operation guarded by the lease so consumers can reject stale work.
type Lease struct {
	Key       LeaseKey
	Owner     OwnerID
	Token     uint64
	ExpiresAt time.Time
}

// LeaseStore is the persistence boundary for coordination. Implementations
// must make the owner/token checks and token allocation atomic.
type LeaseStore interface {
	Acquire(ctx context.Context, key LeaseKey, owner OwnerID, ttl time.Duration) (Lease, error)
	Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error)
	Release(ctx context.Context, lease Lease) error
}

// Coordinator validates inputs and delegates atomic lease operations to an
// injected store. It is deliberately free of cloud/database dependencies.
type Coordinator struct {
	store LeaseStore
}

func New(store LeaseStore) (*Coordinator, error) {
	if store == nil {
		return nil, errors.New("coordination: lease store is required")
	}
	return &Coordinator{store: store}, nil
}

func (c *Coordinator) Acquire(ctx context.Context, key LeaseKey, owner OwnerID, ttl time.Duration) (Lease, error) {
	if err := validateContext(ctx); err != nil {
		return Lease{}, err
	}
	if err := validate(key, owner, ttl); err != nil {
		return Lease{}, err
	}
	return c.store.Acquire(ctx, key, owner, ttl)
}

func (c *Coordinator) Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error) {
	if err := validateContext(ctx); err != nil {
		return Lease{}, err
	}
	if err := validate(lease.Key, lease.Owner, ttl); err != nil {
		return Lease{}, err
	}
	if lease.Token == 0 {
		return Lease{}, ErrStaleToken
	}
	return c.store.Renew(ctx, lease, ttl)
}

func (c *Coordinator) Release(ctx context.Context, lease Lease) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(string(lease.Key)) == "" {
		return ErrInvalidKey
	}
	if strings.TrimSpace(string(lease.Owner)) == "" {
		return ErrInvalidOwner
	}
	if lease.Token == 0 {
		return ErrStaleToken
	}
	return c.store.Release(ctx, lease)
}

func validate(key LeaseKey, owner OwnerID, ttl time.Duration) error {
	if strings.TrimSpace(string(key)) == "" {
		return ErrInvalidKey
	}
	if strings.TrimSpace(string(owner)) == "" {
		return ErrInvalidOwner
	}
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	return nil
}

func validateContext(ctx context.Context) error {
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
