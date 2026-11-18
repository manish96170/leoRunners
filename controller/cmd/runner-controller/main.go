package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/leo-runners/ci-platform/controller/internal/capacity"
	runtimeconfig "github.com/leo-runners/ci-platform/controller/internal/config"
	"github.com/leo-runners/ci-platform/controller/internal/controller"
	"github.com/leo-runners/ci-platform/controller/internal/events"
	eventarchive "github.com/leo-runners/ci-platform/controller/internal/events/archive"
	"github.com/leo-runners/ci-platform/controller/internal/extensions"
	extensionruntime "github.com/leo-runners/ci-platform/controller/internal/extensions/runtime"
	"github.com/leo-runners/ci-platform/controller/internal/github"
	"github.com/leo-runners/ci-platform/controller/internal/lifecycle"
	"github.com/leo-runners/ci-platform/controller/internal/observability"
	"github.com/leo-runners/ci-platform/controller/internal/observability/prometheus"
	"github.com/leo-runners/ci-platform/controller/internal/policy"
	"github.com/leo-runners/ci-platform/controller/internal/providers"
	awsprovider "github.com/leo-runners/ci-platform/controller/internal/providers/aws"
	gcpprovider "github.com/leo-runners/ci-platform/controller/internal/providers/gcp"
	"github.com/leo-runners/ci-platform/controller/internal/runners"
	"github.com/leo-runners/ci-platform/controller/internal/scheduler"
	"github.com/leo-runners/ci-platform/controller/internal/state"
	statedynamodb "github.com/leo-runners/ci-platform/controller/internal/state/dynamodb"
	"github.com/leo-runners/ci-platform/controller/internal/telemetry"
)

func main() {
	runtime, err := runtimeconfig.Load()
	if err != nil {
		log.Fatal(err)
	}
	repository := configureRepository()
	provider := configureProvider(repository)
	poolRegistry := configureCapacityRegistry()
	sink := &telemetry.MemorySink{}
	metrics := prometheus.New(prometheus.Config{Namespace: "leo_runner", AllowedLabels: map[string]struct{}{"event": {}, "provider": {}, "extension": {}, "region": {}, "outcome": {}}})
	eventBus := events.NewBus(events.BusConfig{Buffer: 256})
	extensionSetup, err := extensionruntime.Build(runtime.Extensions, extensionruntime.Dependencies{MetricsSink: &extensionMetricsSink{exporter: metrics}})
	if err != nil {
		log.Fatal(err)
	}
	extensionDispatcher, err := extensions.NewDispatcher(eventBus, extensionSetup.Registry, extensionSetup.DispatcherConfig)
	if err != nil {
		log.Fatal(err)
	}
	if err := extensionDispatcher.Start(); err != nil {
		log.Fatal(err)
	}
	defer extensionDispatcher.Close()
	defer eventBus.Close()
	sinks := []telemetry.Sink{sink, &metricsTelemetrySink{exporter: metrics}, &events.TelemetryPublisher{Bus: eventBus}}
	var archive *eventarchive.Archive
	if path := strings.TrimSpace(os.Getenv("EVENT_ARCHIVE_PATH")); path != "" {
		var archiveErr error
		archive, archiveErr = eventarchive.Open(path, eventarchive.Options{})
		if archiveErr != nil {
			log.Fatal(archiveErr)
		}
		defer archive.Close()
		sinks = append(sinks, archive)
	}
	serviceSink := &telemetryFanout{sinks: sinks}
	_ = metrics.IncCounter("controller_up", nil)
	service := &controller.Service{State: repository, Provider: provider, Sink: serviceSink, JobTTL: 2 * time.Hour, RunnerNamePrefix: envOrDefault("RUNNER_NAME_PREFIX", "leo-runner"), CapacityRegistry: poolRegistry}
	providerKey := envOrDefault("CAPACITY_PROVIDER", "fake")
	if providerKey == "fake" {
		if strings.TrimSpace(os.Getenv("AWS_LAUNCH_TEMPLATE_ID")) != "" || strings.TrimSpace(os.Getenv("AWS_LAUNCH_TEMPLATE_NAME")) != "" {
			providerKey = "aws"
		} else if strings.TrimSpace(os.Getenv("GCP_INSTANCE_TEMPLATE")) != "" {
			providerKey = "gcp"
		}
	}
	service.Providers = map[string]providers.Provider{providerKey: provider}
	mode := policy.Mode(envOrDefault("SCHEDULER_MODE", string(policy.Fallback)))
	poolID := envOrDefault("CAPACITY_POOL_ID", "configured")
	service.Scheduler = &scheduler.Scheduler{State: repository, Mode: mode, PoolRegistry: poolRegistry, PoolProviders: map[string]providers.Provider{poolID: provider}}
	service.Lifecycle = &lifecycle.Manager{State: repository, Provider: provider, Sink: serviceSink}
	configureGitHubAssignment(service, repository, provider)
	startReconciliation(repository, provider)

	secret := runtime.Webhook.Secret
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/metrics", metrics)
	mux.HandleFunc("/webhooks/github", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, runtime.Webhook.MaxBodyBytes))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !github.VerifyRequest(secret, r, body) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		event, err := github.NormalizeWorkflowJobHeaders(body, r.Header, time.Now().UTC())
		if err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		if err := service.HandleWorkflowJob(ctx, event); err != nil {
			log.Printf("workflow job %s failed: %v", event.JobKey, err)
			http.Error(w, "workflow processing failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	server := &http.Server{Addr: runtime.Server.ListenAddr, Handler: mux, ReadHeaderTimeout: runtime.Server.ReadHeaderTimeout}
	log.Printf("runner controller listening on %s", server.Addr)
	shutdownSignal, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	go func() {
		<-shutdownSignal.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

type telemetryFanout struct{ sinks []telemetry.Sink }

func (f *telemetryFanout) Emit(event telemetry.Event) error {
	var first error
	for _, sink := range f.sinks {
		if err := sink.Emit(event); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type metricsTelemetrySink struct{ exporter *prometheus.Exporter }

func (s *metricsTelemetrySink) Emit(event telemetry.Event) error {
	return s.exporter.RecordLifecycle(event.Type, event.Provider)
}

type extensionMetricsSink struct{ exporter *prometheus.Exporter }

func (s *extensionMetricsSink) Record(sample observability.MetricSample) error {
	labels := prometheus.Labels{}
	for key, value := range sample.Dimensions {
		labels[key] = value
	}
	return s.exporter.AddCounter(sample.Name, sample.Value, labels)
}

func configureCapacityRegistry() *capacity.Registry {
	registry := capacity.NewRegistry()
	ownership := capacity.Ownership(envOrDefault("CAPACITY_OWNERSHIP", string(capacity.Managed)))
	providerName := strings.TrimSpace(os.Getenv("CAPACITY_PROVIDER"))
	if providerName == "" {
		switch {
		case strings.TrimSpace(os.Getenv("AWS_LAUNCH_TEMPLATE_ID")) != "":
			providerName = "aws"
		case strings.TrimSpace(os.Getenv("GCP_INSTANCE_TEMPLATE")) != "":
			providerName = "gcp"
		default:
			providerName = "fake"
		}
	}
	region := envOrDefault("AWS_REGION", "us-east-1")
	if providerName == "gcp" {
		region = envOrDefault("GCP_ZONE", "us-central1-a")
	}
	labels := splitCSV(os.Getenv("CAPACITY_LABELS"))
	if len(labels) == 0 {
		labels = []string{"self-hosted", "linux", "x64", "x86_64"}
	}
	pool := capacity.Pool{ID: envOrDefault("CAPACITY_POOL_ID", "configured"), Ownership: ownership, Provider: providerName, Region: region, SecurityProfile: envOrDefault("CAPACITY_SECURITY_PROFILE", "isolated"), Availability: capacity.Available, Labels: labels, Capacity: capacity.Capacity{MaxRunners: 100}}
	if err := registry.Register(pool); err != nil {
		log.Fatal(err)
	}
	return registry
}

func configureRepository() state.Repository {
	if table := strings.TrimSpace(os.Getenv("DYNAMODB_TABLE_NAME")); table != "" {
		region := envOrDefault("AWS_REGION", "us-east-1")
		cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
		if err != nil {
			log.Fatal(err)
		}
		repository, err := statedynamodb.New(awsdynamodb.NewFromConfig(cfg), statedynamodb.Config{TableName: table, StateIndexName: envOrDefault("DYNAMODB_STATE_INDEX", "gsi1"), ExpiryIndexName: envOrDefault("DYNAMODB_EXPIRY_INDEX", "gsi2")})
		if err != nil {
			log.Fatal(err)
		}
		return repository
	}
	if path := strings.TrimSpace(os.Getenv("STATE_PATH")); path != "" {
		repository, err := state.NewFileRepository(path)
		if err != nil {
			log.Fatal(err)
		}
		return repository
	}
	return state.NewMemoryRepository()
}

func configureProvider(repository state.Repository) providers.Provider {
	region := envOrDefault("AWS_REGION", "us-east-1")
	templateID := strings.TrimSpace(os.Getenv("AWS_LAUNCH_TEMPLATE_ID"))
	templateName := strings.TrimSpace(os.Getenv("AWS_LAUNCH_TEMPLATE_NAME"))
	if templateID == "" && templateName == "" {
		if gcpTemplate := strings.TrimSpace(os.Getenv("GCP_INSTANCE_TEMPLATE")); gcpTemplate != "" {
			client, err := compute.NewInstancesRESTClient(context.Background())
			if err != nil {
				log.Fatal(err)
			}
			provider, err := gcpprovider.New(client, gcpprovider.Config{Project: os.Getenv("GCP_PROJECT"), Zone: os.Getenv("GCP_ZONE"), InstanceTemplate: gcpTemplate, NamePrefix: envOrDefault("GCP_NAME_PREFIX", "leo-runner"), UserData: os.Getenv("RUNNER_USER_DATA")})
			if err != nil {
				log.Fatal(err)
			}
			return provider
		}
		return providers.NewFakeProvider(providers.FakeConfig{Capacity: 100})
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		log.Fatal(err)
	}
	userData := os.Getenv("RUNNER_USER_DATA")
	if path := os.Getenv("RUNNER_USER_DATA_FILE"); path != "" {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			log.Fatal(readErr)
		}
		userData = string(raw)
	}
	provider, err := awsprovider.New(ec2.NewFromConfig(cfg), awsprovider.Config{Region: region, LaunchTemplateID: templateID, LaunchTemplateName: templateName, LaunchTemplateVersion: os.Getenv("AWS_LAUNCH_TEMPLATE_VERSION"), SubnetID: os.Getenv("AWS_SUBNET_ID"), SecurityGroupIDs: splitCSV(os.Getenv("AWS_SECURITY_GROUP_IDS")), InstanceProfileARN: os.Getenv("AWS_RUNNER_INSTANCE_PROFILE_ARN"), InstanceProfileName: os.Getenv("AWS_RUNNER_INSTANCE_PROFILE_NAME"), UserData: userData})
	if err != nil {
		log.Fatal(err)
	}
	_ = repository
	return provider
}

func startReconciliation(repository state.Repository, provider providers.Provider) {
	interval := 30 * time.Second
	if value := strings.TrimSpace(os.Getenv("RECONCILE_INTERVAL")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	reconciler := &lifecycle.Reconciler{State: repository, Provider: provider, OperationTimeout: 30 * time.Second}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for now := range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			report, err := reconciler.Reconcile(ctx, now.UTC())
			cancel()
			if err != nil {
				log.Printf("reconciliation errors: scanned=%d runners=%d errors=%v", report.RunnersScanned, report.RunnersRepaired, err)
			}
		}
	}()
}

func configureGitHubAssignment(service *controller.Service, repository state.Repository, provider providers.Provider) {
	token := strings.TrimSpace(os.Getenv("GITHUB_APP_INSTALLATION_TOKEN"))
	if token == "" {
		return
	}
	groupID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("GITHUB_RUNNER_GROUP_ID")), 10, 64)
	if err != nil || groupID <= 0 {
		log.Fatal("GITHUB_RUNNER_GROUP_ID must be a positive integer when JIT is enabled")
	}
	client, err := github.NewJITClient(github.JITClientConfig{TokenSource: github.StaticBearerToken(token)})
	if err != nil {
		log.Fatal(err)
	}
	verifier, err := github.NewRegistrationVerifier(client, github.RegistrationVerifierConfig{})
	if err != nil {
		log.Fatal(err)
	}
	scope := github.ScopePolicy{Organizations: splitCSV(os.Getenv("GITHUB_ALLOWED_ORGANIZATIONS")), Repositories: splitCSV(os.Getenv("GITHUB_ALLOWED_REPOSITORIES")), AllowedLabels: splitCSV(os.Getenv("GITHUB_ALLOWED_LABELS")), AllowForks: strings.EqualFold(os.Getenv("GITHUB_ALLOW_FORKS"), "true")}
	if err := scope.Validate(); err != nil {
		log.Fatalf("GitHub scope policy is required when JIT is enabled: %v", err)
	}
	service.ScopePolicy = &scope
	service.Assignment = &runners.Service{State: repository, Provider: provider, JIT: controller.GitHubJITAdapter{Client: client}, Registration: controller.GitHubRegistrationAdapter{Verifier: verifier}, RunnerGroupID: &groupID, Policy: runners.AssignmentPolicy{Organizations: append([]string(nil), scope.Organizations...), Repositories: append([]string(nil), scope.Repositories...), AllowedLabels: append([]string(nil), scope.AllowedLabels...), RunnerGroupID: groupID, AllowForks: scope.AllowForks}}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
