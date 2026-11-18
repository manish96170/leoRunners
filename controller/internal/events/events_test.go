package events

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func testEvent(n int) Event {
	e, err := New(Event{Kind: KindJob, Type: "job.updated", OccurredAt: time.Unix(int64(n), 0), Job: JobMetadata{ID: fmt.Sprintf("job-%d", n)}, Attributes: map[string]string{"safe": "value"}})
	if err != nil {
		panic(err)
	}
	return e
}

func TestNewDeterministicAndRedactsWithoutMutatingInput(t *testing.T) {
	attrs := map[string]string{"token": "ghp_secret", "safe": "value"}
	e, err := New(Event{Kind: KindRunner, Type: "runner.ready", OccurredAt: time.Unix(1, 0), Attributes: attrs})
	if err != nil {
		t.Fatal(err)
	}
	if e.Attributes["token"] != "[REDACTED]" || attrs["token"] != "ghp_secret" {
		t.Fatalf("redaction boundary failed: %#v %#v", e.Attributes, attrs)
	}
	if e.ID != DeterministicID(e) {
		t.Fatal("event ID is not deterministic")
	}
	copyEvent, err := New(e)
	if err != nil || copyEvent.ID != e.ID {
		t.Fatalf("equivalent event changed ID: %v", err)
	}
}

func TestValidationRejectsUnsupportedAndUnredactedEvents(t *testing.T) {
	e := testEvent(1)
	e.Version = "events.v0"
	if !errors.Is(e.Validate(), ErrInvalidEvent) {
		t.Fatal("expected version validation error")
	}
	if _, err := New(Event{Version: "events.v0", Kind: KindJob, Type: "job.updated", OccurredAt: time.Unix(3, 0)}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatal("New must reject an explicitly unsupported version")
	}
	e = testEvent(2)
	e.Attributes = map[string]string{"secret": "raw"}
	e.ID = DeterministicID(e)
	if !errors.Is(e.Validate(), ErrInvalidEvent) {
		t.Fatal("expected secret validation error")
	}
}

func TestBusOrderDedupAndDefensiveCopies(t *testing.T) {
	b := NewBus(BusConfig{Buffer: 8})
	s, err := b.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for i := 0; i < 3; i++ {
		if err := b.Publish(testEvent(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Publish(testEvent(1)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		select {
		case got := <-s.C():
			if got.Job.ID != fmt.Sprintf("job-%d", i) {
				t.Fatalf("event order changed: %s", got.Job.ID)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
	if m := b.Metrics(); m.Published != 3 || m.Duplicates != 1 {
		t.Fatalf("unexpected metrics: %+v", m)
	}
}

func TestBusDropsForSlowSubscriber(t *testing.T) {
	b := NewBus(BusConfig{Buffer: 1})
	s, _ := b.Subscribe()
	defer b.Close()
	for i := 0; i < 10; i++ {
		if err := b.Publish(testEvent(i)); err != nil {
			t.Fatal(err)
		}
	}
	if s.Dropped() != 9 || b.Metrics().Dropped != 9 {
		t.Fatalf("unexpected drop metrics: sub=%d bus=%+v", s.Dropped(), b.Metrics())
	}
}

func TestBusConcurrentPublishAndClose(t *testing.T) {
	b := NewBus(BusConfig{Buffer: 256})
	s, _ := b.Subscribe()
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				_ = b.Publish(testEvent(worker*25 + i))
			}
		}(worker)
	}
	wg.Wait()
	if b.Metrics().Published != 200 {
		t.Fatalf("unexpected concurrent publish count: %+v", b.Metrics())
	}
	b.Close()
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-s.C():
			if !ok {
				goto closed
			}
		case <-deadline:
			t.Fatal("subscription close timed out")
		}
	}

closed:
	if err := b.Publish(testEvent(999)); !errors.Is(err, ErrBusClosed) {
		t.Fatalf("expected closed error, got %v", err)
	}
}

func TestSubscriptionCloseIsIdempotent(t *testing.T) {
	b := NewBus(BusConfig{Buffer: 1})
	s, _ := b.Subscribe()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if b.Metrics().Subscribers != 0 {
		t.Fatal("subscriber remained registered")
	}
	b.Close()
}
