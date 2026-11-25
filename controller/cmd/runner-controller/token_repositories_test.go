package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestTokenRepositoriesRules(t *testing.T) {
	cases := []struct {
		name                  string
		explicit, orgs, repos []string
		want                  []string
		wantSource            string
	}{
		{"explicit setting wins", []string{"api"}, nil, []string{"acme/widgets"}, []string{"api"}, "GITHUB_APP_TOKEN_REPOSITORIES"},
		{"explicit wins even with an organization allowed", []string{"api"}, []string{"acme"}, nil, []string{"api"}, "GITHUB_APP_TOKEN_REPOSITORIES"},
		{"derived from the repository allowlist", nil, nil, []string{"acme/widgets", "acme/api"}, []string{"widgets", "api"}, "derived"},
		{"a whole organization cannot be listed", nil, []string{"acme"}, []string{"acme/widgets"}, nil, "organizations are allowed"},
		{"no allowlist, no restriction", nil, nil, nil, nil, "no repository allowlist"},
		{"several owners cannot share one installation", nil, nil, []string{"acme/widgets", "other/api"}, nil, "several owners"},
		{"a malformed entry is not guessed at", nil, nil, []string{"acme/"}, nil, "not restricted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, source := tokenRepositories(c.explicit, c.orgs, c.repos)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("repositories = %v, want %v", got, c.want)
			}
			if !strings.Contains(source, c.wantSource) {
				t.Fatalf("source = %q, want it to mention %q", source, c.wantSource)
			}
		})
	}
}
