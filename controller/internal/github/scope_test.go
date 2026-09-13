package github

import (
	"errors"
	"testing"
)

func TestScopePolicyRejectsOutOfScopeRepositoryLabelsAndForks(t *testing.T) {
	policy := ScopePolicy{Organizations: []string{"Acme"}, AllowedLabels: []string{"self-hosted", "linux"}}
	base := WorkflowJobEvent{Job: WorkflowJob{Repository: Repository{FullName: "acme/widgets"}, Labels: []string{"linux"}}}
	if err := policy.ValidateEvent(base); err != nil {
		t.Fatal(err)
	}
	for name, event := range map[string]WorkflowJobEvent{
		"repository": {Job: WorkflowJob{Repository: Repository{FullName: "other/widgets"}, Labels: []string{"linux"}}},
		"label":      {Job: WorkflowJob{Repository: Repository{FullName: "acme/widgets"}, Labels: []string{"privileged"}}},
		"fork":       {Job: WorkflowJob{Repository: Repository{FullName: "acme/widgets", IsFork: true}, Labels: []string{"linux"}}},
	} {
		if err := policy.ValidateEvent(event); !errors.Is(err, ErrScopeDenied) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

func TestCanonicalLabelsIsStableAndDeduplicated(t *testing.T) {
	got := CanonicalLabels([]string{" linux ", "self-hosted", "linux", ""})
	if len(got) != 2 || got[0] != "linux" || got[1] != "self-hosted" {
		t.Fatalf("labels = %#v", got)
	}
}
