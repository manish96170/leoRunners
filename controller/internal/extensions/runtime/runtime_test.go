package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	controllerconfig "github.com/leo-runners/ci-platform/controller/internal/config"
	"github.com/leo-runners/ci-platform/controller/internal/events"
	"github.com/leo-runners/ci-platform/controller/internal/extensions"
	"github.com/leo-runners/ci-platform/controller/internal/extensions/consumers"
)

func enabledConfig() controllerconfig.ExtensionsConfig {
	return controllerconfig.ExtensionsConfig{Enabled: true, TenantAllowlist: []string{"tenant-a"}, QueueSize: 4, Timeout: time.Second, ReportBuffer: 4}
}

func TestBuildDefaultsDisabledAndRegistersNothing(t *testing.T) {
	cfg := controllerconfig.Defaults().Extensions
	result, err := Build(cfg, Dependencies{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Registry.List()) != 0 || result.Policy != nil {
		t.Fatalf("disabled result registered extensions or policy: %+v", result.Registry.List())
	}
}

func TestBuildRegistersFourConsumersAndScopesTenants(t *testing.T) {
	sink := consumers.NewMemorySink(8)
	result, err := Build(enabledConfig(), Dependencies{SummarySink: sink, OutputSink: sink})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := len(result.Registry.List()); got != 4 {
		t.Fatalf("registered %d extensions, want 4", got)
	}
	bus := events.NewBus(events.BusConfig{Buffer: 8})
	dispatcher, err := extensions.NewDispatcher(bus, result.Registry, result.DispatcherConfig)
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	if err := dispatcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer dispatcher.Close()
	for _, tenant := range []string{"tenant-b", "tenant-a"} {
		event, err := events.New(events.Event{Kind: events.KindJob, Type: "job.completed", Attributes: map[string]string{"tenant_id": tenant}})
		if err != nil {
			t.Fatal(err)
		}
		if err := bus.Publish(event); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(30 * time.Millisecond)
	if got := len(sink.Summaries()); got != 3 {
		t.Fatalf("summaries = %d, want one per summary consumer for tenant-a", got)
	}
}

func TestBuildTenantIsolationAllowsOnlyConfiguredTenants(t *testing.T) {
	sink := consumers.NewMemorySink(32)
	result, err := Build(enabledConfig(), Dependencies{SummarySink: sink, OutputSink: sink})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	bus := events.NewBus(events.BusConfig{Buffer: 16})
	dispatcher, err := extensions.NewDispatcher(bus, result.Registry, result.DispatcherConfig)
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	if err := dispatcher.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer dispatcher.Close()

	for _, attrs := range []map[string]string{
		{"tenant_id": "tenant-a", "cache_outcome": "hit"},
		{"tenant": "tenant-a", "trust_class": "fork"},
		{"tenant_id": "tenant-b", "trust_class": "fork"},
		{"trust_class": "fork"},
	} {
		event, err := events.New(events.Event{Kind: events.KindJob, Type: "job.completed", Job: events.JobMetadata{ID: "job-1"}, Attributes: attrs})
		if err != nil {
			t.Fatal(err)
		}
		if err := bus.Publish(event); err != nil {
			t.Fatal(err)
		}
	}

	waitFor(t, func() bool { return len(sink.Summaries()) == 6 && len(sink.Outputs()) == 1 })
	for _, summary := range sink.Summaries() {
		if summary.TenantID != "tenant-a" {
			t.Fatalf("summary crossed tenant boundary: %+v", summary)
		}
	}
	output := sink.Outputs()[0]
	if output.TenantID != "tenant-a" {
		t.Fatalf("policy output tenant = %q, want tenant-a", output.TenantID)
	}
	if output.ExtensionID != "security" {
		t.Fatalf("policy output extension = %q, want security", output.ExtensionID)
	}
}

func TestScopedRejectsMissingTenantBeforeExtension(t *testing.T) {
	sink := consumers.NewMemorySink(8)
	extension := &scoped{extension: consumers.NewAnalytics(sink), tenants: map[string]struct{}{"tenant-a": {}}}
	event, err := events.New(events.Event{Kind: events.KindJob, Type: "job.completed", Attributes: map[string]string{"trust_class": "fork"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := extension.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got := len(sink.Summaries()); got != 0 {
		t.Fatalf("missing tenant produced %d summaries", got)
	}
	if got := len(sink.Outputs()); got != 0 {
		t.Fatalf("missing tenant produced %d policy outputs", got)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestBuildRejectsWildcardOrMissingTenants(t *testing.T) {
	for _, cfg := range []controllerconfig.ExtensionsConfig{
		{Enabled: true, QueueSize: 1, Timeout: time.Second, ReportBuffer: 1},
		{Enabled: true, TenantAllowlist: []string{"*"}, QueueSize: 1, Timeout: time.Second, ReportBuffer: 1},
	} {
		if _, err := Build(cfg, Dependencies{}); !errors.Is(err, controllerconfig.ErrInvalid) {
			t.Fatalf("Build(%+v) error = %v, want config error", cfg, err)
		}
	}
}

func TestBuildValidatesBounds(t *testing.T) {
	cfg := enabledConfig()
	cfg.QueueSize = 4097
	if _, err := Build(cfg, Dependencies{}); err == nil {
		t.Fatal("oversized queue unexpectedly accepted")
	}
}
