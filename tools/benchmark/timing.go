package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// Phase names are kept stable because they are the CSV columns and JSON keys.
type Phase string

const (
	Queue        Phase = "queue"
	Launch       Phase = "launch"
	Boot         Phase = "boot"
	Registration Phase = "registration"
	Ready        Phase = "ready"
	JobStart     Phase = "job_start"
	Cleanup      Phase = "cleanup"
)

var phases = []Phase{Queue, Launch, Boot, Registration, Ready, JobStart, Cleanup}

type Sample struct {
	ID          string                  `json:"id"`
	StartedAt   time.Time               `json:"started_at"`
	CompletedAt time.Time               `json:"completed_at"`
	Timestamps  map[Phase]time.Time     `json:"timestamps"`
	Durations   map[Phase]time.Duration `json:"durations"`
}

func (s Sample) Duration(phase Phase) time.Duration { return s.Durations[phase] }

func (s Sample) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("sample id is required")
	}
	previous := s.StartedAt
	for _, phase := range phases {
		at, ok := s.Timestamps[phase]
		if !ok || at.IsZero() {
			return fmt.Errorf("missing timestamp for phase %q", phase)
		}
		if at.Before(previous) {
			return fmt.Errorf("phase %q occurs before previous phase", phase)
		}
		previous = at
	}
	if s.CompletedAt.IsZero() || s.CompletedAt.Before(previous) {
		return errors.New("completed_at must be after all phase timestamps")
	}
	return nil
}

type Collector struct {
	mu      sync.Mutex
	started time.Time
	marks   map[Phase]time.Time
}

func NewCollector(start time.Time) *Collector {
	return &Collector{started: start.UTC(), marks: make(map[Phase]time.Time)}
}

func (c *Collector) Mark(phase Phase, at time.Time) error {
	if !validPhase(phase) {
		return fmt.Errorf("unknown phase %q", phase)
	}
	if at.IsZero() {
		return errors.New("phase timestamp is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if at.Before(c.started) {
		return errors.New("phase timestamp precedes start")
	}
	if prior, ok := c.marks[phase]; ok && !prior.Equal(at) {
		return fmt.Errorf("phase %q already marked", phase)
	}
	c.marks[phase] = at.UTC()
	return nil
}

func (c *Collector) Finish(id string, at time.Time) (Sample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	timestamps := make(map[Phase]time.Time, len(c.marks))
	for phase, timestamp := range c.marks {
		timestamps[phase] = timestamp
	}
	sample := Sample{ID: id, StartedAt: c.started, CompletedAt: at.UTC(), Timestamps: timestamps, Durations: make(map[Phase]time.Duration, len(phases))}
	previous := c.started
	for _, phase := range phases {
		timestamp, ok := timestamps[phase]
		if !ok {
			return Sample{}, fmt.Errorf("missing timestamp for phase %q", phase)
		}
		sample.Durations[phase] = timestamp.Sub(previous)
		previous = timestamp
	}
	if err := sample.Validate(); err != nil {
		return Sample{}, err
	}
	return sample, nil
}

func validPhase(phase Phase) bool {
	for _, candidate := range phases {
		if phase == candidate {
			return true
		}
	}
	return false
}

type FakeTimeline struct {
	ID        string
	Start     time.Time
	Durations map[Phase]time.Duration
}

func (f FakeTimeline) Replay() (Sample, error) {
	start := f.Start.UTC()
	if start.IsZero() {
		start = time.Unix(0, 0).UTC()
	}
	collector := NewCollector(start)
	at := start
	for _, phase := range phases {
		duration, ok := f.Durations[phase]
		if !ok || duration < 0 {
			return Sample{}, fmt.Errorf("invalid duration for phase %q", phase)
		}
		at = at.Add(duration)
		if err := collector.Mark(phase, at); err != nil {
			return Sample{}, err
		}
	}
	return collector.Finish(f.ID, at)
}

func WriteJSON(w io.Writer, samples []Sample) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(samples)
}

func SortedSamples(samples []Sample) []Sample {
	result := append([]Sample(nil), samples...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
