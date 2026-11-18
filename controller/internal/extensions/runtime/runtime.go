// Package runtime assembles the built-in read-only extensions behind the
// controller's explicit tenant policy boundary.
package runtime

import (
	"context"
	"fmt"
	"strings"

	controllerconfig "github.com/leo-runners/ci-platform/controller/internal/config"
	"github.com/leo-runners/ci-platform/controller/internal/events"
	"github.com/leo-runners/ci-platform/controller/internal/extensions"
	"github.com/leo-runners/ci-platform/controller/internal/extensions/consumers"
	"github.com/leo-runners/ci-platform/controller/internal/extensions/policy"
	"github.com/leo-runners/ci-platform/controller/internal/observability"
)

const (
	analyticsID = "analytics"
	cacheID     = "cache"
	costID      = "cost"
	securityID  = "security"
)

// Dependencies are deliberately limited to read-only sinks and display data.
// No provider, lifecycle, credential, or control-plane dependency can be
// registered through this builder.
type Dependencies struct {
	SummarySink consumers.SummarySink
	OutputSink  consumers.OutputSink
	CostRates   map[string]float64
	MetricsSink observability.MetricSink
}

type Result struct {
	Registry         *extensions.Registry
	DispatcherConfig extensions.Config
	Policy           *policy.Adapter
}

// Build registers all four built-in consumers only when cfg.Enabled is true.
// Every enabled consumer is tenant-scoped before it receives an event, and
// security output is additionally evaluated by the advisory policy adapter.
func Build(cfg controllerconfig.ExtensionsConfig, deps Dependencies) (*Result, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("extension configuration: %w", err)
	}
	registry := extensions.NewRegistry()
	result := &Result{Registry: registry, DispatcherConfig: extensions.Config{
		QueueSize: cfg.QueueSize, Timeout: cfg.Timeout, ReportBuffer: cfg.ReportBuffer,
		MetricsSink: deps.MetricsSink,
	}}
	if !cfg.Enabled {
		return result, nil
	}

	tenantSet := make(map[string]struct{}, len(cfg.TenantAllowlist))
	policyTenants := make(map[string]policy.TenantPolicy, len(cfg.TenantAllowlist))
	for _, tenant := range cfg.TenantAllowlist {
		tenant = strings.TrimSpace(tenant)
		tenantSet[tenant] = struct{}{}
		policyTenants[tenant] = policy.TenantPolicy{Enabled: true, Extensions: map[string]bool{
			analyticsID: true, cacheID: true, costID: true, securityID: true,
		}}
	}
	adapter, err := policy.New(policy.Config{Enabled: true, Tenants: policyTenants})
	if err != nil {
		return nil, fmt.Errorf("extension policy: %w", err)
	}
	result.Policy = adapter
	outputSink := policySink{adapter: adapter, sink: deps.OutputSink}

	items := []extensions.Extension{
		&scoped{extension: consumers.NewAnalytics(deps.SummarySink), tenants: tenantSet},
		&scoped{extension: consumers.NewCache(deps.SummarySink), tenants: tenantSet},
		&scoped{extension: consumers.NewCost(deps.SummarySink, deps.CostRates), tenants: tenantSet},
		&scoped{extension: consumers.NewSecurity(outputSink), tenants: tenantSet},
	}
	for _, item := range items {
		if err := registry.Register(item); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type scoped struct {
	extension extensions.Extension
	tenants   map[string]struct{}
}

type tenantContextKey struct{}

func (s *scoped) Metadata() extensions.Metadata { return s.extension.Metadata() }

func (s *scoped) Handle(ctx context.Context, event events.Event) error {
	tenant := strings.TrimSpace(event.Attributes["tenant_id"])
	if tenant == "" {
		tenant = strings.TrimSpace(event.Attributes["tenant"])
	}
	if _, ok := s.tenants[tenant]; !ok {
		return nil
	}
	return s.extension.Handle(context.WithValue(ctx, tenantContextKey{}, tenant), event)
}

type policySink struct {
	adapter *policy.Adapter
	sink    consumers.OutputSink
}

func (s policySink) EmitOutput(ctx context.Context, output policy.Output) error {
	if s.sink == nil {
		return nil
	}
	if output.TenantID == "" {
		if tenant, ok := ctx.Value(tenantContextKey{}).(string); ok {
			output.TenantID = tenant
		}
	}
	decision, err := s.adapter.Evaluate(output.TenantID, output.ExtensionID, output)
	if err != nil {
		return err
	}
	return s.sink.EmitOutput(ctx, decision.Output)
}
