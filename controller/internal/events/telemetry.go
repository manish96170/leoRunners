package events

import (
	"fmt"
	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
)

// TelemetryPublisher adapts the existing lifecycle telemetry sink to the
// versioned event bus. It publishes operational identity only; arbitrary
// telemetry metadata is redacted before it crosses the event boundary.
type TelemetryPublisher struct{ Bus *Bus }

func (p *TelemetryPublisher) Emit(event telemetry.Event) error {
	if p == nil || p.Bus == nil {
		return fmt.Errorf("event bus is not configured")
	}
	return p.Bus.Publish(Event{Kind: KindLifecycle, Type: event.Type, OccurredAt: event.At, Correlation: CorrelationMetadata{JobID: event.JobID, RunnerID: event.RunnerID}, Attributes: RedactAttributes(event.Metadata)})
}

var _ telemetry.Sink = (*TelemetryPublisher)(nil)
