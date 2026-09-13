package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func validEnv() map[string]string {
	return map[string]string{"GITHUB_WEBHOOK_SECRET": "webhook-secret"}
}

func TestLoadFromUsesSafeDefaults(t *testing.T) {
	c, err := LoadFrom(validEnv())
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if c.Server.ListenAddr != ":8080" || c.Provider.Name != ProviderFake || c.Capacity.Mode != CapacityFallback {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.Reconciliation.Interval != 30*time.Second || c.AI.Enabled {
		t.Fatalf("unexpected timing or AI defaults: %+v", c)
	}
	if c.Extensions.Enabled || c.Extensions.QueueSize != 32 || c.Extensions.Timeout != 2*time.Second || c.Extensions.ReportBuffer != 64 {
		t.Fatalf("unexpected extension defaults: %+v", c.Extensions)
	}
}

func TestLoadFromParsesExplicitExtensionPolicy(t *testing.T) {
	env := validEnv()
	env["EXTENSIONS_ENABLED"] = "true"
	env["EXTENSIONS_TENANT_ALLOWLIST"] = "tenant-a, tenant-b"
	env["EXTENSIONS_QUEUE_SIZE"] = "128"
	env["EXTENSIONS_TIMEOUT"] = "1500ms"
	env["EXTENSIONS_REPORT_BUFFER"] = "256"
	c, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if !c.Extensions.Enabled || len(c.Extensions.TenantAllowlist) != 2 || c.Extensions.QueueSize != 128 || c.Extensions.Timeout != 1500*time.Millisecond || c.Extensions.ReportBuffer != 256 {
		t.Fatalf("extension config = %+v", c.Extensions)
	}
}

func TestValidateRejectsUnsafeExtensionTenantPolicyAndBounds(t *testing.T) {
	tests := []ExtensionsConfig{
		{Enabled: true, QueueSize: 1, Timeout: time.Second, ReportBuffer: 1},
		{Enabled: true, TenantAllowlist: []string{"*"}, QueueSize: 1, Timeout: time.Second, ReportBuffer: 1},
		{TenantAllowlist: []string{"tenant-a", "tenant-a"}, QueueSize: 1, Timeout: time.Second, ReportBuffer: 1},
		{QueueSize: 4097, Timeout: time.Second, ReportBuffer: 1},
		{QueueSize: 1, Timeout: 6 * time.Minute, ReportBuffer: 1},
		{QueueSize: 1, Timeout: time.Second, ReportBuffer: 4097},
	}
	for i, extensionConfig := range tests {
		c := Defaults()
		c.Webhook.Secret = "secret"
		c.Extensions = extensionConfig
		if err := c.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d error = %v, want ErrInvalid", i, err)
		}
	}
}

func TestLoadFromRejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"boolean", "AI_ENABLED", "yes"},
		{"duration", "RECONCILE_INTERVAL", "soon"},
		{"integer", "CAPACITY_MAX_RUNNERS", "many"},
		{"negative integer", "MAX_BODY_BYTES", "-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := validEnv()
			env[test.key] = test.value
			_, err := LoadFrom(env)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateProviderRequirements(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"missing webhook secret", map[string]string{}},
		{"AWS missing launch template", map[string]string{"GITHUB_WEBHOOK_SECRET": "x", "CAPACITY_PROVIDER": "aws"}},
		{"AWS both launch template forms", map[string]string{"GITHUB_WEBHOOK_SECRET": "x", "CAPACITY_PROVIDER": "aws", "AWS_LAUNCH_TEMPLATE_ID": "lt-1", "AWS_LAUNCH_TEMPLATE_NAME": "template"}},
		{"AWS both instance profile forms", map[string]string{"GITHUB_WEBHOOK_SECRET": "x", "CAPACITY_PROVIDER": "aws", "AWS_LAUNCH_TEMPLATE_ID": "lt-1", "AWS_RUNNER_INSTANCE_PROFILE_ARN": "arn", "AWS_RUNNER_INSTANCE_PROFILE_NAME": "name"}},
		{"GCP missing project", map[string]string{"GITHUB_WEBHOOK_SECRET": "x", "CAPACITY_PROVIDER": "gcp", "GCP_INSTANCE_TEMPLATE": "template"}},
		{"unknown provider", map[string]string{"GITHUB_WEBHOOK_SECRET": "x", "CAPACITY_PROVIDER": "azure"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadFrom(test.env)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
	c := Defaults()
	c.Webhook.Secret = "x"
	c.Provider.Name = ProviderAWS
	c.Provider.AWS.LaunchTemplateID = "lt-1"
	c.Capacity.Provider = "gcp"
	if err := c.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatched capacity provider error = %v, want ErrInvalid", err)
	}
}

func TestLoadFromInfersCloudProviderFromTemplate(t *testing.T) {
	awsEnv := validEnv()
	awsEnv["AWS_LAUNCH_TEMPLATE_ID"] = "lt-1"
	awsConfig, err := LoadFrom(awsEnv)
	if err != nil || awsConfig.Provider.Name != ProviderAWS || awsConfig.Capacity.Provider != string(ProviderAWS) {
		t.Fatalf("AWS inference = provider %q, capacity provider %q, error = %v", awsConfig.Provider.Name, awsConfig.Capacity.Provider, err)
	}

	gcpEnv := validEnv()
	gcpEnv["GCP_PROJECT"] = "project"
	gcpEnv["GCP_INSTANCE_TEMPLATE"] = "template"
	gcpConfig, err := LoadFrom(gcpEnv)
	if err != nil || gcpConfig.Provider.Name != ProviderGCP || gcpConfig.Capacity.Provider != string(ProviderGCP) {
		t.Fatalf("GCP inference = provider %q, capacity provider %q, error = %v", gcpConfig.Provider.Name, gcpConfig.Capacity.Provider, err)
	}
}

func TestLoadFromJITRequirementsAndExplicitDisable(t *testing.T) {
	base := validEnv()
	base["GITHUB_JIT_ENABLED"] = "true"
	if _, err := LoadFrom(base); err == nil {
		t.Fatal("enabled JIT without token/group unexpectedly validated")
	}
	base["GITHUB_APP_INSTALLATION_TOKEN"] = "installation-token"
	base["GITHUB_RUNNER_GROUP_ID"] = "42"
	c, err := LoadFrom(base)
	if err != nil || !c.GitHubJIT.Enabled || c.GitHubJIT.RunnerGroupID != 42 {
		t.Fatalf("enabled JIT = %+v, error = %v", c.GitHubJIT, err)
	}

	disabled := validEnv()
	disabled["GITHUB_JIT_ENABLED"] = "false"
	disabled["GITHUB_APP_INSTALLATION_TOKEN"] = "stale-token"
	if _, err := LoadFrom(disabled); err == nil {
		t.Fatal("disabled JIT with stale token unexpectedly validated")
	}
}

func TestValidateRejectsJITAndAIUnsafeCombinations(t *testing.T) {
	tests := []map[string]string{
		{"GITHUB_WEBHOOK_SECRET": "x", "CAPACITY_MODE": "invalid"},
		{"GITHUB_WEBHOOK_SECRET": "x", "GITHUB_API_BASE_URL": "file:///token"},
		{"GITHUB_WEBHOOK_SECRET": "x", "AI_PROVIDER": "hosted"},
		{"GITHUB_WEBHOOK_SECRET": "x", "AI_ENABLED": "true", "AI_PROVIDER": "hosted", "AI_ENDPOINT": "https://ai.example", "AI_MODEL": "model", "AI_REGION": "eu", "AI_CONSENT_APPROVED": "false"},
		{"GITHUB_WEBHOOK_SECRET": "x", "RUNNER_USER_DATA": "a", "RUNNER_USER_DATA_FILE": "/tmp/b", "CAPACITY_PROVIDER": "aws", "AWS_LAUNCH_TEMPLATE_ID": "lt-1"},
	}
	for index, env := range tests {
		if _, err := LoadFrom(env); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d error = %v, want ErrInvalid", index, err)
		}
	}
}

func TestDiagnosticsAndStringRedactSecrets(t *testing.T) {
	env := validEnv()
	env["GITHUB_JIT_ENABLED"] = "true"
	env["GITHUB_APP_INSTALLATION_TOKEN"] = "github-installation-secret"
	env["GITHUB_RUNNER_GROUP_ID"] = "7"
	env["AI_ENABLED"] = "true"
	env["AI_PROVIDER"] = "local"
	env["AI_ENDPOINT"] = "http://127.0.0.1:11434/analyze"
	env["AI_MODEL"] = "local-model"
	env["AI_CONSENT_APPROVED"] = "true"
	env["AI_API_KEY"] = "ai-api-secret"
	c, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	text := c.String()
	for _, secret := range []string{"webhook-secret", "github-installation-secret", "ai-api-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("String() leaked %q: %s", secret, text)
		}
	}
	diagnostics := c.Diagnostics()
	if diagnostics["webhook_secret_set"] != "true" || diagnostics["jit_token_set"] != "true" || diagnostics["ai_api_key_set"] != "true" {
		t.Fatalf("secret presence diagnostics missing: %#v", diagnostics)
	}
}

func TestSecretContractAndSafeSnapshotNeverContainValues(t *testing.T) {
	env := validEnv()
	env["GITHUB_JIT_ENABLED"] = "true"
	env["GITHUB_APP_INSTALLATION_TOKEN"] = "jit-secret"
	env["GITHUB_RUNNER_GROUP_ID"] = "7"
	c, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	requirements := c.SecretContract()
	if len(requirements) != 3 || !requirements[0].Required || !requirements[1].Required || requirements[2].Required {
		t.Fatalf("secret contract = %+v", requirements)
	}
	snapshot := c.SafeSnapshot()
	if !snapshot.SecretPresence["GITHUB_WEBHOOK_SECRET"] || !snapshot.SecretPresence["GITHUB_APP_INSTALLATION_TOKEN"] {
		t.Fatalf("secret presence = %#v", snapshot.SecretPresence)
	}
	if strings.Contains(strings.ToLower(fmt.Sprintf("%+v", snapshot)), "secret") && strings.Contains(fmt.Sprintf("%+v", snapshot), "jit-secret") {
		t.Fatalf("safe snapshot leaked secret: %+v", snapshot)
	}
}

func TestValidateRejectsHostedAIWithoutCredentialAndStaleDisabledCredential(t *testing.T) {
	hosted := validEnv()
	hosted["AI_ENABLED"] = "true"
	hosted["AI_PROVIDER"] = "hosted"
	hosted["AI_ENDPOINT"] = "https://ai.example/analyze"
	hosted["AI_MODEL"] = "model"
	hosted["AI_REGION"] = "us-east-1"
	hosted["AI_CONSENT_APPROVED"] = "true"
	if _, err := LoadFrom(hosted); !errors.Is(err, ErrInvalid) {
		t.Fatal("hosted AI without API key unexpectedly validated")
	}
	disabled := validEnv()
	disabled["AI_API_KEY"] = "stale-key"
	if _, err := LoadFrom(disabled); !errors.Is(err, ErrInvalid) {
		t.Fatal("disabled AI with stale API key unexpectedly validated")
	}
}

func TestConfigValidateDoesNotMutateSlicesOrSecrets(t *testing.T) {
	c := Defaults()
	c.Webhook.Secret = "secret"
	c.Provider.Name = ProviderAWS
	c.Provider.AWS.LaunchTemplateID = "lt-1"
	c.Capacity.Provider = "aws"
	c.Provider.AWS.SecurityGroupIDs = []string{"sg-1"}
	before := append([]string(nil), c.Provider.AWS.SecurityGroupIDs...)
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(c.Provider.AWS.SecurityGroupIDs) != 1 || c.Provider.AWS.SecurityGroupIDs[0] != before[0] {
		t.Fatalf("Validate mutated security groups: %#v", c.Provider.AWS.SecurityGroupIDs)
	}
}
