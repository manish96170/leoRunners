package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/runners"
)

type recordedGitHub struct {
	mu    sync.Mutex
	paths []string
}

func (r *recordedGitHub) record(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

func (r *recordedGitHub) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func fakeGitHub(t *testing.T) (*github.Client, *recordedGitHub) {
	t.Helper()
	recorded := &recordedGitHub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.record(r.Method + " " + r.URL.Path)
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"runner":{"id":5,"name":"runner-5"},"encoded_jit_config":"encoded"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(github.RunnerList{Runners: []github.JITRunner{{ID: 5, Name: "runner-5", Status: "online"}}})
	}))
	t.Cleanup(server.Close)
	client, err := github.NewJITClient(github.JITClientConfig{BaseURL: server.URL, TokenSource: github.StaticBearerToken("token"), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return client, recorded
}

func TestAdaptersRegisterOnTheRepositoryByDefault(t *testing.T) {
	client, recorded := fakeGitHub(t)
	group := int64(3)
	config, err := GitHubJITAdapter{Client: client}.GenerateJITConfig(context.Background(), runners.JITRequest{Repository: "acme/widgets", Name: "runner-5", Labels: []string{"self-hosted"}, RunnerGroupID: &group})
	if err != nil || config.RunnerID != 5 || config.EncodedConfig != "encoded" {
		t.Fatalf("config = %+v, error = %v", config, err)
	}
	verifier, _ := github.NewRegistrationVerifier(client, github.RegistrationVerifierConfig{Timeout: time.Second, Interval: time.Millisecond})
	if _, err := (GitHubRegistrationAdapter{Verifier: verifier}).WaitForRegistration(context.Background(), runners.RegistrationRequest{Repository: "acme/widgets", RunnerID: 5, RunnerName: "runner-5"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /repos/acme/widgets/actions/runners/generate-jitconfig", "GET /repos/acme/widgets/actions/runners"}
	if got := recorded.seen(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestAdaptersRegisterOnTheOrganizationWhenAsked(t *testing.T) {
	client, recorded := fakeGitHub(t)
	group := int64(3)
	config, err := GitHubJITAdapter{Client: client, Organization: true}.GenerateJITConfig(context.Background(), runners.JITRequest{Repository: "acme/widgets", Name: "runner-5", Labels: []string{"self-hosted"}, RunnerGroupID: &group})
	if err != nil || config.RunnerID != 5 {
		t.Fatalf("config = %+v, error = %v", config, err)
	}
	verifier, _ := github.NewRegistrationVerifier(client, github.RegistrationVerifierConfig{Timeout: time.Second, Interval: time.Millisecond})
	if _, err := (GitHubRegistrationAdapter{Verifier: verifier, Organization: true}).WaitForRegistration(context.Background(), runners.RegistrationRequest{Repository: "acme/widgets", RunnerID: 5, RunnerName: "runner-5"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /orgs/acme/actions/runners/generate-jitconfig", "GET /orgs/acme/actions/runners"}
	if got := recorded.seen(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("calls = %v, want %v (nothing may touch the repository path)", got, want)
	}
}
