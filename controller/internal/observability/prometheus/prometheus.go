// Package prometheus provides a small, dependency-free Prometheus exporter.
// It stores only aggregate metric series; callers should never use job, run,
// runner, repository, or request IDs as label values.
package prometheus

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidMetric    = errors.New("invalid prometheus metric")
	ErrCardinalityLimit = errors.New("prometheus cardinality limit exceeded")
)

type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
)

// Config controls name normalization and cardinality. AllowedLabels is an
// allowlist: labels not listed here are dropped before they can create a
// series. An empty allowlist allows no labels, which is the safest default.
type Config struct {
	Namespace      string
	AllowedLabels  map[string]struct{}
	MaxLabelValues int
	MaxSeries      int
	DefaultBuckets []float64
}

type Labels map[string]string

type Exporter struct {
	mu             sync.RWMutex
	namespace      string
	allowedLabels  map[string]struct{}
	maxLabelValues int
	maxSeries      int
	buckets        []float64
	families       map[string]*family
	labelValues    map[string]map[string]map[string]struct{}
	seriesCount    int
}

type family struct {
	name       string
	help       string
	typeName   MetricType
	labelNames []string
	series     map[string]*series
}

type series struct {
	labels  []labelPair
	counter float64
	gauge   float64
	buckets []uint64
	sum     float64
	count   uint64
}

type labelPair struct{ name, value string }

func New(config Config) *Exporter {
	allowed := make(map[string]struct{}, len(config.AllowedLabels))
	for name := range config.AllowedLabels {
		allowed[sanitizeLabelName(name)] = struct{}{}
	}
	buckets := append([]float64(nil), config.DefaultBuckets...)
	if len(buckets) == 0 {
		buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	}
	buckets = normalizeBuckets(buckets)
	maxValues := config.MaxLabelValues
	if maxValues <= 0 {
		maxValues = 100
	}
	maxSeries := config.MaxSeries
	if maxSeries <= 0 {
		maxSeries = 10000
	}
	return &Exporter{
		namespace:      sanitizeName(config.Namespace),
		allowedLabels:  allowed,
		maxLabelValues: maxValues,
		maxSeries:      maxSeries,
		buckets:        buckets,
		families:       make(map[string]*family),
		labelValues:    make(map[string]map[string]map[string]struct{}),
	}
}

// Register declares a metric family. Re-registering the same family with
// the same type and labels is idempotent; incompatible registration fails.
func (e *Exporter) Register(name string, typ MetricType, help string, labelNames ...string) error {
	name = e.fullName(name)
	if !validMetricType(typ) || name == "" {
		return fmt.Errorf("%w: name and type are required", ErrInvalidMetric)
	}
	labels, err := e.normalizeLabelNames(labelNames)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if existing, ok := e.families[name]; ok {
		if existing.typeName != typ || existing.help != help || !sameStrings(existing.labelNames, labels) {
			return fmt.Errorf("%w: family %q was registered incompatibly", ErrInvalidMetric, name)
		}
		return nil
	}
	e.families[name] = &family{name: name, help: help, typeName: typ, labelNames: labels, series: make(map[string]*series)}
	return nil
}

func (e *Exporter) AddCounter(name string, delta float64, labels Labels) error {
	if !finite(delta) || delta < 0 {
		return fmt.Errorf("%w: counter delta must be finite and non-negative", ErrInvalidMetric)
	}
	return e.update(name, Counter, labels, func(s *series) { s.counter += delta })
}

func (e *Exporter) IncCounter(name string, labels Labels) error { return e.AddCounter(name, 1, labels) }

func (e *Exporter) SetGauge(name string, value float64, labels Labels) error {
	if !finite(value) {
		return fmt.Errorf("%w: gauge value must be finite", ErrInvalidMetric)
	}
	return e.update(name, Gauge, labels, func(s *series) { s.gauge = value })
}

func (e *Exporter) AddGauge(name string, delta float64, labels Labels) error {
	if !finite(delta) {
		return fmt.Errorf("%w: gauge delta must be finite", ErrInvalidMetric)
	}
	return e.update(name, Gauge, labels, func(s *series) { s.gauge += delta })
}

func (e *Exporter) Observe(name string, value float64, labels Labels) error {
	if !finite(value) || value < 0 {
		return fmt.Errorf("%w: histogram observation must be finite and non-negative", ErrInvalidMetric)
	}
	return e.update(name, Histogram, labels, func(s *series) {
		s.sum += value
		s.count++
		for i, bound := range e.buckets {
			if value <= bound {
				s.buckets[i]++
				break
			}
		}
	})
}

// RecordLifecycle emits the stable, low-cardinality lifecycle counter. event
// and provider should be finite vocabularies rather than request identifiers.
func (e *Exporter) RecordLifecycle(event, provider string) error {
	return e.IncCounter("lifecycle_events_total", Labels{"event": event, "provider": provider})
}

func (e *Exporter) ObserveLifecycle(phase, provider string, duration time.Duration) error {
	if duration < 0 {
		return fmt.Errorf("%w: lifecycle duration cannot be negative", ErrInvalidMetric)
	}
	return e.Observe("lifecycle_duration_seconds", duration.Seconds(), Labels{"phase": phase, "provider": provider})
}

func (e *Exporter) SetActiveRunners(provider string, count int) error {
	if count < 0 {
		return fmt.Errorf("%w: active runner count cannot be negative", ErrInvalidMetric)
	}
	return e.SetGauge("active_runners", float64(count), Labels{"provider": provider})
}

func (e *Exporter) update(name string, typ MetricType, labels Labels, apply func(*series)) error {
	name = e.fullName(name)
	if name == "" {
		return fmt.Errorf("%w: metric name is required", ErrInvalidMetric)
	}
	normalized, err := normalizeLabelValues(labels)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	f, ok := e.families[name]
	created := false
	if !ok {
		f = &family{name: name, typeName: typ, labelNames: e.allowedNames(normalized), series: make(map[string]*series)}
		e.families[name] = f
		created = true
	} else if f.typeName != typ {
		return fmt.Errorf("%w: family %q has type %q, requested %q", ErrInvalidMetric, name, f.typeName, typ)
	}
	canonical, key := e.canonicalLabels(f.labelNames, normalized)
	if existing := f.series[key]; existing != nil {
		apply(existing)
		return nil
	}
	if e.seriesCount >= e.maxSeries {
		if created {
			delete(e.families, name)
		}
		return fmt.Errorf("%w: exporter allows at most %d series", ErrCardinalityLimit, e.maxSeries)
	}
	if err := e.admitLabelValues(name, canonical); err != nil {
		if created {
			delete(e.families, name)
		}
		return err
	}
	s := &series{labels: canonical}
	if typ == Histogram {
		s.buckets = make([]uint64, len(e.buckets))
	}
	apply(s)
	f.series[key] = s
	e.seriesCount++
	return nil
}

// Gather returns deterministic Prometheus text exposition. A final newline
// is always included, including when the exporter is empty.
func (e *Exporter) Gather() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.families))
	for name := range e.families {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		f := e.families[name]
		if f.help != "" {
			fmt.Fprintf(&b, "# HELP %s %s\n", f.name, sanitizeHelp(f.help))
		}
		fmt.Fprintf(&b, "# TYPE %s %s\n", f.name, f.typeName)
		keys := make([]string, 0, len(f.series))
		for key := range f.series {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			s := f.series[key]
			switch f.typeName {
			case Counter:
				writeSample(&b, f.name, s.labels, s.counter)
			case Gauge:
				writeSample(&b, f.name, s.labels, s.gauge)
			case Histogram:
				cumulative := uint64(0)
				for i, bound := range e.buckets {
					cumulative += s.buckets[i]
					writeSample(&b, f.name+"_bucket", appendLabel(s.labels, labelPair{"le", formatFloat(bound)}), float64(cumulative))
				}
				writeSample(&b, f.name+"_bucket", appendLabel(s.labels, labelPair{"le", "+Inf"}), float64(s.count))
				writeSample(&b, f.name+"_sum", s.labels, s.sum)
				writeSample(&b, f.name+"_count", s.labels, float64(s.count))
			}
		}
	}
	return b.String()
}

// ServeHTTP exposes the current snapshot using Prometheus' text exposition
// content type. Gathering is bounded by the configured series limits and does
// not perform I/O or call external services.
func (e *Exporter) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(e.Gather()))
}

func (e *Exporter) fullName(name string) string {
	name = sanitizeName(name)
	if e.namespace == "" || name == "" {
		return name
	}
	return e.namespace + "_" + name
}

func (e *Exporter) normalizeLabelNames(names []string) ([]string, error) {
	seen := make(map[string]struct{}, len(names))
	result := make([]string, 0, len(names))
	for _, raw := range names {
		name := sanitizeLabelName(raw)
		if name == "" {
			return nil, fmt.Errorf("%w: empty label name", ErrInvalidMetric)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("%w: duplicate label %q", ErrInvalidMetric, name)
		}
		seen[name] = struct{}{}
		if len(e.allowedLabels) == 0 {
			continue
		}
		if _, ok := e.allowedLabels[name]; !ok {
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func (e *Exporter) allowedNames(labels map[string]string) []string {
	set := make(map[string]struct{})
	for raw := range labels {
		name := sanitizeLabelName(raw)
		if _, ok := e.allowedLabels[name]; ok {
			set[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func (e *Exporter) canonicalLabels(names []string, labels map[string]string) ([]labelPair, string) {
	result := make([]labelPair, 0, len(names))
	var key strings.Builder
	for _, name := range names {
		value := labels[name]
		result = append(result, labelPair{name, value})
		key.WriteString(name)
		key.WriteByte('=')
		key.WriteString(value)
		key.WriteByte(0)
	}
	return result, key.String()
}

func (e *Exporter) admitLabelValues(metric string, labels []labelPair) error {
	byName := e.labelValues[metric]
	if byName == nil {
		byName = make(map[string]map[string]struct{})
		e.labelValues[metric] = byName
	}
	for _, label := range labels {
		values := byName[label.name]
		if values == nil {
			values = make(map[string]struct{})
			byName[label.name] = values
		}
		if _, ok := values[label.value]; !ok && len(values) >= e.maxLabelValues {
			return fmt.Errorf("%w: metric %q label %q allows at most %d values", ErrCardinalityLimit, metric, label.name, e.maxLabelValues)
		}
	}
	for _, label := range labels {
		byName[label.name][label.value] = struct{}{}
	}
	return nil
}

func writeSample(b *strings.Builder, name string, labels []labelPair, value float64) {
	b.WriteString(name)
	if len(labels) > 0 {
		b.WriteByte('{')
		for i, label := range labels {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, `%s="%s"`, label.name, escapeLabel(label.value))
		}
		b.WriteByte('}')
	}
	fmt.Fprintf(b, " %s\n", formatFloat(value))
}

func appendLabel(labels []labelPair, pair labelPair) []labelPair {
	result := append([]labelPair(nil), labels...)
	result = append(result, pair)
	return result
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == ':' || (i > 0 && r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	result := b.String()
	if result == "" {
		return ""
	}
	first := result[0]
	if first >= '0' && first <= '9' {
		result = "_" + result
	}
	return result
}

func sanitizeLabelName(value string) string {
	value = sanitizeName(value)
	return strings.ReplaceAll(value, ":", "_")
}

func sanitizeHelp(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(strings.ReplaceAll(value, "\n", `\n`), "\r", `\r`)
}
func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(strings.ReplaceAll(value, "\n", `\n`), "\r", `\r`)
}
func formatFloat(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }
func finite(value float64) bool        { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func validMetricType(value MetricType) bool {
	return value == Counter || value == Gauge || value == Histogram
}

func normalizeLabelValues(labels Labels) (map[string]string, error) {
	result := make(map[string]string, len(labels))
	for raw, value := range labels {
		name := sanitizeLabelName(raw)
		if name == "" {
			return nil, fmt.Errorf("%w: empty label name", ErrInvalidMetric)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("%w: labels %q and a normalized duplicate collide", ErrInvalidMetric, name)
		}
		result[name] = value
	}
	return result, nil
}
func sameStrings(a, b []string) bool {
	return len(a) == len(b) && strings.Join(a, "\x00") == strings.Join(b, "\x00")
}
func normalizeBuckets(values []float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		if finite(value) && value >= 0 {
			result = append(result, value)
		}
	}
	sort.Float64s(result)
	unique := result[:0]
	for _, value := range result {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	return unique
}
