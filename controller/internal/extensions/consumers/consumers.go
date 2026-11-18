// Package consumers contains production-shaped, read-only extension consumers.
// They intentionally depend only on the controller's event and policy
// contracts and never receive provider, lifecycle, or credential interfaces.
package consumers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/events"
	"github.com/leo-runners/ci-platform/controller/internal/extensions"
	"github.com/leo-runners/ci-platform/controller/internal/extensions/policy"
)

const Version = extensions.Version

type Summary struct {
	Version    string            `json:"version"`
	ConsumerID string            `json:"consumer_id"`
	EventID    string            `json:"event_id"`
	OccurredAt time.Time         `json:"occurred_at"`
	Kind       string            `json:"kind"`
	Type       string            `json:"type"`
	TenantID   string            `json:"tenant_id,omitempty"`
	JobID      string            `json:"job_id,omitempty"`
	Provider   string            `json:"provider,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
}

type SummarySink interface {
	EmitSummary(context.Context, Summary) error
}

type OutputSink interface {
	EmitOutput(context.Context, policy.Output) error
}

var (
	_ extensions.Extension = (*AnalyticsConsumer)(nil)
	_ extensions.Extension = (*CacheConsumer)(nil)
	_ extensions.Extension = (*CostConsumer)(nil)
	_ extensions.Extension = (*SecurityConsumer)(nil)
)

// MemorySink is a bounded, non-blocking test and local integration sink.
// Dropped reports are observable and never cause event processing to block.
type MemorySink struct {
	mu        sync.Mutex
	max       int
	summaries []Summary
	outputs   []policy.Output
	dropped   uint64
}

func NewMemorySink(max int) *MemorySink {
	if max <= 0 {
		max = 64
	}
	return &MemorySink{max: max}
}

func (s *MemorySink) EmitSummary(_ context.Context, summary Summary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.summaries) >= s.max {
		s.dropped++
		return nil
	}
	s.summaries = append(s.summaries, cloneSummary(summary))
	return nil
}

func (s *MemorySink) EmitOutput(_ context.Context, output policy.Output) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.outputs) >= s.max {
		s.dropped++
		return nil
	}
	s.outputs = append(s.outputs, cloneOutput(output))
	return nil
}

func (s *MemorySink) Summaries() []Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Summary, len(s.summaries))
	for i := range s.summaries {
		out[i] = cloneSummary(s.summaries[i])
	}
	return out
}

func (s *MemorySink) Outputs() []policy.Output {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]policy.Output, len(s.outputs))
	for i := range s.outputs {
		out[i] = cloneOutput(s.outputs[i])
	}
	return out
}

func (s *MemorySink) Dropped() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.dropped }

type base struct {
	id, name string
	sink     SummarySink
	output   OutputSink
	mu       sync.Mutex
	count    uint64
	unknown  uint64
}

func (b *base) metadata(capability string) extensions.Metadata {
	return extensions.Metadata{ID: b.id, Name: b.name, Version: Version, Enabled: true, Capabilities: []string{capability}}
}

func (b *base) observe(event events.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.count++
	if strings.TrimSpace(event.Type) == "" {
		b.unknown++
	}
}

func (b *base) stats() (uint64, uint64) { b.mu.Lock(); defer b.mu.Unlock(); return b.count, b.unknown }

func summaryFor(id string, event events.Event, fields map[string]string) Summary {
	return Summary{Version: "consumers.v1", ConsumerID: id, EventID: event.ID, OccurredAt: event.OccurredAt.UTC(), Kind: string(event.Kind), Type: bounded(event.Type, 128), TenantID: firstNonEmpty(event.Attributes["tenant_id"], event.Attributes["tenant"], "unknown"), JobID: firstNonEmpty(event.Job.ID, event.Correlation.JobID, "unknown"), Provider: firstNonEmpty(event.Provider.Name, "unknown"), Fields: boundedFields(fields)}
}

func emitSummary(ctx context.Context, sink SummarySink, summary Summary) error {
	if sink == nil {
		return nil
	}
	return sink.EmitSummary(ctx, summary)
}

// AnalyticsConsumer counts event kinds and types and emits one bounded record
// per event. It does not retain event payloads or high-cardinality identifiers.
type AnalyticsConsumer struct {
	base
	mu     sync.Mutex
	byType map[string]uint64
}

func NewAnalytics(sink SummarySink) *AnalyticsConsumer {
	return &AnalyticsConsumer{base: base{id: "analytics", name: "Read-only analytics", sink: sink}, byType: make(map[string]uint64)}
}
func (c *AnalyticsConsumer) Metadata() extensions.Metadata { return c.metadata("event-analytics") }
func (c *AnalyticsConsumer) Handle(ctx context.Context, event events.Event) error {
	c.observe(event)
	typ := bounded(event.Type, 128)
	if typ == "" {
		typ = "unknown"
	}
	c.mu.Lock()
	c.byType[typ]++
	c.mu.Unlock()
	return emitSummary(ctx, c.sink, summaryFor(c.id, event, map[string]string{"event_count": fmt.Sprint(c.countFor(typ)), "type": typ}))
}
func (c *AnalyticsConsumer) countFor(typ string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byType[typ]
}
func (c *AnalyticsConsumer) Snapshot() map[string]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]uint64, len(c.byType))
	for k, v := range c.byType {
		out[k] = v
	}
	return out
}

// CacheConsumer observes cache outcomes only. It never invalidates, warms, or
// writes to a cache; unknown outcomes are counted as unknown.
type CacheConsumer struct {
	base
	mu                    sync.Mutex
	hits, misses, unknown uint64
}

func NewCache(sink SummarySink) *CacheConsumer {
	return &CacheConsumer{base: base{id: "cache", name: "Read-only cache observer", sink: sink}}
}
func (c *CacheConsumer) Metadata() extensions.Metadata { return c.metadata("cache-observation") }
func (c *CacheConsumer) Handle(ctx context.Context, event events.Event) error {
	c.observe(event)
	outcome := strings.ToLower(firstNonEmpty(event.Attributes["cache_outcome"], event.Attributes["cache_result"], "unknown"))
	c.mu.Lock()
	switch outcome {
	case "hit":
		c.hits++
	case "miss":
		c.misses++
	default:
		outcome = "unknown"
		c.unknown++
	}
	c.mu.Unlock()
	return emitSummary(ctx, c.sink, summaryFor(c.id, event, map[string]string{"cache_outcome": outcome}))
}
func (c *CacheConsumer) Snapshot() (hits, misses, unknown uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, c.unknown
}

// CostConsumer estimates cost from supplied duration and provider attributes.
// Rates are caller-supplied, bounded display data; absent or malformed values
// produce a zero estimate and are never treated as permission to provision.
type CostConsumer struct {
	base
	mu    sync.Mutex
	rates map[string]float64
	total float64
}

func NewCost(sink SummarySink, rates map[string]float64) *CostConsumer {
	clean := make(map[string]float64)
	for k, v := range rates {
		if len(clean) < 32 && v >= 0 {
			clean[bounded(k, 64)] = v
		}
	}
	return &CostConsumer{base: base{id: "cost", name: "Read-only cost observer", sink: sink}, rates: clean}
}
func (c *CostConsumer) Metadata() extensions.Metadata { return c.metadata("cost-estimation") }
func (c *CostConsumer) Handle(ctx context.Context, event events.Event) error {
	c.observe(event)
	provider := firstNonEmpty(event.Provider.Name, event.Attributes["provider"], "unknown")
	rate := c.rates[provider]
	seconds := parseNonNegative(event.Attributes["duration_seconds"])
	estimate := rate * seconds / 3600
	c.mu.Lock()
	c.total += estimate
	c.mu.Unlock()
	return emitSummary(ctx, c.sink, summaryFor(c.id, event, map[string]string{"provider": provider, "estimated_cost": fmt.Sprintf("%.8f", estimate), "currency": "USD"}))
}
func (c *CostConsumer) Total() float64 { c.mu.Lock(); defer c.mu.Unlock(); return c.total }

// SecurityConsumer emits only advisory annotations for explicitly known
// untrusted/fork signals. It never blocks, revokes, escalates, or changes a
// runner. Missing and unknown trust data yields no output.
type SecurityConsumer struct {
	base
	maxAnnotations int
}

func NewSecurity(sink OutputSink) *SecurityConsumer {
	return &SecurityConsumer{base: base{id: "security", name: "Read-only security observer", output: sink}, maxAnnotations: 4}
}
func (c *SecurityConsumer) Metadata() extensions.Metadata { return c.metadata("security-observation") }
func (c *SecurityConsumer) Handle(ctx context.Context, event events.Event) error {
	c.observe(event)
	trust := strings.ToLower(firstNonEmpty(event.Attributes["trust_class"], event.Attributes["trust"], ""))
	if trust != "fork" && trust != "untrusted" {
		return nil
	}
	job := firstNonEmpty(event.Job.ID, event.Correlation.JobID, "unknown")
	output := policy.Output{Version: policy.CurrentVersion, ExtensionID: c.id, JobID: job, Actions: []policy.ExtensionAction{{Action: policy.ActionAnnotate, Title: "Review runner trust", Summary: "This event is associated with an untrusted execution context.", Rationale: "Use the platform's configured trust policy before relying on artifacts or logs."}}, Annotations: []policy.Annotation{{Path: ".github/workflows", Level: "warning", Title: "Untrusted execution context", Message: "Review this run's permissions and artifacts before use."}}, Metadata: map[string]string{"trust_class": trust}}
	if c.maxAnnotations < len(output.Annotations) {
		output.Annotations = output.Annotations[:c.maxAnnotations]
	}
	if c.output == nil {
		return nil
	}
	return c.output.EmitOutput(ctx, output)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return bounded(strings.TrimSpace(v), 256)
		}
	}
	return ""
}
func bounded(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}
func boundedFields(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string)
	for _, k := range keys {
		if len(out) >= 16 {
			break
		}
		out[bounded(k, 64)] = bounded(in[k], 256)
	}
	return out
}
func parseNonNegative(value string) float64 {
	var n float64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%f", &n); err != nil || n < 0 || n > 31536000 {
		return 0
	}
	return n
}
func cloneSummary(in Summary) Summary { in.Fields = boundedFields(in.Fields); return in }
func cloneOutput(in policy.Output) policy.Output {
	out := in
	out.Actions = append([]policy.ExtensionAction(nil), in.Actions...)
	out.Annotations = append([]policy.Annotation(nil), in.Annotations...)
	if in.Metadata != nil {
		out.Metadata = map[string]string{}
		for k, v := range in.Metadata {
			out.Metadata[bounded(k, 64)] = bounded(v, 256)
		}
	}
	return out
}
