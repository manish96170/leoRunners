package telemetry

import (
	"sync"
	"time"
)

type Event struct {
	ID, Type, JobID, RunnerID, Provider string
	At                                  time.Time
	Metadata                            map[string]string
}
type Sink interface{ Emit(Event) error }
type MemorySink struct {
	mu     sync.Mutex
	Events []Event
}

func (s *MemorySink) Emit(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, e)
	return nil
}
func (s *MemorySink) Snapshot() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.Events...)
}
