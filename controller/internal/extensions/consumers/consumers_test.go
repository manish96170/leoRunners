package consumers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/events"
)

func consumerEvent(attrs map[string]string) events.Event {
	e, err := events.New(events.Event{Kind: events.KindJob, Type: "job.completed", OccurredAt: time.Unix(10, 0), Job: events.JobMetadata{ID: "job-1"}, Provider: events.ProviderMetadata{Name: "aws"}, Attributes: attrs})
	if err != nil {
		panic(err)
	}
	return e
}

func TestConsumersImplementExtensionAndHandleMissingFields(t *testing.T) {
	sink := NewMemorySink(16)
	event := consumerEvent(nil)
	analytics := NewAnalytics(sink)
	cache := NewCache(sink)
	cost := NewCost(sink, map[string]float64{"aws": 3.6})
	security := NewSecurity(sink)
	for _, consumer := range []interface {
		Handle(context.Context, events.Event) error
	}{analytics, cache, cost, security} {
		if err := consumer.Handle(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.Summaries()) != 3 {
		t.Fatalf("expected three summaries, got %d", len(sink.Summaries()))
	}
	if len(sink.Outputs()) != 0 {
		t.Fatal("missing trust data must not emit security output")
	}
}

func TestAnalyticsCacheAndCostAreBoundedAndReadOnly(t *testing.T) {
	sink := NewMemorySink(2)
	analytics := NewAnalytics(sink)
	cache := NewCache(sink)
	cost := NewCost(sink, map[string]float64{"aws": 3.6})
	e := consumerEvent(map[string]string{"tenant_id": "tenant-a", "cache_outcome": "hit", "duration_seconds": "3600"})
	original := e.Attributes["cache_outcome"]
	if err := analytics.Handle(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := cache.Handle(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := cost.Handle(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if e.Attributes["cache_outcome"] != original {
		t.Fatal("consumer mutated event")
	}
	if got := analytics.Snapshot()["job.completed"]; got != 1 {
		t.Fatalf("analytics count=%d", got)
	}
	hits, misses, unknown := cache.Snapshot()
	if hits != 1 || misses != 0 || unknown != 0 {
		t.Fatalf("cache snapshot=%d,%d,%d", hits, misses, unknown)
	}
	if cost.Total() != 3.6 {
		t.Fatalf("cost total=%f", cost.Total())
	}
	if sink.Dropped() != 1 {
		t.Fatalf("expected bounded sink drop, got %d", sink.Dropped())
	}
}

func TestSecurityOnlyEmitsAdvisoryForKnownUntrustedClass(t *testing.T) {
	sink := NewMemorySink(4)
	consumer := NewSecurity(sink)
	if err := consumer.Handle(context.Background(), consumerEvent(map[string]string{"trust_class": "fork"})); err != nil {
		t.Fatal(err)
	}
	outputs := sink.Outputs()
	if len(outputs) != 1 || len(outputs[0].Actions) != 1 {
		t.Fatalf("unexpected output: %+v", outputs)
	}
	if outputs[0].Actions[0].Action != "annotate" {
		t.Fatalf("unexpected action: %q", outputs[0].Actions[0].Action)
	}
	if err := consumer.Handle(context.Background(), consumerEvent(map[string]string{"trust_class": "terminate"})); err != nil {
		t.Fatal(err)
	}
	if len(sink.Outputs()) != 1 {
		t.Fatal("unknown action-like trust value emitted output")
	}
}

type failingSummarySink struct{}

func (failingSummarySink) EmitSummary(context.Context, Summary) error {
	return errors.New("sink failed")
}
func TestSinkErrorsPropagate(t *testing.T) {
	if err := NewAnalytics(failingSummarySink{}).Handle(context.Background(), consumerEvent(nil)); err == nil {
		t.Fatal("expected sink error")
	}
}
