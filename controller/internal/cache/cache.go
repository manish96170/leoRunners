// Package cache contains the backend-neutral cache contract used by the
// controller. Cache keys are opaque hashes and never contain lockfile data.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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
	Repository string
	Workflow   string
	Lockfile   string
	Profile    string
}

// Key is an opaque, content-addressed cache identifier.
type Key string

func (k Key) String() string { return string(k) }

func NewKey(input KeyInput) (Key, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	// Length-prefixing prevents ambiguous concatenations. The digest is the
	// only value that crosses a backend or telemetry boundary.
	canonical := fmt.Sprintf("%d:%s%d:%s%d:%s%d:%s", len(input.Repository), input.Repository, len(input.Workflow), input.Workflow, len(input.Lockfile), input.Lockfile, len(input.Profile), input.Profile)
	digest := sha256.Sum256([]byte(canonical))
	return Key("sha256:" + hex.EncodeToString(digest[:])), nil
}

func (i KeyInput) validate() error {
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
	Type   string
	Key    Key
	At     time.Time
	Hit    bool
	Size   int64
	Reason string
	Mode   Mode
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
	_ = s.Emit(telemetry.Event{Type: event.Type, At: event.At, Metadata: map[string]string{"cache_key": event.Key.String(), "mode": string(event.Mode), "hit": fmt.Sprint(event.Hit), "size_bytes": fmt.Sprint(event.Size), "reason": event.Reason}})
}
