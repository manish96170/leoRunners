// Package policy defines the controller boundary for extension output.
// Extensions can advise operators, but they cannot control platform state.
package policy

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrDisabled        = errors.New("extension policy is disabled")
	ErrTenantDenied    = errors.New("tenant is not enabled for extension")
	ErrExtensionDenied = errors.New("extension is not enabled for tenant")
	ErrInvalidOutput   = errors.New("invalid extension output")
	ErrPolicyViolation = errors.New("extension output contains a forbidden action")
)

const CurrentVersion = "v1"

// AdvisoryAction is descriptive output. No value in this package is executed
// by the controller.
type AdvisoryAction string

const (
	ActionNone      AdvisoryAction = "none"
	ActionAnnotate  AdvisoryAction = "annotate"
	ActionExplain   AdvisoryAction = "explain"
	ActionRecommend AdvisoryAction = "recommend"
	ActionLink      AdvisoryAction = "link"
)

// ExtensionAction is deliberately a string so untrusted extensions can be
// decoded without expanding the controller's executable capability set.
type ExtensionAction struct {
	Action    AdvisoryAction `json:"action"`
	Title     string         `json:"title,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Rationale string         `json:"rationale,omitempty"`
	URL       string         `json:"url,omitempty"`
}

type Annotation struct {
	Path      string `json:"path"`
	StartLine int    `json:"startLine,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	Level     string `json:"level"`
	Title     string `json:"title,omitempty"`
	Message   string `json:"message"`
}

// Output is the only data an extension may return to the policy adapter.
// Metadata is bounded and treated as display data, never as instructions.
type Output struct {
	Version     string            `json:"version"`
	ExtensionID string            `json:"extensionId"`
	TenantID    string            `json:"tenantId"`
	JobID       string            `json:"jobId"`
	Actions     []ExtensionAction `json:"actions,omitempty"`
	Annotations []Annotation      `json:"annotations,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type TenantPolicy struct {
	Enabled    bool
	Extensions map[string]bool
}

type Config struct {
	Enabled        bool
	Tenants        map[string]TenantPolicy
	MaxActions     int
	MaxAnnotations int
	MaxMetadata    int
}

func (c Config) withDefaults() Config {
	if c.MaxActions <= 0 {
		c.MaxActions = 32
	}
	if c.MaxAnnotations <= 0 {
		c.MaxAnnotations = 64
	}
	if c.MaxMetadata <= 0 {
		c.MaxMetadata = 32
	}
	return c
}

func (c Config) Validate() error {
	if c.MaxActions <= 0 || c.MaxAnnotations <= 0 || c.MaxMetadata <= 0 {
		return fmt.Errorf("%w: output limits must be positive", ErrInvalidOutput)
	}
	for tenant, setting := range c.Tenants {
		if strings.TrimSpace(tenant) == "" {
			return fmt.Errorf("%w: tenant ID is empty", ErrInvalidOutput)
		}
		for extension := range setting.Extensions {
			if strings.TrimSpace(extension) == "" {
				return fmt.Errorf("%w: extension ID is empty", ErrInvalidOutput)
			}
		}
	}
	return nil
}

type Decision struct {
	Accepted        bool
	Output          Output
	RejectedActions []AdvisoryAction
}

type Adapter struct{ config Config }

func New(config Config) (*Adapter, error) {
	config = config.withDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Adapter{config: config}, nil
}

// Evaluate authorizes and normalizes one extension result. It never invokes
// an extension and never performs a lifecycle, security, or credential action.
func (a *Adapter) Evaluate(tenantID, extensionID string, output Output) (Decision, error) {
	if a == nil || !a.config.Enabled {
		return Decision{}, ErrDisabled
	}
	tenantID, extensionID = strings.TrimSpace(tenantID), strings.TrimSpace(extensionID)
	setting, ok := a.config.Tenants[tenantID]
	if !ok || !setting.Enabled {
		return Decision{}, ErrTenantDenied
	}
	if !setting.Extensions[extensionID] {
		return Decision{}, ErrExtensionDenied
	}
	if err := validateIdentity(output, tenantID, extensionID); err != nil {
		return Decision{}, err
	}
	if len(output.Actions) > a.config.MaxActions || len(output.Annotations) > a.config.MaxAnnotations || len(output.Metadata) > a.config.MaxMetadata {
		return Decision{}, fmt.Errorf("%w: output exceeds configured limits", ErrInvalidOutput)
	}

	decision := Decision{Output: cloneOutput(output)}
	for _, action := range output.Actions {
		if !isAllowed(action.Action) {
			decision.RejectedActions = append(decision.RejectedActions, action.Action)
		}
	}
	if len(decision.RejectedActions) > 0 {
		return decision, fmt.Errorf("%w: %s", ErrPolicyViolation, strings.Join(actionNames(decision.RejectedActions), ","))
	}
	if err := validateAnnotations(decision.Output.Annotations); err != nil {
		return Decision{}, err
	}
	decision.Output.Version = CurrentVersion
	decision.Output.TenantID = tenantID
	decision.Output.ExtensionID = extensionID
	decision.Output = normalize(decision.Output)
	decision.Accepted = true
	return decision, nil
}

func validateIdentity(output Output, tenantID, extensionID string) error {
	if output.Version != "" && output.Version != CurrentVersion {
		return fmt.Errorf("%w: unsupported version", ErrInvalidOutput)
	}
	if output.TenantID != "" && output.TenantID != tenantID {
		return fmt.Errorf("%w: tenant mismatch", ErrInvalidOutput)
	}
	if output.ExtensionID != "" && output.ExtensionID != extensionID {
		return fmt.Errorf("%w: extension mismatch", ErrInvalidOutput)
	}
	if strings.TrimSpace(output.JobID) == "" {
		return fmt.Errorf("%w: job ID is required", ErrInvalidOutput)
	}
	return nil
}

func isAllowed(action AdvisoryAction) bool {
	switch strings.ToLower(strings.TrimSpace(string(action))) {
	case "", string(ActionNone), string(ActionAnnotate), string(ActionExplain), string(ActionRecommend), string(ActionLink):
		return true
	default:
		return false
	}
}

func validateAnnotations(in []Annotation) error {
	for _, annotation := range in {
		if strings.TrimSpace(annotation.Path) == "" || strings.TrimSpace(annotation.Message) == "" {
			return fmt.Errorf("%w: annotation path and message are required", ErrInvalidOutput)
		}
		if annotation.StartLine < 0 || annotation.EndLine < 0 || (annotation.EndLine != 0 && annotation.StartLine > annotation.EndLine) {
			return fmt.Errorf("%w: invalid annotation line range", ErrInvalidOutput)
		}
		level := strings.ToLower(strings.TrimSpace(annotation.Level))
		if level != "notice" && level != "warning" && level != "failure" {
			return fmt.Errorf("%w: invalid annotation level", ErrInvalidOutput)
		}
	}
	return nil
}

func normalize(in Output) Output {
	out := cloneOutput(in)
	out.Version = CurrentVersion
	out.TenantID = strings.TrimSpace(out.TenantID)
	out.ExtensionID = strings.TrimSpace(out.ExtensionID)
	out.JobID = strings.TrimSpace(out.JobID)
	for i := range out.Actions {
		out.Actions[i].Action = AdvisoryAction(strings.ToLower(strings.TrimSpace(string(out.Actions[i].Action))))
	}
	for i := range out.Annotations {
		out.Annotations[i].Path = strings.TrimSpace(out.Annotations[i].Path)
		out.Annotations[i].Level = strings.ToLower(strings.TrimSpace(out.Annotations[i].Level))
		out.Annotations[i].Title = strings.TrimSpace(out.Annotations[i].Title)
		out.Annotations[i].Message = strings.TrimSpace(out.Annotations[i].Message)
	}
	sort.SliceStable(out.Actions, func(i, j int) bool { return actionKey(out.Actions[i]) < actionKey(out.Actions[j]) })
	sort.SliceStable(out.Annotations, func(i, j int) bool { return annotationKey(out.Annotations[i]) < annotationKey(out.Annotations[j]) })
	out.Actions = uniqueActions(out.Actions)
	out.Annotations = uniqueAnnotations(out.Annotations)
	return out
}

func actionKey(a ExtensionAction) string {
	return strings.Join([]string{string(a.Action), a.Title, a.Summary, a.Rationale, a.URL}, "\x00")
}
func annotationKey(a Annotation) string {
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%s\x00%s", a.Path, a.StartLine, a.EndLine, a.Level, a.Title, a.Message)
}

func uniqueActions(in []ExtensionAction) []ExtensionAction {
	out := in[:0]
	seen := map[string]bool{}
	for _, v := range in {
		k := actionKey(v)
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}
func uniqueAnnotations(in []Annotation) []Annotation {
	out := in[:0]
	seen := map[string]bool{}
	for _, v := range in {
		k := annotationKey(v)
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}

func cloneOutput(in Output) Output {
	out := in
	out.Actions = append([]ExtensionAction(nil), in.Actions...)
	out.Annotations = append([]Annotation(nil), in.Annotations...)
	if in.Metadata != nil {
		out.Metadata = make(map[string]string, len(in.Metadata))
		for k, v := range in.Metadata {
			out.Metadata[k] = v
		}
	}
	return out
}

func actionNames(in []AdvisoryAction) []string {
	out := make([]string, len(in))
	for i, action := range in {
		out[i] = string(action)
	}
	return out
}
