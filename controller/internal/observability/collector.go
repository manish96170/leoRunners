package observability

import (
	"sync"
	"time"
)

// Collector is a bounded, concurrency-safe in-memory metric sink. It stores
// samples rather than aggregating them so backend adapters can choose the
// appropriate aggregation semantics without losing observations.
type Collector struct {
	mu      sync.RWMutex
	max     int
	samples []MetricSample
	dropped uint64
}

var _ MetricSink = (*Collector)(nil)

func NewCollector(maxSamples int) *Collector {
	if maxSamples <= 0 {
		maxSamples = 10000
	}
	return &Collector{max: maxSamples}
}

func (c *Collector) Record(sample MetricSample) error {
	if err := sample.Validate(); err != nil {
		return err
	}
	sample.Dimensions = sample.Dimensions.Clone()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.samples) == c.max {
		copy(c.samples, c.samples[1:])
		c.samples[len(c.samples)-1] = sample
		c.dropped++
		return nil
	}
	c.samples = append(c.samples, sample)
	return nil
}

func (c *Collector) AddCounter(name string, delta float64, dimensions Dimensions) error {
	return c.Record(MetricSample{Name: name, Kind: MetricCounter, Unit: UnitCount, Value: delta, Timestamp: time.Now().UTC(), Dimensions: dimensions})
}

func (c *Collector) SetGauge(name string, value float64, unit MetricUnit, dimensions Dimensions) error {
	return c.Record(MetricSample{Name: name, Kind: MetricGauge, Unit: unit, Value: value, Timestamp: time.Now().UTC(), Dimensions: dimensions})
}

func (c *Collector) Observe(name string, value float64, unit MetricUnit, dimensions Dimensions) error {
	return c.Record(MetricSample{Name: name, Kind: MetricHistogram, Unit: unit, Value: value, Timestamp: time.Now().UTC(), Dimensions: dimensions})
}

func (c *Collector) Snapshot() []MetricSample {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]MetricSample, len(c.samples))
	for i, sample := range c.samples {
		out[i] = sample
		out[i].Dimensions = sample.Dimensions.Clone()
	}
	return out
}

func (c *Collector) Dropped() uint64 { c.mu.RLock(); defer c.mu.RUnlock(); return c.dropped }
func (c *Collector) Len() int        { c.mu.RLock(); defer c.mu.RUnlock(); return len(c.samples) }
