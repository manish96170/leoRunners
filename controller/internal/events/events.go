// Package events provides the controller's versioned in-process event contract.
package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const Version = "events.v1"

type Kind string

const (
	KindLifecycle Kind = "lifecycle"
	KindJob       Kind = "job"
	KindRunner    Kind = "runner"
	KindProvider  Kind = "provider"
)

var (
	ErrInvalidEvent       = errors.New("invalid event")
	ErrBusClosed          = errors.New("event bus is closed")
	ErrSubscriptionClosed = errors.New("event subscription is closed")
)

type LifecycleMetadata struct {
	FromState string `json:"from_state,omitempty"`
	ToState   string `json:"to_state,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
}

type JobMetadata struct {
	ID         string `json:"id,omitempty"`
	Repository string `json:"repository,omitempty"`
	Workflow   string `json:"workflow,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	Attempt    string `json:"attempt,omitempty"`
}

type RunnerMetadata struct {
	ID                 string `json:"id,omitempty"`
	ProviderInstanceID string `json:"provider_instance_id,omitempty"`
	State              string `json:"state,omitempty"`
}

type ProviderMetadata struct {
	Name         string `json:"name,omitempty"`
	Region       string `json:"region,omitempty"`
	CapacityPool string `json:"capacity_pool,omitempty"`
}

type CorrelationMetadata struct {
	RequestID  string `json:"request_id,omitempty"`
	DeliveryID string `json:"delivery_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	RunnerID   string `json:"runner_id,omitempty"`
	LeaseID    string `json:"lease_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
}

// Event is immutable after construction. Attributes are normalized and cloned
// by New; Publish also normalizes defensively for callers using a literal.
type Event struct {
	ID          string              `json:"id"`
	Version     string              `json:"version"`
	Kind        Kind                `json:"kind"`
	Type        string              `json:"type"`
	OccurredAt  time.Time           `json:"occurred_at"`
	Lifecycle   LifecycleMetadata   `json:"lifecycle,omitempty"`
	Job         JobMetadata         `json:"job,omitempty"`
	Runner      RunnerMetadata      `json:"runner,omitempty"`
	Provider    ProviderMetadata    `json:"provider,omitempty"`
	Correlation CorrelationMetadata `json:"correlation,omitempty"`
	Attributes  map[string]string   `json:"attributes,omitempty"`
}

const (
	maxType           = 160
	maxField          = 512
	maxAttributes     = 64
	maxAttributeValue = 4096
)

var secretKey = regexp.MustCompile(`(?i)(token|secret|password|passwd|credential|authorization|private[_-]?key|access[_-]?key|jit[_-]?config)`)

func New(event Event) (Event, error) {
	if event.Version == "" {
		event.Version = Version
	}
	event.Attributes = RedactAttributes(event.Attributes)
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	event.ID = DeterministicID(event)
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

// DeterministicID hashes the event without its ID. encoding/json sorts map
// keys, making equivalent attribute maps produce the same identifier.
func DeterministicID(event Event) string {
	event.ID = ""
	payload, _ := json.Marshal(event)
	sum := sha256.Sum256(payload)
	return "evt_" + hex.EncodeToString(sum[:])
}

func (e Event) Validate() error {
	if e.Version != Version {
		return fmt.Errorf("%w: unsupported version %q", ErrInvalidEvent, e.Version)
	}
	if e.ID == "" || e.ID != DeterministicID(e) {
		return fmt.Errorf("%w: id is missing or does not match event content", ErrInvalidEvent)
	}
	if e.Kind != KindLifecycle && e.Kind != KindJob && e.Kind != KindRunner && e.Kind != KindProvider {
		return fmt.Errorf("%w: unsupported kind %q", ErrInvalidEvent, e.Kind)
	}
	if strings.TrimSpace(e.Type) == "" || len(e.Type) > maxType {
		return fmt.Errorf("%w: type is required and bounded", ErrInvalidEvent)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("%w: occurred_at is required", ErrInvalidEvent)
	}
	for _, value := range []string{e.Job.ID, e.Runner.ID, e.Provider.Name, e.Correlation.JobID, e.Correlation.RunnerID, e.Correlation.LeaseID} {
		if len(value) > maxField {
			return fmt.Errorf("%w: metadata field is too large", ErrInvalidEvent)
		}
	}
	if len(e.Attributes) > maxAttributes {
		return fmt.Errorf("%w: too many attributes", ErrInvalidEvent)
	}
	for key, value := range e.Attributes {
		if strings.TrimSpace(key) == "" || len(key) > maxField || len(value) > maxAttributeValue {
			return fmt.Errorf("%w: invalid attribute", ErrInvalidEvent)
		}
		if (secretKey.MatchString(key) && value != "[REDACTED]") || looksLikeSecret(value) {
			return fmt.Errorf("%w: unredacted sensitive attribute %q", ErrInvalidEvent, key)
		}
	}
	return nil
}

func (e Event) clone() Event {
	e.Attributes = cloneAttributes(e.Attributes)
	return e
}

// RedactAttributes replaces values whose keys or shapes indicate credentials.
// It returns a new map and never mutates caller-owned data.
func RedactAttributes(attributes map[string]string) map[string]string {
	if attributes == nil {
		return nil
	}
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(attributes))
	for _, key := range keys {
		value := attributes[key]
		if secretKey.MatchString(key) || looksLikeSecret(value) {
			out[key] = "[REDACTED]"
		} else {
			out[key] = value
		}
	}
	return out
}

func cloneAttributes(attributes map[string]string) map[string]string {
	if attributes == nil {
		return nil
	}
	out := make(map[string]string, len(attributes))
	for key, value := range attributes {
		out[key] = value
	}
	return out
}

func looksLikeSecret(value string) bool {
	return strings.HasPrefix(value, "ghp_") || strings.HasPrefix(value, "ghs_") ||
		strings.HasPrefix(value, "github_pat_") || strings.HasPrefix(value, "AKIA") ||
		strings.HasPrefix(value, "Bearer ") || strings.Contains(value, "-----BEGIN ")
}
