// Package cloudwatch serializes low-cardinality metrics as CloudWatch
// Embedded Metric Format (EMF) log events. It does not contact AWS.
package cloudwatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
)

const (
	maxMetrics     = 100
	maxDimensions  = 30
	maxNameLength  = 255
	maxValueLength = 1024
)

// Metric is one numeric CloudWatch metric in an EMF event.
type Metric struct {
	Name              string
	Value             float64
	Unit              string
	StorageResolution int
}

// MetricSet is a single EMF log event. Dimensions are checked against the
// allowlist configured on Exporter; arbitrary event metadata is not emitted.
type MetricSet struct {
	Timestamp  time.Time
	Dimensions map[string]string
	Metrics    []Metric
}

// Config controls serialization and the optional output sink. Dimensions are
// fixed, low-cardinality values such as ServiceName, Environment, and
// Provider. DimensionAllowlist must contain every dimension key that may be
// emitted, including keys in MetricSet.Dimensions.
type Config struct {
	Namespace          string
	Dimensions         map[string]string
	DimensionAllowlist []string
	Writer             io.Writer
	Now                func() time.Time
}

// Exporter writes EMF events. A nil Writer is valid for callers that only use
// Marshal or Write.
type Exporter struct {
	namespace  string
	dimensions map[string]string
	allowed    map[string]struct{}
	writer     io.Writer
	now        func() time.Time
	mu         sync.Mutex
}

// New validates the namespace and static dimensions without making any AWS
// calls.
func New(cfg Config) (*Exporter, error) {
	if err := validateNamespace(cfg.Namespace); err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(cfg.DimensionAllowlist))
	for _, key := range cfg.DimensionAllowlist {
		if err := validateDimensionKey(key); err != nil {
			return nil, err
		}
		allowed[key] = struct{}{}
	}
	dimensions := cloneStrings(cfg.Dimensions)
	if len(dimensions) > maxDimensions {
		return nil, fmt.Errorf("cloudwatch: at most %d dimensions are allowed", maxDimensions)
	}
	for key, value := range dimensions {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("cloudwatch: dimension %q is not allowlisted", key)
		}
		if err := validateDimension(key, value); err != nil {
			return nil, err
		}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Exporter{namespace: cfg.Namespace, dimensions: dimensions, allowed: allowed, writer: cfg.Writer, now: now}, nil
}

// Marshal returns one EMF JSON event terminated by no newline.
func (e *Exporter) Marshal(set MetricSet) ([]byte, error) {
	if e == nil {
		return nil, errors.New("cloudwatch: nil exporter")
	}
	timestamp := set.Timestamp
	if timestamp.IsZero() {
		timestamp = e.now()
	}
	if timestamp.IsZero() || timestamp.UnixMilli() < 0 {
		return nil, errors.New("cloudwatch: timestamp must be a valid non-negative time")
	}
	if len(set.Metrics) == 0 || len(set.Metrics) > maxMetrics {
		return nil, fmt.Errorf("cloudwatch: metrics must contain 1-%d items", maxMetrics)
	}
	dimensions := cloneStrings(e.dimensions)
	for key, value := range set.Dimensions {
		if _, ok := e.allowed[key]; !ok {
			return nil, fmt.Errorf("cloudwatch: dimension %q is not allowlisted", key)
		}
		if _, exists := dimensions[key]; exists {
			return nil, fmt.Errorf("cloudwatch: duplicate dimension %q", key)
		}
		if err := validateDimension(key, value); err != nil {
			return nil, err
		}
		dimensions[key] = value
	}
	if len(dimensions) > maxDimensions {
		return nil, fmt.Errorf("cloudwatch: at most %d dimensions are allowed", maxDimensions)
	}
	metricDefs := make([]metricDefinition, 0, len(set.Metrics))
	values := make(map[string]any, len(set.Metrics)+len(dimensions))
	for _, metric := range set.Metrics {
		if err := validateMetric(metric); err != nil {
			return nil, err
		}
		if _, exists := dimensions[metric.Name]; exists {
			return nil, fmt.Errorf("cloudwatch: metric %q collides with a dimension", metric.Name)
		}
		if _, exists := values[metric.Name]; exists {
			return nil, fmt.Errorf("cloudwatch: duplicate metric %q", metric.Name)
		}
		values[metric.Name] = metric.Value
		metricDefs = append(metricDefs, metricDefinition{Name: metric.Name, Unit: metric.Unit, StorageResolution: metric.StorageResolution})
	}
	for key, value := range dimensions {
		values[key] = value
	}
	aws := emfMetadata{Timestamp: timestamp.UnixMilli(), CloudWatchMetrics: []directive{{Namespace: e.namespace, Dimensions: [][]string{sortedKeys(dimensions)}, Metrics: metricDefs}}}
	root := emfDocument{AWS: aws, Values: values}
	return json.Marshal(root)
}

// Write serializes and writes one complete EMF event. A single writer call is
// used while holding the mutex so concurrent callers cannot interleave JSON.
func (e *Exporter) Write(w io.Writer, set MetricSet) error {
	if w == nil {
		return errors.New("cloudwatch: nil writer")
	}
	b, err := e.Marshal(set)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	line := append(b, '\n')
	var written int
	written, err = w.Write(line)
	if err == nil && written != len(line) {
		err = io.ErrShortWrite
	}
	return err
}

// EmitMetricSet writes to the configured Writer.
func (e *Exporter) EmitMetricSet(set MetricSet) error {
	if e.writer == nil {
		return errors.New("cloudwatch: no writer configured")
	}
	return e.Write(e.writer, set)
}

// Emit adapts the existing telemetry sink contract. Identifiers and arbitrary
// metadata are intentionally discarded; each lifecycle event becomes Count=1.
func (e *Exporter) Emit(event telemetry.Event) error {
	dimensions := map[string]string{}
	if event.Provider != "" {
		dimensions["Provider"] = event.Provider
	}
	if event.Type != "" {
		dimensions["Operation"] = event.Type
	}
	return e.EmitMetricSet(MetricSet{Timestamp: event.At, Dimensions: dimensions, Metrics: []Metric{{Name: "LifecycleEvents", Value: 1, Unit: "Count"}}})
}

type emfDocument struct {
	AWS    emfMetadata    `json:"_aws"`
	Values map[string]any `json:"-"`
}

func (d emfDocument) MarshalJSON() ([]byte, error) {
	root := make(map[string]any, len(d.Values)+1)
	root["_aws"] = d.AWS
	for key, value := range d.Values {
		root[key] = value
	}
	return json.Marshal(root)
}

type emfMetadata struct {
	Timestamp         int64       `json:"Timestamp"`
	CloudWatchMetrics []directive `json:"CloudWatchMetrics"`
}
type directive struct {
	Namespace  string             `json:"Namespace"`
	Dimensions [][]string         `json:"Dimensions"`
	Metrics    []metricDefinition `json:"Metrics"`
}
type metricDefinition struct {
	Name              string `json:"Name"`
	Unit              string `json:"Unit,omitempty"`
	StorageResolution int    `json:"StorageResolution,omitempty"`
}

func validateNamespace(namespace string) error {
	if namespace == "" || len(namespace) > 1024 || strings.HasPrefix(namespace, "AWS/") {
		return errors.New("cloudwatch: namespace must be 1-1024 characters and may not start with AWS/")
	}
	return nil
}
func validateDimensionKey(key string) error {
	if key == "" || len(key) > maxNameLength || key == "_aws" || containsSensitiveWord(key) {
		return fmt.Errorf("cloudwatch: invalid or sensitive dimension key %q", key)
	}
	return nil
}
func validateDimension(key, value string) error {
	if err := validateDimensionKey(key); err != nil {
		return err
	}
	if value == "" || len(value) > maxValueLength {
		return fmt.Errorf("cloudwatch: dimension %q value must be 1-%d characters", key, maxValueLength)
	}
	return nil
}
func validateMetric(metric Metric) error {
	if metric.Name == "" || len(metric.Name) > maxNameLength || metric.Name == "_aws" || containsSensitiveWord(metric.Name) {
		return fmt.Errorf("cloudwatch: invalid or sensitive metric name %q", metric.Name)
	}
	if math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
		return fmt.Errorf("cloudwatch: metric %q must be finite", metric.Name)
	}
	if metric.StorageResolution != 0 && metric.StorageResolution != 1 && metric.StorageResolution != 60 {
		return fmt.Errorf("cloudwatch: metric %q storage resolution must be 1 or 60", metric.Name)
	}
	return nil
}
func containsSensitiveWord(value string) bool {
	lower := strings.ToLower(value)
	for _, word := range []string{"secret", "token", "password", "credential", "authorization", "privatekey", "jitconfig"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}
func cloneStrings(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
