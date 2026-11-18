// Package cache contains the backend-neutral cache contract used by the
// controller. Cache keys are opaque hashes and never contain lockfile data.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
)

type Mode string

const (
	ModeDisabled    Mode = "disabled"
	ModeObserveOnly Mode = "observe-only"
	ModeEnabled     Mode = "enabled"
)

const (
	EventHit     = "cache_hit"
	EventMiss    = "cache_miss"
	EventRestore = "cache_restore"
	EventSave    = "cache_save"
	EventDelete  = "cache_invalidate"
)

var (
	ErrInvalidKey    = errors.New("invalid cache key")
	ErrInvalidEntry  = errors.New("invalid cache entry")
	ErrEntryTooLarge = errors.New("cache entry exceeds size limit")
	ErrCacheFull     = errors.New("cache entry limit reached")
)

// KeyInput is the complete identity of a cache namespace. Do not add secrets,
// tokens, paths containing credentials, or job-specific values to this type.
type KeyInput struct {
	TenantID   string
	Namespace  string
	Repository string
	Workflow   string
	Lockfile   string
	Profile    string
}

// Key is an opaque, content-addressed cache identifier.
type Key string

func (k Key) String() string { return string(k) }

func NewKey(input KeyInput) (Key, error) {
	if input.TenantID == "" {
		input.TenantID = DefaultTenant
	}
	if input.Namespace == "" {
		input.Namespace = DefaultNamespace
	}
	if err := input.validate(); err != nil {
		return "", err
	}
	// Length-prefixing prevents ambiguous concatenations. The digest is the
	// only value that crosses a backend or telemetry boundary.
	canonical := fmt.Sprintf("%d:%s%d:%s%d:%s%d:%s%d:%s%d:%s", len(input.TenantID), input.TenantID, len(input.Namespace), input.Namespace, len(input.Repository), input.Repository, len(input.Workflow), input.Workflow, len(input.Lockfile), input.Lockfile, len(input.Profile), input.Profile)
	digest := sha256.Sum256([]byte(canonical))
	return Key("sha256:" + hex.EncodeToString(digest[:])), nil
}

func (i KeyInput) validate() error {
	if err := ValidateScope(i.TenantID, i.Namespace); err != nil {
		return err
	}
	for name, value := range map[string]string{"repository": i.Repository, "workflow": i.Workflow, "lockfile": i.Lockfile, "profile": i.Profile} {
		if value == "" || !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool { return r == '\x00' || r == '\r' || r == '\n' }) >= 0 {
			return fmt.Errorf("%w: %s is empty or contains invalid characters", ErrInvalidKey, name)
		}
	}
	if len(i.Repository) > 512 || len(i.Workflow) > 512 || len(i.Profile) > 256 || len(i.Lockfile) > 4<<20 {
		return fmt.Errorf("%w: key component is too large", ErrInvalidKey)
	}
	return nil
}

func validateKey(key Key) error {
	value := string(key)
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return ErrInvalidKey
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err != nil {
		return fmt.Errorf("%w: digest is not hexadecimal", ErrInvalidKey)
	}
	return nil
}

type Metadata struct {
	ContentType string
	TTL         time.Duration
}

type Entry struct {
	Key       Key
	Value     []byte
	Metadata  Metadata
	StoredAt  time.Time
	ExpiresAt time.Time
	Size      int64
	Checksum  string
}

// Cache is deliberately small so cloud and local backends can implement it
// without importing controller lifecycle or provider packages.
type Cache interface {
	Get(context.Context, Key) (Entry, bool, error)
	Put(context.Context, Key, []byte, Metadata) (Entry, error)
	Delete(context.Context, Key) error
}

type Event struct {
	Type      string
	Key       Key
	At        time.Time
	Hit       bool
	Size      int64
	Reason    string
	Mode      Mode
	TenantID  string
	Namespace string
}

type Config struct {
	Mode             Mode
	MaxEntryBytes    int64
	MaxEntries       int
	DefaultTTL       time.Duration
	MaxTTL           time.Duration
	OperationTimeout time.Duration
	Clock            func() time.Time
	Telemetry        telemetry.Sink
	TenantID         string
	Namespace        string
}

const (
	DefaultTenant    = "default"
	DefaultNamespace = "default"
	maxScopeLength   = 64
)

// ValidateScope protects telemetry dimensions and cache identity from
// unbounded, path-like, or ambiguous tenant and namespace values.
func ValidateScope(tenantID, namespace string) error {
	for label, value := range map[string]string{"tenant": tenantID, "namespace": namespace} {
		if value == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidEntry, label)
		}
		if len(value) > maxScopeLength || !utf8.ValidString(value) || strings.ContainsAny(value, "/\\\x00\r\n") {
			return fmt.Errorf("%w: %s must be a bounded identifier", ErrInvalidEntry, label)
		}
		for _, r := range value {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
				return fmt.Errorf("%w: %s contains unsupported characters", ErrInvalidEntry, label)
			}
		}
	}
	return nil
}

func (c Config) normalized() (Config, error) {
	if c.Mode == "" {
		c.Mode = ModeDisabled
	}
	if c.Mode != ModeDisabled && c.Mode != ModeObserveOnly && c.Mode != ModeEnabled {
		return c, fmt.Errorf("%w: unsupported cache mode %q", ErrInvalidEntry, c.Mode)
	}
	if c.MaxEntryBytes <= 0 {
		c.MaxEntryBytes = 64 << 20
	}
	if c.MaxEntries <= 0 {
		c.MaxEntries = 1000
	}
	if c.DefaultTTL < 0 || c.MaxTTL < 0 || c.OperationTimeout < 0 {
		return c, fmt.Errorf("%w: durations cannot be negative", ErrInvalidEntry)
	}
	if c.MaxTTL > 0 && c.DefaultTTL > c.MaxTTL {
		return c, fmt.Errorf("%w: default TTL exceeds maximum TTL", ErrInvalidEntry)
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	if c.TenantID == "" {
		c.TenantID = DefaultTenant
	}
	if c.Namespace == "" {
		c.Namespace = DefaultNamespace
	}
	if err := ValidateScope(c.TenantID, c.Namespace); err != nil {
		return c, err
	}
	return c, nil
}

func (c Config) context(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.OperationTimeout > 0 {
		return context.WithTimeout(ctx, c.OperationTimeout)
	}
	return context.WithCancel(ctx)
}

func emit(s telemetry.Sink, event Event) {
	if s == nil {
		return
	}
	_ = s.Emit(telemetry.Event{Type: event.Type, At: event.At, Metadata: map[string]string{"mode": string(event.Mode), "hit": fmt.Sprint(event.Hit), "size_bytes": fmt.Sprint(event.Size), "reason": event.Reason, "tenant_id": event.TenantID, "namespace": event.Namespace}})
}

// Summary is an aggregate of cache observations for one bounded tenant scope.
// It intentionally contains no cache key, repository, lockfile, or payload.
type Summary struct {
	TenantID      string  `json:"tenant_id"`
	Namespace     string  `json:"namespace"`
	Requests      uint64  `json:"requests"`
	Hits          uint64  `json:"hits"`
	Misses        uint64  `json:"misses"`
	Saves         uint64  `json:"saves"`
	Restores      uint64  `json:"restores"`
	Invalidations uint64  `json:"invalidations"`
	HitRate       float64 `json:"hit_rate"`
}

type summaryScope struct{ tenant, namespace string }

// SummaryCollector is a bounded telemetry sink suitable for real workload
// measurements. Once the scope limit is reached, new scopes are dropped.
type SummaryCollector struct {
	mu        sync.Mutex
	maxScopes int
	scopes    map[summaryScope]*Summary
	dropped   uint64
}

func NewSummaryCollector(maxScopes int) *SummaryCollector {
	if maxScopes <= 0 {
		maxScopes = 128
	}
	return &SummaryCollector{maxScopes: maxScopes, scopes: make(map[summaryScope]*Summary)}
}

func (c *SummaryCollector) Emit(event telemetry.Event) error {
	tenant, namespace := event.Metadata["tenant_id"], event.Metadata["namespace"]
	if tenant == "" {
		tenant = DefaultTenant
	}
	if namespace == "" {
		namespace = DefaultNamespace
	}
	if ValidateScope(tenant, namespace) != nil {
		c.mu.Lock()
		c.dropped++
		c.mu.Unlock()
		return nil
	}
	scope := summaryScope{tenant: tenant, namespace: namespace}
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.scopes[scope]
	if !ok {
		if len(c.scopes) >= c.maxScopes {
			c.dropped++
			return nil
		}
		s = &Summary{TenantID: tenant, Namespace: namespace}
		c.scopes[scope] = s
	}
	switch event.Type {
	case EventHit:
		s.Requests++
		s.Hits++
	case EventMiss:
		s.Requests++
		s.Misses++
	case EventSave:
		s.Saves++
	case EventRestore:
		s.Restores++
	case EventDelete:
		s.Invalidations++
	default:
		return nil
	}
	if s.Requests > 0 {
		s.HitRate = float64(s.Hits) / float64(s.Requests)
	}
	return nil
}

func (c *SummaryCollector) Snapshot() []Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Summary, 0, len(c.scopes))
	for _, s := range c.scopes {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID == out[j].TenantID {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out
}

func (c *SummaryCollector) Dropped() uint64 { c.mu.Lock(); defer c.mu.Unlock(); return c.dropped }
