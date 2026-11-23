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
	required := map[string]bool{}
	for _, requirement := range requirements {
		required[requirement.EnvVar] = requirement.Required
	}
	if len(requirements) != 4 || !required["GITHUB_WEBHOOK_SECRET"] || !required["GITHUB_APP_INSTALLATION_TOKEN"] || required["GITHUB_APP_PRIVATE_KEY"] || required["AI_API_KEY"] {
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

func appEnv() map[string]string {
	env := validEnv()
	env["GITHUB_JIT_ENABLED"] = "true"
	env["GITHUB_RUNNER_GROUP_ID"] = "42"
	env["GITHUB_APP_ID"] = "12345"
	env["GITHUB_APP_INSTALLATION_ID"] = "678"
	env["GITHUB_APP_PRIVATE_KEY"] = "pem-fixture-not-a-real-key"
	return env
}

func TestLoadFromAcceptsGitHubAppSettings(t *testing.T) {
	env := appEnv()
	env["GITHUB_APP_TOKEN_PERMISSIONS"] = "administration=write, actions=read"
	c, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	jit := c.GitHubJIT
	if !jit.Enabled || !jit.AppConfigured() || jit.AppID != "12345" || jit.AppInstallationID != 678 || jit.AppPrivateKey == "" || jit.InstallationToken != "" {
		t.Fatalf("GitHub App config = %+v", jit)
	}
	if jit.AppTokenPermissions["administration"] != "write" || jit.AppTokenPermissions["actions"] != "read" {
		t.Fatalf("permissions = %v", jit.AppTokenPermissions)
	}
}

func TestLoadFromEnablesJITImplicitlyFromAppSettings(t *testing.T) {
	env := appEnv()
	delete(env, "GITHUB_JIT_ENABLED")
	c, err := LoadFrom(env)
	if err != nil || !c.GitHubJIT.Enabled {
		t.Fatalf("implicit enable = %+v, %v", c.GitHubJIT, err)
	}
}

func TestLoadFromRejectsUnsafeGitHubAppCombinations(t *testing.T) {
	mutate := func(f func(map[string]string)) map[string]string {
		env := appEnv()
		f(env)
		return env
	}
	tests := map[string]map[string]string{
		"token and app together":   mutate(func(e map[string]string) { e["GITHUB_APP_INSTALLATION_TOKEN"] = "static-token" }),
		"missing app id":           mutate(func(e map[string]string) { delete(e, "GITHUB_APP_ID") }),
		"missing installation id":  mutate(func(e map[string]string) { delete(e, "GITHUB_APP_INSTALLATION_ID") }),
		"missing private key":      mutate(func(e map[string]string) { delete(e, "GITHUB_APP_PRIVATE_KEY") }),
		"non numeric installation": mutate(func(e map[string]string) { e["GITHUB_APP_INSTALLATION_ID"] = "abc" }),
		"zero installation":        mutate(func(e map[string]string) { e["GITHUB_APP_INSTALLATION_ID"] = "0" }),
		"no runner group":          mutate(func(e map[string]string) { delete(e, "GITHUB_RUNNER_GROUP_ID") }),
		"plain http api":           mutate(func(e map[string]string) { e["GITHUB_API_BASE_URL"] = "http://github.example.com" }),
		"jit disabled with app":    mutate(func(e map[string]string) { e["GITHUB_JIT_ENABLED"] = "false"; delete(e, "GITHUB_RUNNER_GROUP_ID") }),
		"bad permission level":     mutate(func(e map[string]string) { e["GITHUB_APP_TOKEN_PERMISSIONS"] = "administration=owner" }),
		"bad permission name":      mutate(func(e map[string]string) { e["GITHUB_APP_TOKEN_PERMISSIONS"] = "Admin=read" }),
		"duplicate permission":     mutate(func(e map[string]string) { e["GITHUB_APP_TOKEN_PERMISSIONS"] = "actions=read,actions=write" }),
		"permissions without app":  {"GITHUB_WEBHOOK_SECRET": "x", "GITHUB_APP_TOKEN_PERMISSIONS": "actions=read"},
	}
	for name, env := range tests {
		if _, err := LoadFrom(env); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
}

func TestGitHubAppSecretContractAndDiagnosticsNeverContainValues(t *testing.T) {
	c, err := LoadFrom(appEnv())
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{}
	for _, requirement := range c.SecretContract() {
		required[requirement.EnvVar] = requirement.Required
	}
	if !required["GITHUB_APP_PRIVATE_KEY"] || required["GITHUB_APP_INSTALLATION_TOKEN"] {
		t.Fatalf("app mode must require the private key and not the static token: %v", required)
	}
	rendered := fmt.Sprintf("%v %+v %s", c.Diagnostics(), c.SafeSnapshot(), c)
	for _, forbidden := range []string{"pem-fixture", "not-a-real-key"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("diagnostics leak %q: %s", forbidden, rendered)
		}
	}
	if !c.SafeSnapshot().SecretPresence["GITHUB_APP_PRIVATE_KEY"] {
		t.Fatalf("presence flag missing: %#v", c.SafeSnapshot().SecretPresence)
	}
}

func TestLoadFromStaticTokenModeIsUnchanged(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_INSTALLATION_TOKEN"] = "static-token"
	env["GITHUB_RUNNER_GROUP_ID"] = "9"
	c, err := LoadFrom(env)
	if err != nil || !c.GitHubJIT.Enabled || c.GitHubJIT.AppConfigured() {
		t.Fatalf("static token mode = %+v, %v", c.GitHubJIT, err)
	}
}

// The shipped Kubernetes ConfigMap lists the GitHub App keys with empty
// values so operators can find them. Empty must mean "not configured".
func TestLoadFromTreatsEmptyGitHubAppSettingsAsUnset(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_ID"] = ""
	env["GITHUB_APP_INSTALLATION_ID"] = ""
	env["GITHUB_APP_PRIVATE_KEY"] = ""
	env["GITHUB_APP_TOKEN_PERMISSIONS"] = ""
	env["GITHUB_JIT_ENABLED"] = "false"
	c, err := LoadFrom(env)
	if err != nil || c.GitHubJIT.AppConfigured() || c.GitHubJIT.Enabled {
		t.Fatalf("empty App settings = %+v, %v", c.GitHubJIT, err)
	}
}

func TestLoadFromWebhookProcessingDefaultsAndOverrides(t *testing.T) {
	c, err := LoadFrom(validEnv())
	if err != nil || c.Webhook.Workers != 16 || c.Webhook.QueueSize != 1024 || c.Webhook.EventTimeout != 30*time.Minute {
		t.Fatalf("defaults = %+v, %v", c.Webhook, err)
	}
	env := validEnv()
	env["WEBHOOK_WORKERS"] = "4"
	env["WEBHOOK_QUEUE_SIZE"] = "64"
	env["WEBHOOK_EVENT_TIMEOUT"] = "10m"
	c, err = LoadFrom(env)
	if err != nil || c.Webhook.Workers != 4 || c.Webhook.QueueSize != 64 || c.Webhook.EventTimeout != 10*time.Minute {
		t.Fatalf("overrides = %+v, %v", c.Webhook, err)
	}
}

func TestLoadFromRejectsUnsafeWebhookProcessingSettings(t *testing.T) {
	bad := []map[string]string{
		{"WEBHOOK_WORKERS": "0"},
		{"WEBHOOK_WORKERS": "257"},
		{"WEBHOOK_WORKERS": "8", "WEBHOOK_QUEUE_SIZE": "4"},
		{"WEBHOOK_QUEUE_SIZE": "100001"},
		{"WEBHOOK_EVENT_TIMEOUT": "25h"},
		{"WEBHOOK_EVENT_TIMEOUT": "soon"},
		{"WEBHOOK_WORKERS": "many"},
	}
	for _, extra := range bad {
		env := validEnv()
		for k, v := range extra {
			env[k] = v
		}
		if _, err := LoadFrom(env); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: error = %v, want ErrInvalid", extra, err)
		}
	}
}

func TestRunnerScopeDefaultsToRepositoryAndAcceptsOrganization(t *testing.T) {
	c, err := LoadFrom(validEnv())
	if err != nil || c.GitHubJIT.RunnerScope != RunnerScopeRepository {
		t.Fatalf("default scope = %q, error = %v", c.GitHubJIT.RunnerScope, err)
	}
	env := appEnv()
	env["GITHUB_RUNNER_SCOPE"] = "Organization"
	c, err = LoadFrom(env)
	if err != nil || c.GitHubJIT.RunnerScope != RunnerScopeOrganization {
		t.Fatalf("scope = %q, error = %v", c.GitHubJIT.RunnerScope, err)
	}
}

func TestRunnerScopeRejectsUnknownValuesAndStaleSettings(t *testing.T) {
	env := appEnv()
	env["GITHUB_RUNNER_SCOPE"] = "enterprise"
	if _, err := LoadFrom(env); err == nil {
		t.Fatal("an unknown runner scope must be rejected")
	}
	disabled := validEnv()
	disabled["GITHUB_RUNNER_SCOPE"] = "organization"
	if _, err := LoadFrom(disabled); err == nil {
		t.Fatal("an organization scope with JIT disabled must be rejected, like other stale JIT settings")
	}
}

func TestRunnerLifetimeDefaultsAndIsBounded(t *testing.T) {
	c, err := LoadFrom(validEnv())
	if err != nil || c.Provider.RunnerMaxLifetime != 2*time.Hour {
		t.Fatalf("default lifetime = %s, error = %v", c.Provider.RunnerMaxLifetime, err)
	}
	env := validEnv()
	env["RUNNER_MAX_LIFETIME"] = "90m"
	if c, err = LoadFrom(env); err != nil || c.Provider.RunnerMaxLifetime != 90*time.Minute {
		t.Fatalf("lifetime = %s, error = %v", c.Provider.RunnerMaxLifetime, err)
	}
	for _, bad := range []string{"1m", "25h", "0s", "-5m", "soon"} {
		env["RUNNER_MAX_LIFETIME"] = bad
		if _, err := LoadFrom(env); err == nil {
			t.Errorf("RUNNER_MAX_LIFETIME=%q must be rejected", bad)
		}
	}
}

func TestRunnerTagsParseAndRejectReservedOrMalformed(t *testing.T) {
	env := validEnv()
	env["AWS_RUNNER_TAGS"] = "RequestId=req-1, Environment=test"
	c, err := LoadFrom(env)
	if err != nil || len(c.Provider.AWS.Tags) != 2 || c.Provider.AWS.Tags["RequestId"] != "req-1" || c.Provider.AWS.Tags["Environment"] != "test" {
		t.Fatalf("tags = %v, error = %v", c.Provider.AWS.Tags, err)
	}
	for _, bad := range []string{"leo-runners:managed-by=x", "LEO-RUNNERS:x=y", "aws:cloudformation:stack=x", "novalue", "k=", "=v", "a=1,a=2"} {
		env["AWS_RUNNER_TAGS"] = bad
		if _, err := LoadFrom(env); err == nil {
			t.Errorf("AWS_RUNNER_TAGS=%q must be rejected", bad)
		}
	}
	if c, err = LoadFrom(validEnv()); err != nil || c.Provider.AWS.Tags != nil {
		t.Fatalf("no tags configured: tags = %v, error = %v", c.Provider.AWS.Tags, err)
	}
}
