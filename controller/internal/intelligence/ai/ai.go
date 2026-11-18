// Package ai defines an optional, read-only boundary for CI failure analysis.
// It deliberately has no dependency on providers, lifecycle, credentials, or
// security policy implementations.
package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrDisabled        = errors.New("AI analysis is disabled")
	ErrNoTrigger       = errors.New("AI analysis trigger did not match")
	ErrInvalidRequest  = errors.New("invalid AI analysis request")
	ErrPolicyViolation = errors.New("AI response contains a forbidden action")
)

type Classification string

const (
	ClassificationTestFailure       Classification = "test-failure"
	ClassificationDependencyFailure Classification = "dependency-failure"
	ClassificationInfrastructure    Classification = "infrastructure-failure"
	ClassificationTimeout           Classification = "timeout"
	ClassificationCancellation      Classification = "cancellation"
	ClassificationConfiguration     Classification = "configuration-failure"
	ClassificationUnknown           Classification = "unknown"
)

// SuggestedAction is descriptive output only. The analyzer never executes it.
// The policy gate permits only observations and annotations.
type SuggestedAction string

const (
	ActionNone             SuggestedAction = "none"
	ActionAnnotate         SuggestedAction = "annotate"
	ActionRetry            SuggestedAction = "retry"
	ActionRerun            SuggestedAction = "rerun"
	ActionCancel           SuggestedAction = "cancel"
	ActionProvision        SuggestedAction = "provision"
	ActionTerminate        SuggestedAction = "terminate"
	ActionChangeSecurity   SuggestedAction = "change-security"
	ActionAccessCredential SuggestedAction = "access-credential"
)

// ModelProvider is the only model integration point. Implementations may be
// local or hosted, but must not receive lifecycle or credential capabilities.
type ModelProvider interface {
	Analyze(context.Context, Request) (Response, error)
}

type TriggerContext struct {
	Repository     string
	Workflow       string
	Branch         string
	PullRequest    bool
	PullRequestNum int64
	Labels         []string
	Manual         bool
}

// TriggerRule uses empty strings, nil booleans, and zero PR numbers as
// wildcards. All listed labels must be present in the incoming context.
type TriggerRule struct {
	Repository     string
	Workflow       string
	Branch         string
	PullRequest    *bool
	PullRequestNum int64
	Labels         []string
	Manual         *bool
}

func (r TriggerRule) Matches(c TriggerContext) bool {
	if r.Repository != "" && r.Repository != c.Repository {
		return false
	}
	if r.Workflow != "" && r.Workflow != c.Workflow {
		return false
	}
	if r.Branch != "" && r.Branch != c.Branch {
		return false
	}
	if r.PullRequest != nil && *r.PullRequest != c.PullRequest {
		return false
	}
	if r.PullRequestNum != 0 && r.PullRequestNum != c.PullRequestNum {
		return false
	}
	if r.Manual != nil && *r.Manual != c.Manual {
		return false
	}
	for _, want := range r.Labels {
		found := false
		for _, got := range c.Labels {
			if want == got {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

type FailureLog struct {
	JobID      string
	ExitCode   int
	Log        string
	Truncated  bool
	StartedAt  time.Time
	FinishedAt time.Time
}

type Request struct {
	Trigger       TriggerContext
	Failure       FailureLog
	CorrelationID string
	Metadata      map[string]string
}

type Explanation struct {
	Summary            string
	Evidence           []string
	Confidence         float64
	SuggestedNextSteps []string
}

type Recommendation struct {
	Action    SuggestedAction
	Rationale string
}

type Response struct {
	Classification  Classification
	Explanation     Explanation
	Recommendations []Recommendation
}

type PolicyDecision struct {
	Accepted        bool
	Classification  Classification
	Explanation     Explanation
	Recommendations []Recommendation
	RejectedActions []SuggestedAction
}

// Gate is deterministic and intentionally has no method that performs an
// action. It accepts only harmless analysis output.
type Gate struct{}

func (Gate) Evaluate(response Response) (PolicyDecision, error) {
	decision := PolicyDecision{
		Classification: response.Classification,
		Explanation:    cloneExplanation(response.Explanation),
	}
	for _, recommendation := range response.Recommendations {
		switch recommendation.Action {
		case "", ActionNone, ActionAnnotate, ActionRetry, ActionRerun:
			decision.Recommendations = append(decision.Recommendations, recommendation)
		default:
			decision.RejectedActions = append(decision.RejectedActions, recommendation.Action)
		}
	}
	if len(decision.RejectedActions) != 0 {
		return decision, fmt.Errorf("%w: %s", ErrPolicyViolation, strings.Join(stringActions(decision.RejectedActions), ","))
	}
	decision.Accepted = true
	return decision, nil
}

type Config struct {
	Enabled     bool
	Provider    ModelProvider
	Triggers    []TriggerRule
	Timeout     time.Duration
	MaxLogBytes int
	Policy      Gate
}

func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
	if c.MaxLogBytes <= 0 {
		c.MaxLogBytes = 64 * 1024
	}
	return c
}

func (c Config) Validate() error {
	if c.Timeout <= 0 {
		return errors.New("AI timeout must be positive")
	}
	if c.MaxLogBytes <= 0 {
		return errors.New("AI max log bytes must be positive")
	}
	if c.Enabled && c.Provider == nil {
		return errors.New("AI provider is required when AI is enabled")
	}
	return nil
}

type Analyzer struct {
	config Config
}

func New(config Config) (*Analyzer, error) {
	config = config.withDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Analyzer{config: config}, nil
}

func (a *Analyzer) Analyze(ctx context.Context, request Request) (PolicyDecision, error) {
	if a == nil || !a.config.Enabled {
		return PolicyDecision{}, ErrDisabled
	}
	if request.Failure.JobID == "" || request.Failure.Log == "" {
		return PolicyDecision{}, fmt.Errorf("%w: job ID and failure log are required", ErrInvalidRequest)
	}
	matched := false
	for _, trigger := range a.config.Triggers {
		if trigger.Matches(request.Trigger) {
			matched = true
			break
		}
	}
	if !matched {
		return PolicyDecision{}, ErrNoTrigger
	}
	request = cloneRequest(request)
	if len(request.Failure.Log) > a.config.MaxLogBytes {
		request.Failure.Log = request.Failure.Log[:a.config.MaxLogBytes]
		request.Failure.Truncated = true
	}
	bounded, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()
	response, err := a.config.Provider.Analyze(bounded, request)
	if err != nil {
		return PolicyDecision{}, err
	}
	return a.config.Policy.Evaluate(response)
}

func cloneRequest(in Request) Request {
	out := in
	out.Trigger.Labels = append([]string(nil), in.Trigger.Labels...)
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func cloneExplanation(in Explanation) Explanation {
	out := in
	out.Evidence = append([]string(nil), in.Evidence...)
	out.SuggestedNextSteps = append([]string(nil), in.SuggestedNextSteps...)
	return out
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func stringActions(in []SuggestedAction) []string {
	out := make([]string, len(in))
	for i, action := range in {
		out[i] = string(action)
	}
	return out
}
