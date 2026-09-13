package config

import (
	"errors"
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
