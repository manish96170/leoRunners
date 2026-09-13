package github

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrScopeDenied  = errors.New("GitHub workload is outside the approved scope")
	ErrInvalidScope = errors.New("invalid GitHub workload scope")
)

// ScopePolicy is the fail-closed admission contract for webhook workloads.
// Empty allowlists are intentionally invalid: production callers must make
// the organization/repository and label boundary explicit.
type ScopePolicy struct {
	Organizations []string
	Repositories  []string
	AllowedLabels []string
	AllowForks    bool
}

func (p ScopePolicy) Validate() error {
	if len(p.Organizations) == 0 && len(p.Repositories) == 0 {
		return fmt.Errorf("%w: organization or repository allowlist is required", ErrInvalidScope)
	}
	if len(p.AllowedLabels) == 0 {
		return fmt.Errorf("%w: label allowlist is required", ErrInvalidScope)
	}
	seen := map[string]struct{}{}
	for _, value := range append(append([]string{}, p.Organizations...), append(p.Repositories, p.AllowedLabels...)...) {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%w: allowlist contains an empty or multiline value", ErrInvalidScope)
		}
		key := strings.ToLower(strings.TrimSpace(value))
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate allowlist value %q", ErrInvalidScope, value)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ValidateEvent applies organization/repository, label, and fork boundaries
// before a workflow job can reach scheduling or JIT generation.
func (p ScopePolicy) ValidateEvent(event WorkflowJobEvent) error {
	if err := p.Validate(); err != nil {
		return err
	}
	repository := strings.ToLower(strings.TrimSpace(event.Job.Repository.FullName))
	if repository == "" {
		return fmt.Errorf("%w: repository is missing", ErrScopeDenied)
	}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("%w: repository must be owner/name", ErrScopeDenied)
	}
	if event.Job.Repository.IsFork && !p.AllowForks {
		return fmt.Errorf("%w: fork repositories are denied", ErrScopeDenied)
	}
	if !containsFold(p.Organizations, parts[0]) && !containsFold(p.Repositories, repository) {
		return fmt.Errorf("%w: repository %q is not approved", ErrScopeDenied, repository)
	}
	for _, label := range event.Job.Labels {
		if !containsFold(p.AllowedLabels, label) {
			return fmt.Errorf("%w: label %q is not approved", ErrScopeDenied, label)
		}
	}
	return nil
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// CanonicalLabels returns a stable, duplicate-free label set for API requests.
func CanonicalLabels(labels []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, label)
	}
	sort.Strings(result)
	return result
}
