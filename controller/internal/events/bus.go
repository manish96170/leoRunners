package events

import (
	"sync"
)

type BusConfig struct{ Buffer int }

type Metrics struct {
	Published   uint64
	Duplicates  uint64
	Dropped     uint64
	Subscribers int
	Closed      bool
}

type Bus struct {
	mu          sync.Mutex
	buffer      int
	subscribers map[uint64]*Subscription
	dedup       map[string]struct{}
	nextID      uint64
	metrics     Metrics
}

type Subscription struct {
	bus     *Bus
	id      uint64
	ch      chan Event
	once    sync.Once
	mu      sync.RWMutex
	dropped uint64
	closed  bool
}

func NewBus(config BusConfig) *Bus {
	if config.Buffer <= 0 {
		config.Buffer = 64
	}
	return &Bus{buffer: config.Buffer, subscribers: make(map[uint64]*Subscription), dedup: make(map[string]struct{})}
}

func (b *Bus) Subscribe() (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.metrics.Closed {
		return nil, ErrBusClosed
	}
	b.nextID++
	s := &Subscription{bus: b, id: b.nextID, ch: make(chan Event, b.buffer)}
	b.subscribers[s.id] = s
	b.metrics.Subscribers = len(b.subscribers)
	return s, nil
}

func (b *Bus) Publish(event Event) error {
	prepared, err := New(event)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.metrics.Closed {
		return ErrBusClosed
	}
	if _, exists := b.dedup[prepared.ID]; exists {
		b.metrics.Duplicates++
		return nil
	}
	b.dedup[prepared.ID] = struct{}{}
	b.metrics.Published++
	for _, sub := range b.subscribers {
		select {
		case sub.ch <- prepared.clone():
		default:
			sub.dropped++
			b.metrics.Dropped++
		}
	}
	return nil
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.metrics.Closed {
		return
	}
	b.metrics.Closed = true
	for id, sub := range b.subscribers {
		sub.mu.Lock()
		sub.closed = true
		sub.mu.Unlock()
		close(sub.ch)
		delete(b.subscribers, id)
	}
	b.metrics.Subscribers = 0
}

func (b *Bus) Metrics() Metrics {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.metrics
}

func (s *Subscription) C() <-chan Event { return s.ch }

func (s *Subscription) Close() error {
	if s == nil || s.bus == nil {
		return nil
	}
	s.once.Do(func() { s.bus.remove(s) })
	return nil
}

func (s *Subscription) Dropped() uint64 {
	if s == nil {
		return 0
	}
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	return s.dropped
}

func (b *Bus) remove(s *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.subscribers[s.id]; !exists {
		return
	}
	delete(b.subscribers, s.id)
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	close(s.ch)
	b.metrics.Subscribers = len(b.subscribers)
}
