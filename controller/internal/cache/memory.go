package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

type Memory struct {
	mu      sync.Mutex
	config  Config
	entries map[Key]Entry
}

var _ Cache = (*Memory)(nil)

func NewMemory(config Config) (*Memory, error) {
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	return &Memory{config: normalized, entries: make(map[Key]Entry)}, nil
}

func (m *Memory) Get(ctx context.Context, key Key) (Entry, bool, error) {
	if err := validateKey(key); err != nil {
		return Entry{}, false, err
	}
	ctx, cancel := m.config.context(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Entry{}, false, err
	}
	now := m.config.Clock().UTC()
	m.mu.Lock()
	entry, ok := m.entries[key]
	if ok && !entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt) {
		delete(m.entries, key)
		ok = false
	}
	if ok {
		entry = cloneEntry(entry)
	}
	m.mu.Unlock()

	if m.config.Mode != ModeEnabled {
		emit(m.config.Telemetry, Event{Type: EventMiss, Key: key, At: now, Mode: m.config.Mode, Reason: "cache_not_served", TenantID: m.config.TenantID, Namespace: m.config.Namespace})
		return Entry{}, false, nil
	}
	if !ok {
		emit(m.config.Telemetry, Event{Type: EventMiss, Key: key, At: now, Mode: m.config.Mode, Reason: "not_found_or_expired", TenantID: m.config.TenantID, Namespace: m.config.Namespace})
		return Entry{}, false, nil
	}
	emit(m.config.Telemetry, Event{Type: EventHit, Key: key, At: now, Mode: m.config.Mode, Hit: true, Size: entry.Size, TenantID: m.config.TenantID, Namespace: m.config.Namespace})
	emit(m.config.Telemetry, Event{Type: EventRestore, Key: key, At: now, Mode: m.config.Mode, Hit: true, Size: entry.Size, TenantID: m.config.TenantID, Namespace: m.config.Namespace})
	return entry, true, nil
}

func (m *Memory) Put(ctx context.Context, key Key, value []byte, metadata Metadata) (Entry, error) {
	if err := validateKey(key); err != nil {
		return Entry{}, err
	}
	if int64(len(value)) > m.config.MaxEntryBytes {
		return Entry{}, fmt.Errorf("%w: got %d bytes, limit is %d", ErrEntryTooLarge, len(value), m.config.MaxEntryBytes)
	}
	if metadata.TTL < 0 || (m.config.MaxTTL > 0 && metadata.TTL > m.config.MaxTTL) {
		return Entry{}, fmt.Errorf("%w: invalid TTL", ErrInvalidEntry)
	}
	ctx, cancel := m.config.context(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	now := m.config.Clock().UTC()
	ttl := metadata.TTL
	if ttl == 0 {
		ttl = m.config.DefaultTTL
	}
	entry := Entry{Key: key, Value: append([]byte(nil), value...), Metadata: metadata, StoredAt: now, Size: int64(len(value))}
	if ttl > 0 {
		entry.ExpiresAt = now.Add(ttl)
	}
	sum := sha256.Sum256(value)
	entry.Checksum = hex.EncodeToString(sum[:])

	if m.config.Mode == ModeEnabled {
		m.mu.Lock()
		if _, exists := m.entries[key]; !exists && len(m.entries) >= m.config.MaxEntries {
			m.mu.Unlock()
			return Entry{}, ErrCacheFull
		}
		m.entries[key] = cloneEntry(entry)
		m.mu.Unlock()
		emit(m.config.Telemetry, Event{Type: EventSave, Key: key, At: now, Mode: m.config.Mode, Size: entry.Size, TenantID: m.config.TenantID, Namespace: m.config.Namespace})
		return cloneEntry(entry), nil
	}
	emit(m.config.Telemetry, Event{Type: EventSave, Key: key, At: now, Mode: m.config.Mode, Size: entry.Size, Reason: "cache_not_written", TenantID: m.config.TenantID, Namespace: m.config.Namespace})
	return entry, nil
}

func (m *Memory) Delete(ctx context.Context, key Key) error {
	if err := validateKey(key); err != nil {
		return err
	}
	ctx, cancel := m.config.context(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
	emit(m.config.Telemetry, Event{Type: EventDelete, Key: key, At: m.config.Clock().UTC(), Mode: m.config.Mode, Reason: "explicit_invalidation", TenantID: m.config.TenantID, Namespace: m.config.Namespace})
	return nil
}

// InvalidateAll is useful for profile or repository rollouts. It is not part
// of Cache because remote backends may need a namespace-specific operation.
func (m *Memory) InvalidateAll(ctx context.Context) error {
	ctx, cancel := m.config.context(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	m.entries = make(map[Key]Entry)
	m.mu.Unlock()
	return nil
}

func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func cloneEntry(entry Entry) Entry {
	entry.Value = append([]byte(nil), entry.Value...)
	return entry
}
