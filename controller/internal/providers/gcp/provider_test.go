package gcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2"
	providers "github.com/leo-runners/ci-platform/controller/internal/providers"
)

type mockOperation struct {
	waitErr error
	waits   int
}

func (o *mockOperation) Wait(ctx context.Context, _ ...gax.CallOption) error {
	o.waits++
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return o.waitErr
	}
}

type mockInstances struct {
	insertRequest                    *computepb.InsertInstanceRequest
	getRequests                      []*computepb.GetInstanceRequest
	deleteRequest                    *computepb.DeleteInstanceRequest
	insertOperation, deleteOperation *mockOperation
	insertErr, getErr, deleteErr     error
	statuses                         []computepb.Instance_Status
}

func (m *mockInstances) Insert(_ context.Context, req *computepb.InsertInstanceRequest, _ ...gax.CallOption) (Operation, error) {
	m.insertRequest = req
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	if m.insertOperation == nil {
		m.insertOperation = &mockOperation{}
	}
	return m.insertOperation, nil
}
func (m *mockInstances) Get(_ context.Context, req *computepb.GetInstanceRequest, _ ...gax.CallOption) (*computepb.Instance, error) {
	m.getRequests = append(m.getRequests, req)
	if m.getErr != nil {
		return nil, m.getErr
	}
	status := computepb.Instance_RUNNING
	if len(m.statuses) > 0 {
		status, m.statuses = m.statuses[0], m.statuses[1:]
	}
	return &computepb.Instance{Name: stringPtr(req.Instance), Status: stringPtr(status.String())}, nil
}
func (m *mockInstances) Delete(_ context.Context, req *computepb.DeleteInstanceRequest, _ ...gax.CallOption) (Operation, error) {
	m.deleteRequest = req
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	if m.deleteOperation == nil {
		m.deleteOperation = &mockOperation{}
	}
	return m.deleteOperation, nil
}

func testConfig() Config {
	return Config{Project: "ci-project", Zone: "us-central1-a", InstanceTemplate: "global/instanceTemplates/runner", NamePrefix: "leo-runner", PollInterval: time.Millisecond, ReadyTimeout: time.Second, OperationTimeout: time.Second}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing project", func(c *Config) { c.Project = "" }},
		{"missing zone", func(c *Config) { c.Zone = "" }},
		{"missing template", func(c *Config) { c.InstanceTemplate = "" }},
		{"bad prefix", func(c *Config) { c.NamePrefix = "Bad Prefix" }},
		{"negative timeout", func(c *Config) { c.ReadyTimeout = -time.Second }},
		{"bad label", func(c *Config) { c.Labels = map[string]string{"Team": "platform"} }},
		{"reserved label", func(c *Config) { c.Labels = map[string]string{runnerLabel: "override"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig()
			test.mutate(&config)
			if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestProvisionWaitsForOperationAndBuildsLabels(t *testing.T) {
	client := &mockInstances{}
	provider, err := NewWithClient(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	provider.newID = func() (string, error) { return "runner-fixed", nil }
	provider.now = func() time.Time { return time.Unix(100, 0).UTC() }
	spec := providers.RunnerSpec{ExpiresAt: time.Unix(200, 0), Metadata: map[string]string{MetadataAttemptKey: "job-7-attempt-1", "repository": "Org/Repo", "workflow": "Build / Test", "run_id": "42", "job_id": "job-7"}}
	instance, err := provider.Provision(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if instance.Provider != "gcp" || instance.ProviderID != "leo-runner-fixed" || instance.Status != providers.StatusProvisioning {
		t.Fatalf("unexpected instance: %+v", instance)
	}
	if client.insertOperation.waits != 1 {
		t.Fatalf("insert waits = %d, want 1", client.insertOperation.waits)
	}
	req := client.insertRequest
	if req == nil || req.Project != "ci-project" || req.Zone != "us-central1-a" || req.InstanceResource == nil {
		t.Fatalf("unexpected insert request: %+v", req)
	}
	resource := req.InstanceResource
	if resource.GetName() != "leo-runner-fixed" || req.GetSourceInstanceTemplate() != testConfig().InstanceTemplate || req.GetRequestId() == "" {
		t.Fatalf("unexpected resource: %+v", resource)
	}
	want := map[string]string{managedByLabel: "leo-runners", runnerLabel: "runner-fixed", repositoryLabel: "org-repo", workflowLabel: "build-test", runLabel: "42", jobLabel: "job-7", createdLabel: "t19700101t000140z", expiryLabel: "t19700101t000320z"}
	for key, value := range want {
		if resource.Labels[key] != value {
			t.Errorf("label %q = %q, want %q", key, resource.Labels[key], value)
		}
	}
}

func TestWaitReadyPollsAndMapsStatuses(t *testing.T) {
	client := &mockInstances{statuses: []computepb.Instance_Status{computepb.Instance_PROVISIONING, computepb.Instance_STAGING, computepb.Instance_RUNNING}}
	provider, err := NewWithClient(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.WaitReady(context.Background(), providers.RunnerInstance{ProviderID: "runner-vm"}); err != nil {
		t.Fatal(err)
	}
	if len(client.getRequests) != 3 {
		t.Fatalf("Get calls = %d, want 3", len(client.getRequests))
	}
}

func TestStatusMapsTerminalAndNotFound(t *testing.T) {
	for status, want := range map[computepb.Instance_Status]providers.RunnerStatus{computepb.Instance_STOPPING: providers.StatusTerminating, computepb.Instance_TERMINATED: providers.StatusTerminated, computepb.Instance_REPAIRING: providers.StatusFailed} {
		client := &mockInstances{statuses: []computepb.Instance_Status{status}}
		provider, err := NewWithClient(client, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		got, err := provider.Status(context.Background(), providers.RunnerInstance{ProviderID: "runner-vm"})
		if err != nil || got != want {
			t.Errorf("Status(%v) = %v, %v; want %v", status, got, err, want)
		}
	}
	client := &mockInstances{getErr: errors.New("404 Not Found")}
	provider, err := NewWithClient(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Status(context.Background(), providers.RunnerInstance{ProviderID: "runner-vm"}); !errors.Is(err, ErrInstanceMissing) {
		t.Fatalf("Status() error = %v, want ErrInstanceMissing", err)
	}
}

func TestTerminateWaitsAndIsIdempotentForNotFound(t *testing.T) {
	client := &mockInstances{}
	provider, err := NewWithClient(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Terminate(context.Background(), providers.RunnerInstance{ProviderID: "runner-vm"}); err != nil {
		t.Fatal(err)
	}
	if client.deleteRequest == nil || client.deleteRequest.Instance != "runner-vm" || client.deleteOperation.waits != 1 {
		t.Fatalf("unexpected delete: %+v operation=%+v", client.deleteRequest, client.deleteOperation)
	}
	client = &mockInstances{deleteErr: errors.New("instance not found")}
	provider, err = NewWithClient(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Terminate(context.Background(), providers.RunnerInstance{ProviderID: "runner-vm"}); err != nil {
		t.Fatalf("not-found Terminate() = %v", err)
	}
}

func TestWaitReadyHonorsCancellation(t *testing.T) {
	client := &mockInstances{statuses: []computepb.Instance_Status{computepb.Instance_PROVISIONING}}
	provider, err := NewWithClient(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.WaitReady(ctx, providers.RunnerInstance{ProviderID: "runner-vm"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitReady() = %v, want context.Canceled", err)
	}
}

func TestMetadataLabelsAreSafeAndConfigLabelsCannotOverrideOwnership(t *testing.T) {
	client := &mockInstances{}
	config := testConfig()
	config.Labels = map[string]string{"team": "platform"}
	provider, err := NewWithClient(client, config)
	if err != nil {
		t.Fatal(err)
	}
	provider.newID = func() (string, error) { return "runner-fixed", nil }
	_, err = provider.Provision(context.Background(), providers.RunnerSpec{Metadata: map[string]string{MetadataAttemptKey: "attempt-1", "repository": "../../SECRET"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(client.insertRequest.InstanceResource.Labels[repositoryLabel], "/") {
		t.Fatal("unsafe slash in label")
	}
}
