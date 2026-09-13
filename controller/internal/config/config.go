// Package config owns the controller's process configuration boundary.
// It deliberately has no dependencies on providers or the HTTP server so it
// can validate a deployment before any cloud or GitHub client is constructed.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid controller configuration")

type ProviderName string

const (
	ProviderFake ProviderName = "fake"
	ProviderAWS  ProviderName = "aws"
	ProviderGCP  ProviderName = "gcp"
)

type CapacityMode string

const (
	CapacityCustomerFirst CapacityMode = "customer-first"
	CapacityManagedFirst  CapacityMode = "managed-first"
	CapacityCustomerOnly  CapacityMode = "customer-only"
	CapacityManagedOnly   CapacityMode = "managed-only"
	CapacityFallback      CapacityMode = "fallback"
)

type Config struct {
	Server         ServerConfig
	Webhook        WebhookConfig
	State          StateConfig
	Reconciliation ReconciliationConfig
	Provider       ProviderConfig
	Capacity       CapacityConfig
	GitHubJIT      GitHubJITConfig
	AI             AIConfig
	Extensions     ExtensionsConfig
}

type ServerConfig struct {
	ListenAddr        string
	ReadHeaderTimeout time.Duration
	MaxBodyBytes      int64
}

type WebhookConfig struct {
	Secret       string
	MaxBodyBytes int64
}

type StateConfig struct {
	Backend         string
	Path            string
	DynamoDBTable   string
	StateIndexName  string
	ExpiryIndexName string
}

type ReconciliationConfig struct {
	Interval         time.Duration
	OperationTimeout time.Duration
	MaxRecords       int
}

type ProviderConfig struct {
	Name ProviderName
	AWS  AWSConfig
	GCP  GCPConfig
}

type AWSConfig struct {
	Region                string
	LaunchTemplateID      string
	LaunchTemplateName    string
	LaunchTemplateVersion string
	SubnetID              string
	SecurityGroupIDs      []string
	InstanceType          string
	InstanceProfileARN    string
	InstanceProfileName   string
	UserData              string
	UserDataFile          string
}

type GCPConfig struct {
	Project          string
	Zone             string
	InstanceTemplate string
	NamePrefix       string
}

type CapacityConfig struct {
	Mode            CapacityMode
	PoolID          string
	Ownership       string
	Provider        string
	MaxRunners      int
	SecurityProfile string
}

type GitHubJITConfig struct {
	Enabled             bool
	InstallationToken   string
	RunnerGroupID       int64
	APIBaseURL          string
	RequestTimeout      time.Duration
	RegistrationTimeout time.Duration
	PollInterval        time.Duration
}

type AIConfig struct {
	Enabled         bool
	Provider        string
	Endpoint        string
	Model           string
	Region          string
	APIKey          string
	Timeout         time.Duration
	MaxLogBytes     int
	ConsentApproved bool
}

// ExtensionsConfig controls the optional, read-only extension runtime. An
// enabled runtime must name every tenant it may observe; there is no wildcard
// tenant grant.
type ExtensionsConfig struct {
	Enabled         bool
	TenantAllowlist []string
	QueueSize       int
	Timeout         time.Duration
	ReportBuffer    int
}

func (c ExtensionsConfig) Validate() error {
	var problems []string
	if c.QueueSize <= 0 || c.QueueSize > 4096 {
		problems = append(problems, "queue size must be between 1 and 4096")
	}
	if c.Timeout <= 0 || c.Timeout > 5*time.Minute {
		problems = append(problems, "timeout must be between 1ns and 5m")
	}
	if c.ReportBuffer <= 0 || c.ReportBuffer > 4096 {
		problems = append(problems, "report buffer must be between 1 and 4096")
	}
	seen := make(map[string]struct{}, len(c.TenantAllowlist))
	for _, tenant := range c.TenantAllowlist {
		tenant = strings.TrimSpace(tenant)
		if tenant == "" || tenant == "*" || len(tenant) > 128 {
			problems = append(problems, "tenant allowlist contains an invalid tenant")
			continue
		}
		if _, exists := seen[tenant]; exists {
			problems = append(problems, "tenant allowlist contains a duplicate tenant")
		}
		seen[tenant] = struct{}{}
	}
	if c.Enabled && len(seen) == 0 {
		problems = append(problems, "tenant allowlist is required when extensions are enabled")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: extensions %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

func Defaults() Config {
	return Config{
		Server:         ServerConfig{ListenAddr: ":8080", ReadHeaderTimeout: 5 * time.Second, MaxBodyBytes: 2 << 20},
		Webhook:        WebhookConfig{MaxBodyBytes: 2 << 20},
		State:          StateConfig{Backend: "memory", StateIndexName: "gsi1", ExpiryIndexName: "gsi2"},
		Reconciliation: ReconciliationConfig{Interval: 30 * time.Second, OperationTimeout: 30 * time.Second, MaxRecords: 1000},
		Provider:       ProviderConfig{Name: ProviderFake, AWS: AWSConfig{Region: "us-east-1"}, GCP: GCPConfig{Zone: "us-central1-a", NamePrefix: "leo-runner"}},
		Capacity:       CapacityConfig{Mode: CapacityFallback, PoolID: "configured", Ownership: "managed", Provider: "fake", MaxRunners: 100, SecurityProfile: "isolated"},
		GitHubJIT:      GitHubJITConfig{APIBaseURL: "https://api.github.com", RequestTimeout: 30 * time.Second, RegistrationTimeout: 10 * time.Minute, PollInterval: 5 * time.Second},
		AI:             AIConfig{Provider: "disabled", Timeout: 5 * time.Second, MaxLogBytes: 64 * 1024},
		Extensions:     ExtensionsConfig{QueueSize: 32, Timeout: 2 * time.Second, ReportBuffer: 64},
	}
}

// Load reads process environment variables and validates the complete result.
func Load() (Config, error) { return LoadFrom(environment()) }

// LoadFrom is equivalent to Load but accepts an explicit environment map,
// making startup validation deterministic and avoiding process-global tests.
func LoadFrom(env map[string]string) (Config, error) {
	c := Defaults()
	var err error
	get := func(name string) string { return strings.TrimSpace(env[name]) }
	if c.Server.ListenAddr, err = optionalString(env, "LISTEN_ADDR", c.Server.ListenAddr); err != nil {
		return c, err
	}
	if c.Server.ReadHeaderTimeout, err = optionalDuration(env, "READ_HEADER_TIMEOUT", c.Server.ReadHeaderTimeout); err != nil {
		return c, err
	}
	if c.Server.MaxBodyBytes, err = optionalInt64(env, "MAX_BODY_BYTES", c.Server.MaxBodyBytes); err != nil {
		return c, err
	}
	c.Webhook.Secret = get("GITHUB_WEBHOOK_SECRET")
	if c.Webhook.MaxBodyBytes, err = optionalInt64(env, "WEBHOOK_MAX_BODY_BYTES", c.Webhook.MaxBodyBytes); err != nil {
		return c, err
	}
	c.State.Path = get("STATE_PATH")
	c.State.DynamoDBTable = get("DYNAMODB_TABLE_NAME")
	c.State.StateIndexName = getOr(env, "DYNAMODB_STATE_INDEX", c.State.StateIndexName)
	c.State.ExpiryIndexName = getOr(env, "DYNAMODB_EXPIRY_INDEX", c.State.ExpiryIndexName)
	switch {
	case c.State.DynamoDBTable != "":
		c.State.Backend = "dynamodb"
	case c.State.Path != "":
		c.State.Backend = "file"
	}
	if c.Reconciliation.Interval, err = optionalDuration(env, "RECONCILE_INTERVAL", c.Reconciliation.Interval); err != nil {
		return c, err
	}
	if c.Reconciliation.OperationTimeout, err = optionalDuration(env, "RECONCILE_OPERATION_TIMEOUT", c.Reconciliation.OperationTimeout); err != nil {
		return c, err
	}
	if c.Reconciliation.MaxRecords, err = optionalInt(env, "RECONCILE_MAX_RECORDS", c.Reconciliation.MaxRecords); err != nil {
		return c, err
	}

	providerName := get("CAPACITY_PROVIDER")
	if providerName == "" {
		switch {
		case get("AWS_LAUNCH_TEMPLATE_ID") != "" || get("AWS_LAUNCH_TEMPLATE_NAME") != "":
			providerName = string(ProviderAWS)
		case get("GCP_INSTANCE_TEMPLATE") != "":
			providerName = string(ProviderGCP)
		default:
			providerName = string(ProviderFake)
		}
	}
	c.Provider.Name = ProviderName(providerName)
	c.Capacity.Mode = CapacityMode(getOr(env, "CAPACITY_MODE", string(c.Capacity.Mode)))
	c.Capacity.PoolID = getOr(env, "CAPACITY_POOL_ID", c.Capacity.PoolID)
	c.Capacity.Ownership = getOr(env, "CAPACITY_OWNERSHIP", c.Capacity.Ownership)
	c.Capacity.Provider = getOr(env, "CAPACITY_POOL_PROVIDER", getOr(env, "CAPACITY_PROVIDER", string(c.Provider.Name)))
	c.Capacity.SecurityProfile = getOr(env, "CAPACITY_SECURITY_PROFILE", c.Capacity.SecurityProfile)
	if c.Capacity.MaxRunners, err = optionalInt(env, "CAPACITY_MAX_RUNNERS", c.Capacity.MaxRunners); err != nil {
		return c, err
	}

	c.Provider.AWS.Region = getOr(env, "AWS_REGION", c.Provider.AWS.Region)
	c.Provider.AWS.LaunchTemplateID = get("AWS_LAUNCH_TEMPLATE_ID")
	c.Provider.AWS.LaunchTemplateName = get("AWS_LAUNCH_TEMPLATE_NAME")
	c.Provider.AWS.LaunchTemplateVersion = get("AWS_LAUNCH_TEMPLATE_VERSION")
	c.Provider.AWS.SubnetID = get("AWS_SUBNET_ID")
	c.Provider.AWS.SecurityGroupIDs = csv(get("AWS_SECURITY_GROUP_IDS"))
	c.Provider.AWS.InstanceType = get("AWS_INSTANCE_TYPE")
	c.Provider.AWS.InstanceProfileARN = get("AWS_RUNNER_INSTANCE_PROFILE_ARN")
	c.Provider.AWS.InstanceProfileName = get("AWS_RUNNER_INSTANCE_PROFILE_NAME")
	c.Provider.AWS.UserData = env["RUNNER_USER_DATA"]
	c.Provider.AWS.UserDataFile = get("RUNNER_USER_DATA_FILE")

	c.Provider.GCP.Project = get("GCP_PROJECT")
	c.Provider.GCP.Zone = getOr(env, "GCP_ZONE", c.Provider.GCP.Zone)
	c.Provider.GCP.InstanceTemplate = get("GCP_INSTANCE_TEMPLATE")
	c.Provider.GCP.NamePrefix = getOr(env, "GCP_NAME_PREFIX", c.Provider.GCP.NamePrefix)

	jitEnabled, present, parseErr := optionalBool(env, "GITHUB_JIT_ENABLED")
	if parseErr != nil {
		return c, parseErr
	}
	c.GitHubJIT.Enabled = present && jitEnabled
	c.GitHubJIT.InstallationToken = get("GITHUB_APP_INSTALLATION_TOKEN")
	if !present && c.GitHubJIT.InstallationToken != "" {
		c.GitHubJIT.Enabled = true
	}
	if c.GitHubJIT.RunnerGroupID, err = optionalInt64(env, "GITHUB_RUNNER_GROUP_ID", 0); err != nil {
		return c, err
	}
	c.GitHubJIT.APIBaseURL = getOr(env, "GITHUB_API_BASE_URL", c.GitHubJIT.APIBaseURL)
	if c.GitHubJIT.RequestTimeout, err = optionalDuration(env, "GITHUB_JIT_REQUEST_TIMEOUT", c.GitHubJIT.RequestTimeout); err != nil {
		return c, err
	}
	if c.GitHubJIT.RegistrationTimeout, err = optionalDuration(env, "GITHUB_REGISTRATION_TIMEOUT", c.GitHubJIT.RegistrationTimeout); err != nil {
		return c, err
	}
	if c.GitHubJIT.PollInterval, err = optionalDuration(env, "GITHUB_REGISTRATION_POLL_INTERVAL", c.GitHubJIT.PollInterval); err != nil {
		return c, err
	}

	if c.AI.Enabled, err = optionalBoolDefault(env, "AI_ENABLED", false); err != nil {
		return c, err
	}
	c.AI.Provider = getOr(env, "AI_PROVIDER", c.AI.Provider)
	c.AI.Endpoint = get("AI_ENDPOINT")
	c.AI.Model = get("AI_MODEL")
	c.AI.Region = get("AI_REGION")
	c.AI.APIKey = get("AI_API_KEY")
	if c.AI.Timeout, err = optionalDuration(env, "AI_TIMEOUT", c.AI.Timeout); err != nil {
		return c, err
	}
	if c.AI.MaxLogBytes, err = optionalInt(env, "AI_MAX_LOG_BYTES", c.AI.MaxLogBytes); err != nil {
		return c, err
	}
	c.AI.ConsentApproved, err = optionalBoolDefault(env, "AI_CONSENT_APPROVED", false)
	if err != nil {
		return c, err
	}
	if c.Extensions.Enabled, err = optionalBoolDefault(env, "EXTENSIONS_ENABLED", false); err != nil {
		return c, err
	}
	c.Extensions.TenantAllowlist = csv(env["EXTENSIONS_TENANT_ALLOWLIST"])
	if c.Extensions.QueueSize, err = optionalInt(env, "EXTENSIONS_QUEUE_SIZE", c.Extensions.QueueSize); err != nil {
		return c, err
	}
	if c.Extensions.Timeout, err = optionalDuration(env, "EXTENSIONS_TIMEOUT", c.Extensions.Timeout); err != nil {
		return c, err
	}
	if c.Extensions.ReportBuffer, err = optionalInt(env, "EXTENSIONS_REPORT_BUFFER", c.Extensions.ReportBuffer); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	var problems []string
	positive := func(name string, value int64) {
		if value <= 0 {
			problems = append(problems, name+" must be positive")
		}
	}
	positive("server max body bytes", c.Server.MaxBodyBytes)
	positive("webhook max body bytes", c.Webhook.MaxBodyBytes)
	if strings.TrimSpace(c.Server.ListenAddr) == "" {
		problems = append(problems, "server listen address is required")
	}
	if c.Server.ReadHeaderTimeout <= 0 {
		problems = append(problems, "server read header timeout must be positive")
	}
	if strings.TrimSpace(c.Webhook.Secret) == "" {
		problems = append(problems, "GITHUB_WEBHOOK_SECRET is required")
	}
	if c.Reconciliation.Interval <= 0 || c.Reconciliation.OperationTimeout <= 0 {
		problems = append(problems, "reconciliation durations must be positive")
	}
	if c.Reconciliation.MaxRecords <= 0 {
		problems = append(problems, "reconciliation max records must be positive")
	}
	if c.State.Backend != "memory" && c.State.Backend != "file" && c.State.Backend != "dynamodb" {
		problems = append(problems, "state backend must be memory, file, or dynamodb")
	}
	if c.State.Backend == "dynamodb" && c.State.DynamoDBTable == "" {
		problems = append(problems, "DYNAMODB_TABLE_NAME is required for dynamodb state backend")
	}

	switch c.Provider.Name {
	case ProviderFake:
	case ProviderAWS:
		if strings.TrimSpace(c.Provider.AWS.Region) == "" {
			problems = append(problems, "AWS_REGION is required for aws provider")
		}
		if (c.Provider.AWS.LaunchTemplateID == "") == (c.Provider.AWS.LaunchTemplateName == "") {
			problems = append(problems, "exactly one AWS launch template ID or name is required")
		}
		if c.Provider.AWS.InstanceProfileARN != "" && c.Provider.AWS.InstanceProfileName != "" {
			problems = append(problems, "AWS instance profile ARN and name are mutually exclusive")
		}
		if c.Provider.AWS.UserData != "" && c.Provider.AWS.UserDataFile != "" {
			problems = append(problems, "RUNNER_USER_DATA and RUNNER_USER_DATA_FILE are mutually exclusive")
		}
	case ProviderGCP:
		if c.Provider.GCP.Project == "" {
			problems = append(problems, "GCP_PROJECT is required for gcp provider")
		}
		if c.Provider.GCP.Zone == "" {
			problems = append(problems, "GCP_ZONE is required for gcp provider")
		}
		if c.Provider.GCP.InstanceTemplate == "" {
			problems = append(problems, "GCP_INSTANCE_TEMPLATE is required for gcp provider")
		}
	default:
		problems = append(problems, "provider must be fake, aws, or gcp")
	}
	if c.Capacity.Mode != CapacityCustomerFirst && c.Capacity.Mode != CapacityManagedFirst && c.Capacity.Mode != CapacityCustomerOnly && c.Capacity.Mode != CapacityManagedOnly && c.Capacity.Mode != CapacityFallback {
		problems = append(problems, "capacity mode is invalid")
	}
	if c.Capacity.PoolID == "" || c.Capacity.Ownership == "" || c.Capacity.Provider == "" || c.Capacity.SecurityProfile == "" || c.Capacity.MaxRunners <= 0 {
		problems = append(problems, "capacity settings are incomplete or invalid")
	}
	if c.Capacity.Provider != string(c.Provider.Name) {
		problems = append(problems, "capacity provider must match selected provider")
	}

	if c.GitHubJIT.Enabled {
		if c.GitHubJIT.InstallationToken == "" {
			problems = append(problems, "GITHUB_APP_INSTALLATION_TOKEN is required when JIT is enabled")
		}
		if c.GitHubJIT.RunnerGroupID <= 0 {
			problems = append(problems, "GITHUB_RUNNER_GROUP_ID must be positive when JIT is enabled")
		}
	} else if c.GitHubJIT.InstallationToken != "" || c.GitHubJIT.RunnerGroupID != 0 {
		problems = append(problems, "JIT credentials and runner group must be unset when JIT is disabled")
	}
	if !httpsURL(c.GitHubJIT.APIBaseURL) {
		problems = append(problems, "GitHub API base URL must be an absolute HTTP(S) URL")
	}
	if c.GitHubJIT.RequestTimeout <= 0 || c.GitHubJIT.RegistrationTimeout <= 0 || c.GitHubJIT.PollInterval <= 0 {
		problems = append(problems, "GitHub JIT durations must be positive")
	}

	if !c.AI.Enabled {
		if c.AI.Provider != "disabled" {
			problems = append(problems, "AI provider must be disabled when AI is disabled")
		}
	} else {
		if c.AI.Provider != "local" && c.AI.Provider != "hosted" {
			problems = append(problems, "enabled AI provider must be local or hosted")
		}
		if c.AI.Endpoint == "" || !httpsURL(c.AI.Endpoint) {
			problems = append(problems, "enabled AI requires an absolute HTTP(S) endpoint")
		}
		if c.AI.Model == "" || !c.AI.ConsentApproved {
			problems = append(problems, "enabled AI requires a model and explicit consent")
		}
		if c.AI.Provider == "hosted" && c.AI.Region == "" {
			problems = append(problems, "hosted AI requires a region")
		}
	}
	if c.AI.Timeout <= 0 || c.AI.MaxLogBytes <= 0 {
		problems = append(problems, "AI limits must be positive")
	}
	if err := c.Extensions.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

// Diagnostics returns only safe, non-secret values suitable for startup logs.
func (c Config) Diagnostics() map[string]string {
	return map[string]string{
		"listen_addr": c.Server.ListenAddr, "state_path_set": strconv.FormatBool(c.State.Path != ""),
		"provider": string(c.Provider.Name), "capacity_mode": string(c.Capacity.Mode), "capacity_pool_id": c.Capacity.PoolID,
		"jit_enabled": strconv.FormatBool(c.GitHubJIT.Enabled), "jit_token_set": strconv.FormatBool(c.GitHubJIT.InstallationToken != ""),
		"webhook_secret_set": strconv.FormatBool(c.Webhook.Secret != ""), "ai_enabled": strconv.FormatBool(c.AI.Enabled),
		"ai_provider": c.AI.Provider, "ai_api_key_set": strconv.FormatBool(c.AI.APIKey != ""),
		"extensions_enabled": strconv.FormatBool(c.Extensions.Enabled), "extension_tenants": strconv.Itoa(len(c.Extensions.TenantAllowlist)),
	}
}

func (c Config) String() string { return fmt.Sprintf("controller config: %v", c.Diagnostics()) }

func environment() map[string]string {
	out := make(map[string]string)
	for _, item := range os.Environ() {
		if key, value, ok := strings.Cut(item, "="); ok {
			out[key] = value
		}
	}
	return out
}
func getOr(env map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(env[key]); value != "" {
		return value
	}
	return fallback
}
func csv(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
func optionalString(env map[string]string, key, fallback string) (string, error) {
	if _, ok := env[key]; !ok {
		return fallback, nil
	}
	value := strings.TrimSpace(env[key])
	if value == "" {
		return "", fmt.Errorf("%w: %s cannot be empty", ErrInvalid, key)
	}
	return value, nil
}
func optionalDuration(env map[string]string, key string, fallback time.Duration) (time.Duration, error) {
	if _, ok := env[key]; !ok {
		return fallback, nil
	}
	value, err := time.ParseDuration(strings.TrimSpace(env[key]))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%w: %s must be a positive duration", ErrInvalid, key)
	}
	return value, nil
}
func optionalInt(env map[string]string, key string, fallback int) (int, error) {
	value, err := optionalInt64(env, key, int64(fallback))
	return int(value), err
}
func optionalInt64(env map[string]string, key string, fallback int64) (int64, error) {
	if _, ok := env[key]; !ok {
		return fallback, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(env[key]), 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", ErrInvalid, key)
	}
	return value, nil
}
func optionalBoolDefault(env map[string]string, key string, fallback bool) (bool, error) {
	value, _, err := optionalBool(env, key)
	if err != nil {
		return false, err
	}
	if _, ok := env[key]; !ok {
		return fallback, nil
	}
	return value, nil
}
func optionalBool(env map[string]string, key string) (bool, bool, error) {
	raw, ok := env[key]
	if !ok {
		return false, false, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, true, fmt.Errorf("%w: %s must be true or false", ErrInvalid, key)
	}
	return value, true, nil
}
func httpsURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil
}
