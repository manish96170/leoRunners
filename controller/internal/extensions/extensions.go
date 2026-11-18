// Package extensions provides an isolated execution boundary for optional
// event-driven controller extensions.
package extensions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/events"
	"github.com/leo-runners/ci-platform/controller/internal/observability"
)

const Version = "extensions.v1"

var (
	ErrInvalidExtension = errors.New("invalid extension")
	ErrDuplicate        = errors.New("extension already registered")
	ErrStarted          = errors.New("extension dispatcher already started")
	ErrClosed           = errors.New("extension dispatcher is closed")
	ErrCloseTimeout     = errors.New("extension dispatcher close timed out")
)

// Metadata is the versioned, non-secret description used to register an
// extension. Capabilities are descriptive and never grant lifecycle access.
type Metadata struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Enabled      bool     `json:"enabled"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Extension handles events independently of the controller lifecycle. Handle
// must honor ctx so shutdown and per-extension timeouts remain effective.
type Extension interface {
	Metadata() Metadata
	Handle(context.Context, events.Event) error
}

// Closer is optional. If implemented, it is called once during dispatcher
// shutdown after queued work has stopped.
type Closer interface{ Close() error }

type Registry struct {
	mu         sync.RWMutex
	started    bool
	extensions map[string]Extension
}

func NewRegistry() *Registry {
	return &Registry{extensions: make(map[string]Extension)}
}

func (r *Registry) Register(extension Extension) error {
	if extension == nil {
		return fmt.Errorf("%w: nil extension", ErrInvalidExtension)
	}
	metadata := extension.Metadata()
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrStarted
	}
	if _, exists := r.extensions[metadata.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, metadata.ID)
	}
	r.extensions[metadata.ID] = extension
	return nil
}

func (r *Registry) List() []Metadata {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metadata := make([]Metadata, 0, len(r.extensions))
	for _, extension := range r.extensions {
		metadata = append(metadata, cloneMetadata(extension.Metadata()))
	}
	for i := 1; i < len(metadata); i++ {
		for j := i; j > 0 && metadata[j].ID < metadata[j-1].ID; j-- {
			metadata[j], metadata[j-1] = metadata[j-1], metadata[j]
		}
	}
	return metadata
}

func (r *Registry) snapshot() []Extension {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil
	}
	r.started = true
	items := make([]Extension, 0, len(r.extensions))
	for _, extension := range r.extensions {
		items = append(items, extension)
	}
	return items
}

func validateMetadata(metadata Metadata) error {
	if metadata.Version == "" {
		metadata.Version = Version
	}
	if metadata.Version != Version {
		return fmt.Errorf("%w: unsupported version %q", ErrInvalidExtension, metadata.Version)
	}
	if strings.TrimSpace(metadata.ID) == "" || len(metadata.ID) > 128 {
		return fmt.Errorf("%w: id is required and bounded", ErrInvalidExtension)
	}
	if strings.TrimSpace(metadata.Name) == "" || len(metadata.Name) > 256 {
		return fmt.Errorf("%w: name is required and bounded", ErrInvalidExtension)
	}
	seen := make(map[string]struct{}, len(metadata.Capabilities))
	for _, capability := range metadata.Capabilities {
		if strings.TrimSpace(capability) == "" || len(capability) > 128 {
			return fmt.Errorf("%w: invalid capability", ErrInvalidExtension)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("%w: duplicate capability %q", ErrInvalidExtension, capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func cloneMetadata(metadata Metadata) Metadata {
	metadata.Capabilities = append([]string(nil), metadata.Capabilities...)
	return metadata
}

type Config struct {
	QueueSize    int
	Timeout      time.Duration
	CloseTimeout time.Duration
	ReportBuffer int
	Reporter     Reporter
	MetricsSink  observability.MetricSink
}

func (c *Config) setDefaults() {
	if c.QueueSize <= 0 {
		c.QueueSize = 32
	}
	if c.Timeout <= 0 {
		c.Timeout = 2 * time.Second
	}
	if c.CloseTimeout <= 0 {
		c.CloseTimeout = 5 * time.Second
	}
	if c.ReportBuffer <= 0 {
		c.ReportBuffer = 64
	}
}

// Reporter receives bounded execution results. Implementations should return
// promptly; reports are delivered by a separate goroutine.
type Reporter interface{ Report(Execution) }

type Execution struct {
	ExtensionID string        `json:"extension_id"`
	EventID     string        `json:"event_id"`
	StartedAt   time.Time     `json:"started_at"`
	Duration    time.Duration `json:"duration"`
	Outcome     string        `json:"outcome"`
	Error       string        `json:"error,omitempty"`
}

type Counters struct {
	Received  uint64 `json:"received"`
	Queued    uint64 `json:"queued"`
	Dropped   uint64 `json:"dropped"`
	Succeeded uint64 `json:"succeeded"`
	Failed    uint64 `json:"failed"`
	TimedOut  uint64 `json:"timed_out"`
	Panicked  uint64 `json:"panicked"`
}

type Metrics struct {
	ByExtension    map[string]Counters `json:"by_extension"`
	ReportsDropped uint64              `json:"reports_dropped"`
}

type counter struct {
	received, queued, dropped, succeeded, failed, timedOut, panicked atomic.Uint64
}

func (c *counter) snapshot() Counters {
	return Counters{Received: c.received.Load(), Queued: c.queued.Load(), Dropped: c.dropped.Load(), Succeeded: c.succeeded.Load(), Failed: c.failed.Load(), TimedOut: c.timedOut.Load(), Panicked: c.panicked.Load()}
}
