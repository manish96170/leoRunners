// Package github contains the GitHub-facing contracts used by the controller.
package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const WorkflowJobEventName = "workflow_job"

// Action is the subset of workflow_job actions that can affect runner
// lifecycle. GitHub normally reports cancellation as a completed action with
// conclusion "cancelled", but the explicit cancelled action is accepted too.
type Action string

const (
	ActionQueued     Action = "queued"
	ActionInProgress Action = "in_progress"
	ActionCompleted  Action = "completed"
	ActionCancelled  Action = "cancelled"
)

func (a Action) valid() bool {
	return a == ActionQueued || a == ActionInProgress || a == ActionCompleted || a == ActionCancelled
}

// WorkflowJobEvent is the stable, provider-independent representation of a
// GitHub workflow_job webhook delivery.
type WorkflowJobEvent struct {
	EventName  string      `json:"event_name"`
	EventID    string      `json:"event_id"`
	DeliveryID string      `json:"delivery_id,omitempty"`
	Action     Action      `json:"action"`
	ReceivedAt time.Time   `json:"received_at"`
	DedupKey   string      `json:"dedup_key"`
	JobKey     string      `json:"job_key"`
	RunKey     string      `json:"run_key"`
	Job        WorkflowJob `json:"job"`
}

// WorkflowJob contains only fields needed by scheduling, lifecycle, and
// reconciliation. The webhook decoder deliberately ignores future GitHub
// fields so payload evolution does not break intake.
type WorkflowJob struct {
	ID           int64      `json:"id"`
	RunID        int64      `json:"run_id"`
	RunAttempt   int        `json:"run_attempt"`
	Name         string     `json:"name"`
	WorkflowName string     `json:"workflow_name"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	Labels       []string   `json:"labels"`
	RunnerName   string     `json:"runner_name"`
	RunnerID     int64      `json:"runner_id"`
	Repository   Repository `json:"repository"`
}

type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Owner    any    `json:"owner"`
	Name     string `json:"name"`
}

type Owner struct {
	Login string `json:"login"`
}

type workflowJobPayload struct {
	Action      Action      `json:"action"`
	WorkflowJob WorkflowJob `json:"workflow_job"`
	Repository  Repository  `json:"repository"`
}

// NormalizeWorkflowJob parses and validates a workflow_job webhook. deliveryID
// should be the X-GitHub-Delivery header; it is used as the primary event ID.
func NormalizeWorkflowJob(payload []byte, deliveryID string, receivedAt time.Time) (WorkflowJobEvent, error) {
	var raw workflowJobPayload
	if err := json.Unmarshal(payload, &raw); err != nil {
		return WorkflowJobEvent{}, fmt.Errorf("decode workflow_job payload: %w", err)
	}
	if !raw.Action.valid() {
		return WorkflowJobEvent{}, fmt.Errorf("unsupported workflow_job action %q", raw.Action)
	}
	if raw.WorkflowJob.ID <= 0 {
		return WorkflowJobEvent{}, errors.New("workflow_job payload has no valid job id")
	}
	if raw.WorkflowJob.RunID <= 0 {
		return WorkflowJobEvent{}, errors.New("workflow_job payload has no valid run id")
	}
	if raw.WorkflowJob.RunAttempt <= 0 {
		raw.WorkflowJob.RunAttempt = 1
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	} else {
		receivedAt = receivedAt.UTC()
	}

	// Repository is top-level in GitHub's webhook schema. Keep the nested
	// fallback for older fixtures and callers that already shaped the payload.
	if raw.Repository.FullName != "" {
		raw.WorkflowJob.Repository = raw.Repository
	}
	repo := strings.TrimSpace(raw.WorkflowJob.Repository.FullName)
	jobKey := fmt.Sprintf("%s/job/%d", repo, raw.WorkflowJob.ID)
	runKey := fmt.Sprintf("%s/run/%d/attempt/%d", repo, raw.WorkflowJob.RunID, raw.WorkflowJob.RunAttempt)
	eventID := strings.TrimSpace(deliveryID)
	dedupKey := eventID
	if dedupKey == "" {
		dedupKey = fmt.Sprintf("%s/action/%s", jobKey, raw.Action)
		eventID = dedupKey
	}

	return WorkflowJobEvent{
		EventName: WorkflowJobEventName, EventID: eventID, DeliveryID: strings.TrimSpace(deliveryID),
		Action: raw.Action, ReceivedAt: receivedAt, DedupKey: dedupKey, JobKey: jobKey, RunKey: runKey,
		Job: raw.WorkflowJob,
	}, nil
}

// NormalizeWorkflowJobHeaders extracts delivery and event identity from
// GitHub headers before normalizing the JSON payload.
func NormalizeWorkflowJobHeaders(payload []byte, headers http.Header, receivedAt time.Time) (WorkflowJobEvent, error) {
	eventName := strings.TrimSpace(headerValue(headers, "X-GitHub-Event"))
	if eventName != "" && eventName != WorkflowJobEventName {
		return WorkflowJobEvent{}, fmt.Errorf("unexpected GitHub event %q", eventName)
	}
	deliveryID := strings.TrimSpace(headerValue(headers, "X-GitHub-Delivery"))
	return NormalizeWorkflowJob(payload, deliveryID, receivedAt)
}

func headerValue(headers http.Header, name string) string {
	if value := headers.Get(name); value != "" {
		return value
	}
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
