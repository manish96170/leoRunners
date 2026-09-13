// Package observability contains backend-neutral lifecycle events, logs, and
// metrics for the controller.
package observability

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidEvent  = errors.New("invalid observability event")
	ErrInvalidMetric = errors.New("invalid observability metric")
)

type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// Correlation identifies one request and its related controller resources.
// It intentionally contains operational identity, not credentials or payloads.
type Correlation struct {
	RequestID    string `json:"request_id,omitempty"`
	DeliveryID   string `json:"delivery_id,omitempty"`
	JobID        string `json:"job_id,omitempty"`
	RunnerID     string `json:"runner_id,omitempty"`
	LeaseID      string `json:"lease_id,omitempty"`
	Repository   string `json:"repository,omitempty"`
	Workflow     string `json:"workflow,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	Attempt      string `json:"attempt,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Region       string `json:"region,omitempty"`
	CapacityPool string `json:"capacity_pool,omitempty"`
}

// LogEvent is the common structured log envelope. Fields are redacted by the
// JSON logger before they cross the process boundary.
type LogEvent struct {
	Timestamp   time.Time      `json:"timestamp"`
	Level       LogLevel       `json:"level"`
	Message     string         `json:"message"`
	EventType   string         `json:"event_type,omitempty"`
	Correlation Correlation    `json:"correlation,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

// LifecycleEvent describes a state transition or a lifecycle operation.
type LifecycleEvent struct {
	EventID     string            `json:"event_id,omitempty"`
	EventType   string            `json:"event_type"`
	Timestamp   time.Time         `json:"timestamp"`
	Correlation Correlation       `json:"correlation"`
	FromState   string            `json:"from_state,omitempty"`
	ToState     string            `json:"to_state,omitempty"`
	Outcome     string            `json:"outcome,omitempty"`
	ErrorClass  string            `json:"error_class,omitempty"`
	Duration    time.Duration     `json:"duration,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

func (e LifecycleEvent) Validate() error {
	if strings.TrimSpace(e.EventType) == "" || e.Timestamp.IsZero() {
		return fmt.Errorf("%w: event type and timestamp are required", ErrInvalidEvent)
	}
	if e.Duration < 0 {
		return fmt.Errorf("%w: duration cannot be negative", ErrInvalidEvent)
	}
	return nil
}

type MetricKind string

const (
	MetricCounter   MetricKind = "counter"
	MetricGauge     MetricKind = "gauge"
	MetricHistogram MetricKind = "histogram"
)

type MetricUnit string

const (
	UnitCount        MetricUnit = "count"
	UnitMilliseconds MetricUnit = "milliseconds"
	UnitBytes        MetricUnit = "bytes"
	UnitRatio        MetricUnit = "ratio"
)

// Dimensions is deliberately a named type so every collector and adapter uses
// the same low-cardinality validation rules.
type Dimensions map[string]string

var allowedDimensions = map[string]struct{}{
	"provider": {}, "region": {}, "outcome": {}, "event_type": {},
	"state": {}, "capacity_owner": {}, "capacity_pool": {}, "error_class": {},
}

const (
	maxDimensions          = 8
	maxDimensionValueBytes = 128
)

func (d Dimensions) Clone() Dimensions {
	if d == nil {
		return nil
	}
	out := make(Dimensions, len(d))
	for key, value := range d {
		out[key] = value
	}
	return out
}

func (d Dimensions) Validate() error {
	if len(d) > maxDimensions {
		return fmt.Errorf("%w: at most %d dimensions are allowed", ErrInvalidMetric, maxDimensions)
	}
	for key, value := range d {
		if _, ok := allowedDimensions[key]; !ok {
			return fmt.Errorf("%w: dimension %q is not low-cardinality", ErrInvalidMetric, key)
		}
		if strings.TrimSpace(value) == "" || len(value) > maxDimensionValueBytes {
			return fmt.Errorf("%w: dimension %q has an invalid value", ErrInvalidMetric, key)
		}
	}
	return nil
}

type MetricSample struct {
	Name       string     `json:"name"`
	Kind       MetricKind `json:"kind"`
	Unit       MetricUnit `json:"unit"`
	Value      float64    `json:"value"`
	Timestamp  time.Time  `json:"timestamp"`
	Dimensions Dimensions `json:"dimensions,omitempty"`
}

func (m MetricSample) Validate() error {
	if strings.TrimSpace(m.Name) == "" || strings.ContainsAny(m.Name, " \t\n") {
		return fmt.Errorf("%w: metric name is required and cannot contain whitespace", ErrInvalidMetric)
	}
	if m.Kind != MetricCounter && m.Kind != MetricGauge && m.Kind != MetricHistogram {
		return fmt.Errorf("%w: unsupported metric kind %q", ErrInvalidMetric, m.Kind)
	}
	if m.Unit == "" {
		return fmt.Errorf("%w: metric unit is required", ErrInvalidMetric)
	}
	if m.Timestamp.IsZero() {
		return fmt.Errorf("%w: metric timestamp is required", ErrInvalidMetric)
	}
	if m.Value != m.Value || m.Value > 1.7976931348623157e308 || m.Value < -1.7976931348623157e308 {
		return fmt.Errorf("%w: metric value must be finite", ErrInvalidMetric)
	}
	return m.Dimensions.Validate()
}

type EventSink interface {
	EmitLog(LogEvent) error
	EmitLifecycle(LifecycleEvent) error
}
type MetricSink interface{ Record(MetricSample) error }
