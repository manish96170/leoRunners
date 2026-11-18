package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	providers "github.com/leo-runners/ci-platform/controller/internal/providers"
)

type mockEC2 struct {
	runInput        *ec2.RunInstancesInput
	runInputs       []*ec2.RunInstancesInput
	describeInputs  []*ec2.DescribeInstancesInput
	terminateInput  *ec2.TerminateInstancesInput
	terminateInputs []*ec2.TerminateInstancesInput
	runOutput       *ec2.RunInstancesOutput
	describeOutputs []*ec2.DescribeInstancesOutput
	describeDefault *ec2.DescribeInstancesOutput
	runErr          error
	describeErr     error
	terminateErr    error
}

func (m *mockEC2) RunInstances(_ context.Context, input *ec2.RunInstancesInput, _ ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	m.runInput = input
	m.runInputs = append(m.runInputs, input)
	if m.runErr != nil {
		return nil, m.runErr
	}
	if m.runOutput != nil {
		return m.runOutput, nil
	}
	return &ec2.RunInstancesOutput{Instances: []types.Instance{{InstanceId: awsv2.String("i-123")}}}, nil
}

func (m *mockEC2) DescribeInstances(_ context.Context, input *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.describeInputs = append(m.describeInputs, input)
	if m.describeErr != nil {
		return nil, m.describeErr
	}
	if len(m.describeOutputs) == 0 {
		if m.describeDefault != nil {
			return m.describeDefault, nil
		}
		return instanceState(types.InstanceStateNameRunning), nil
	}
	output := m.describeOutputs[0]
	m.describeOutputs = m.describeOutputs[1:]
	return output, nil
}

func (m *mockEC2) TerminateInstances(_ context.Context, input *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	m.terminateInput = input
	m.terminateInputs = append(m.terminateInputs, input)
	return &ec2.TerminateInstancesOutput{}, m.terminateErr
}

func instanceState(state types.InstanceStateName) *ec2.DescribeInstancesOutput {
	return &ec2.DescribeInstancesOutput{Reservations: []types.Reservation{{Instances: []types.Instance{{State: &types.InstanceState{Name: state}}}}}}
}

func testConfig() Config {
	return Config{Region: "us-east-1", LaunchTemplateID: "lt-123", SubnetID: "subnet-123", SecurityGroupIDs: []string{"sg-123"}, PollInterval: time.Millisecond, ReadyTimeout: time.Second}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing region", func(c *Config) { c.Region = "" }},
		{"both launch template selectors", func(c *Config) { c.LaunchTemplateName = "template" }},
		{"neither launch template selector", func(c *Config) { c.LaunchTemplateID = "" }},
		{"both instance profile selectors", func(c *Config) {
			c.InstanceProfileARN = "arn:aws:iam::1:instance-profile/x"
			c.InstanceProfileName = "x"
		}},
		{"negative timeout", func(c *Config) { c.ReadyTimeout = -time.Second }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig()
			test.mutate(&config)
			if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestProvisionUsesLaunchTemplateIMDSv2AndOwnershipTags(t *testing.T) {
	client := &mockEC2{}
	provider, err := New(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	provider.newID = func() (string, error) { return "runner-fixed", nil }
	instance, err := provider.Provision(context.Background(), providers.RunnerSpec{ExpiresAt: time.Unix(100, 0), Metadata: map[string]string{"job_id": "job-7", "owner": "org/repo", "repository": "org/repo", "run_id": "88"}})
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID != "runner-fixed" || instance.ProviderID != "i-123" || instance.Status != providers.StatusProvisioning {
		t.Fatalf("unexpected instance: %+v", instance)
	}
	in := client.runInput
	if in == nil || in.LaunchTemplate == nil || awsv2.ToString(in.LaunchTemplate.LaunchTemplateId) != "lt-123" {
		t.Fatalf("missing launch template: %+v", in)
	}
	if in.MinCount == nil || in.MaxCount == nil || *in.MinCount != 1 || *in.MaxCount != 1 {
		t.Fatalf("unexpected counts: %v %v", in.MinCount, in.MaxCount)
	}
	if in.LaunchTemplate.Version != nil {
		t.Fatalf("launch template version = %q, want provider default", awsv2.ToString(in.LaunchTemplate.Version))
	}
	if in.MetadataOptions == nil || in.MetadataOptions.HttpTokens != types.HttpTokensStateRequired || in.MetadataOptions.HttpEndpoint != types.InstanceMetadataEndpointStateEnabled {
		t.Fatalf("IMDSv2 not required: %+v", in.MetadataOptions)
	}
	if len(in.TagSpecifications) != 1 {
		t.Fatalf("TagSpecifications = %d", len(in.TagSpecifications))
	}
	tags := map[string]string{}
	for _, tag := range in.TagSpecifications[0].Tags {
		tags[awsv2.ToString(tag.Key)] = awsv2.ToString(tag.Value)
	}
	for key, want := range map[string]string{managedByTag: "leo-runners", runnerIDTag: "runner-fixed", jobIDTag: "job-7", ownerTag: "org/repo", repositoryTag: "org/repo", runIDTag: "88"} {
		if tags[key] != want {
			t.Errorf("tag %q = %q, want %q", key, tags[key], want)
		}
	}
}

func TestProvisionLaunchRequestContractWithPinnedTemplate(t *testing.T) {
	client := &mockEC2{}
	config := testConfig()
	config.LaunchTemplateID = ""
	config.LaunchTemplateName = "leo-runner-template"
	config.LaunchTemplateVersion = "42"
	config.InstanceType = "m7g.large"
	config.InstanceProfileARN = "arn:aws:iam::123456789012:instance-profile/runner"
	config.SubnetID = "subnet-pinned"
	config.SecurityGroupIDs = []string{"sg-one", "sg-two"}
	provider, err := New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	provider.newID = func() (string, error) { return "runner-contract", nil }
	_, err = provider.Provision(context.Background(), providers.RunnerSpec{})
	if err != nil {
		t.Fatal(err)
	}
	in := client.runInput
	if in == nil || in.LaunchTemplate == nil {
		t.Fatalf("missing launch template request: %+v", in)
	}
	if awsv2.ToString(in.LaunchTemplate.LaunchTemplateName) != "leo-runner-template" || awsv2.ToString(in.LaunchTemplate.Version) != "42" {
		t.Fatalf("unexpected launch template: %+v", in.LaunchTemplate)
	}
	if in.LaunchTemplate.LaunchTemplateId != nil {
		t.Fatal("LaunchTemplateId set when name selector was configured")
	}
	if awsv2.ToString(in.SubnetId) != "subnet-pinned" || len(in.SecurityGroupIds) != 2 || awsv2.ToString(in.IamInstanceProfile.Arn) == "" || string(in.InstanceType) != "m7g.large" {
		t.Fatalf("launch request network/profile/type contract violated: %+v", in)
	}
	if in.MinCount == nil || in.MaxCount == nil || *in.MinCount != 1 || *in.MaxCount != 1 {
		t.Fatalf("counts = %v/%v, want 1/1", in.MinCount, in.MaxCount)
	}
}

func TestProvisionUsesStableAttemptClientTokenAndSecureJITUserData(t *testing.T) {
	client := &mockEC2{}
	config := testConfig()
	config.UserData = "#!/bin/sh\nGITHUB_JIT_CONFIG_B64=" + UserDataJITPlaceholder + "\nexec ./bootstrap.sh"
	provider, err := New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	provider.newID = func() (string, error) { return "runner-fixed", nil }
	const secret = "c2Vuc2l0aXZlLXBheWxvYWQ="
	spec := providers.RunnerSpec{Metadata: map[string]string{
		MetadataProviderAttemptKey: "job-7-attempt-1",
		MetadataJITConfig:          secret,
	}}
	instance, err := provider.Provision(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}

	wantToken, err := providerClientToken(spec)
	if err != nil {
		t.Fatal(err)
	}
	if client.runInput == nil || awsv2.ToString(client.runInput.ClientToken) != wantToken {
		t.Fatalf("ClientToken = %q, want %q", awsv2.ToString(client.runInput.ClientToken), wantToken)
	}
	if len(wantToken) != 64 || strings.ContainsAny(wantToken, "\r\n") {
		t.Fatalf("invalid EC2 client token %q", wantToken)
	}
	if instance.Spec.Metadata[MetadataJITConfig] != "" {
		t.Fatal("JIT config was retained in provider instance state")
	}
	if client.runInput.UserData == nil || strings.Contains(awsv2.ToString(client.runInput.UserData), secret) {
		t.Fatal("JIT config was sent as unencoded user data")
	}
	decoded, err := base64.StdEncoding.DecodeString(awsv2.ToString(client.runInput.UserData))
	if err != nil {
		t.Fatalf("decode user data: %v", err)
	}
	if got := string(decoded); !strings.Contains(got, "GITHUB_JIT_CONFIG_B64='"+secret+"'") {
		t.Fatalf("rendered user data does not contain the quoted JIT config: %q", got)
	}
}

func TestProvisionRejectsJITWithoutExactlyOneUserDataPlaceholder(t *testing.T) {
	for _, template := range []string{"", "#!/bin/sh\nexec ./bootstrap.sh", UserDataJITPlaceholder + "\n" + UserDataJITPlaceholder} {
		t.Run(template, func(t *testing.T) {
			client := &mockEC2{}
			config := testConfig()
			config.UserData = template
			provider, err := New(client, config)
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Provision(context.Background(), providers.RunnerSpec{Metadata: map[string]string{MetadataJITConfig: "c2VjcmV0"}})
			if !errors.Is(err, ErrInvalidBootstrap) {
				t.Fatalf("Provision() error = %v, want ErrInvalidBootstrap", err)
			}
			if client.runInput != nil {
				t.Fatal("RunInstances called after invalid bootstrap template")
			}
		})
	}
}

func TestRenderUserDataWithoutJITPreservesStaticValue(t *testing.T) {
	const userData = "already-rendered-user-data"
	got, err := renderUserData(userData, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != userData {
		t.Fatalf("renderUserData() = %q, want %q", got, userData)
	}
}

func TestProviderClientTokenIsStableForAttemptKey(t *testing.T) {
	spec := providers.RunnerSpec{Metadata: map[string]string{MetadataProviderAttemptKey: "attempt-123"}}
	first, err := providerClientToken(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := providerClientToken(spec)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("tokens are not stable: %q and %q", first, second)
	}
}

func TestProvisionUsesSameClientTokenAcrossRetries(t *testing.T) {
	client := &mockEC2{}
	provider, err := New(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	provider.newID = func() (string, error) { return "runner-retry", nil }
	spec := providers.RunnerSpec{Metadata: map[string]string{MetadataProviderAttemptKey: "durable-attempt-9"}}
	if _, err = provider.Provision(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Provision(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if len(client.runInputs) != 2 {
		t.Fatalf("RunInstances calls = %d, want 2", len(client.runInputs))
	}
	for i, input := range client.runInputs {
		if awsv2.ToString(input.ClientToken) != awsv2.ToString(client.runInputs[0].ClientToken) {
			t.Errorf("attempt %d ClientToken = %q, want %q", i, awsv2.ToString(input.ClientToken), awsv2.ToString(client.runInputs[0].ClientToken))
		}
	}
}

func TestWaitReadyPollsUntilRunning(t *testing.T) {
	client := &mockEC2{describeOutputs: []*ec2.DescribeInstancesOutput{instanceState(types.InstanceStateNamePending), instanceState(types.InstanceStateNameRunning)}}
	provider, err := New(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	err = provider.WaitReady(context.Background(), providers.RunnerInstance{ProviderID: "i-123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.describeInputs) != 2 {
		t.Fatalf("DescribeInstances calls = %d, want 2", len(client.describeInputs))
	}
}

func TestWaitReadyHonorsCancellation(t *testing.T) {
	client := &mockEC2{describeDefault: instanceState(types.InstanceStateNamePending)}
	config := testConfig()
	config.PollInterval = time.Hour
	provider, err := New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.WaitReady(ctx, providers.RunnerInstance{ProviderID: "i-123"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitReady() error = %v, want context.Canceled", err)
	}
}

func TestWaitReadyReturnsBoundedTimeoutForPendingInstance(t *testing.T) {
	client := &mockEC2{describeDefault: instanceState(types.InstanceStateNamePending)}
	config := testConfig()
	config.PollInterval = time.Millisecond
	config.ReadyTimeout = 8 * time.Millisecond
	provider, err := New(client, config)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = provider.WaitReady(context.Background(), providers.RunnerInstance{ProviderID: "i-pending"})
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("WaitReady() error = %v, want ErrNotReady", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("WaitReady exceeded bounded test duration: %s", elapsed)
	}
	if len(client.describeInputs) < 2 {
		t.Fatalf("DescribeInstances calls = %d, want repeated polling", len(client.describeInputs))
	}
}

func TestStatusMapsStoppedAndTerminated(t *testing.T) {
	for state, want := range map[types.InstanceStateName]providers.RunnerStatus{types.InstanceStateNameStopped: providers.StatusFailed, types.InstanceStateNameTerminated: providers.StatusTerminated} {
		client := &mockEC2{describeOutputs: []*ec2.DescribeInstancesOutput{instanceState(state)}}
		provider, err := New(client, testConfig())
		if err != nil {
			t.Fatal(err)
		}
		got, err := provider.Status(context.Background(), providers.RunnerInstance{ProviderID: "i-123"})
		if err != nil || got != want {
			t.Errorf("Status(%s) = %s, %v; want %s", state, got, err, want)
		}
	}
}

func TestStatusMapsAllLifecycleStates(t *testing.T) {
	tests := map[types.InstanceStateName]providers.RunnerStatus{
		types.InstanceStateNamePending:      providers.StatusProvisioning,
		types.InstanceStateNameRunning:      providers.StatusReady,
		types.InstanceStateNameShuttingDown: providers.StatusTerminating,
		types.InstanceStateNameTerminated:   providers.StatusTerminated,
		types.InstanceStateNameStopping:     providers.StatusFailed,
		types.InstanceStateNameStopped:      providers.StatusFailed,
	}
	for state, want := range tests {
		t.Run(string(state), func(t *testing.T) {
			client := &mockEC2{describeOutputs: []*ec2.DescribeInstancesOutput{instanceState(state)}}
			provider, err := New(client, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			got, err := provider.Status(context.Background(), providers.RunnerInstance{ProviderID: "i-lifecycle"})
			if err != nil || got != want {
				t.Fatalf("Status(%s) = %s, %v; want %s", state, got, err, want)
			}
		})
	}
}

func TestTerminateIsIdempotentForNotFound(t *testing.T) {
	client := &mockEC2{terminateErr: notFoundError{}}
	provider, err := New(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Terminate(context.Background(), providers.RunnerInstance{ProviderID: "i-123"}); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	if client.terminateInput == nil || len(client.terminateInput.InstanceIds) != 1 || client.terminateInput.InstanceIds[0] != "i-123" {
		t.Fatalf("unexpected terminate input: %+v", client.terminateInput)
	}
}

func TestTerminateCanBeRepeatedAfterNotFound(t *testing.T) {
	client := &mockEC2{terminateErr: notFoundError{}}
	provider, err := New(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	instance := providers.RunnerInstance{ProviderID: "i-already-gone"}
	for attempt := 0; attempt < 3; attempt++ {
		if err := provider.Terminate(context.Background(), instance); err != nil {
			t.Fatalf("Terminate() attempt %d error = %v", attempt+1, err)
		}
	}
	if len(client.terminateInputs) != 3 {
		t.Fatalf("TerminateInstances calls = %d, want 3", len(client.terminateInputs))
	}
	for _, input := range client.terminateInputs {
		if len(input.InstanceIds) != 1 || input.InstanceIds[0] != instance.ProviderID {
			t.Fatalf("unexpected termination input: %+v", input)
		}
	}
}

type notFoundError struct{}

func (notFoundError) Error() string { return "InvalidInstanceID.NotFound" }

func TestTerminateHonorsCancellation(t *testing.T) {
	client := &mockEC2{}
	provider, err := New(client, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.Terminate(ctx, providers.RunnerInstance{ProviderID: "i-123"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Terminate() error = %v", err)
	}
	if client.terminateInput != nil {
		t.Fatal("TerminateInstances called with canceled context")
	}
}
