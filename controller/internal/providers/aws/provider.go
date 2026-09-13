// Package aws implements the runner provider using Amazon EC2.
package aws

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	providers "github.com/leo-runners/ci-platform/controller/internal/providers"
)

var (
	ErrInvalidConfig   = errors.New("invalid AWS provider configuration")
	ErrNotReady        = errors.New("EC2 runner did not become ready")
	ErrInstanceMissing = providers.ErrInstanceNotFound
)

const (
	// MetadataProviderAttemptKey is a stable controller-generated key for one
	// provisioning attempt. It is hashed before being sent to EC2 as a
	// ClientToken so arbitrary controller IDs cannot violate EC2's token rules.
	MetadataProviderAttemptKey = "provider_attempt_key"
	MetadataJITConfig          = "github_jit_config"
	UserDataJITPlaceholder     = "{{GITHUB_JIT_CONFIG_B64}}"

	managedByTag  = "leo-runners:managed-by"
	runnerIDTag   = "leo-runners:runner-id"
	jobIDTag      = "leo-runners:job-id"
	ownerTag      = "leo-runners:owner"
	repositoryTag = "leo-runners:repository"
	runIDTag      = "leo-runners:run-id"
	workflowTag   = "leo-runners:workflow"
	createdAtTag  = "leo-runners:created-at"
	expiresAtTag  = "leo-runners:expires-at"
)

var ErrInvalidBootstrap = errors.New("invalid runner bootstrap user data")

// EC2API is the subset of the AWS EC2 client used by Provider. It keeps cloud
// calls mockable while remaining directly compatible with *ec2.Client.
type EC2API interface {
	RunInstances(context.Context, *ec2.RunInstancesInput, ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

// Config contains AWS-specific provisioning settings. Network and IAM values
// may be supplied by the launch template or overridden here.
type Config struct {
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
	Tags                  map[string]string
	PollInterval          time.Duration
	ReadyTimeout          time.Duration
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Region) == "" {
		return fmt.Errorf("%w: region is required", ErrInvalidConfig)
	}
	if (strings.TrimSpace(c.LaunchTemplateID) == "") == (strings.TrimSpace(c.LaunchTemplateName) == "") {
		return fmt.Errorf("%w: exactly one launch template ID or name is required", ErrInvalidConfig)
	}
	if c.InstanceProfileARN != "" && c.InstanceProfileName != "" {
		return fmt.Errorf("%w: instance profile ARN and name are mutually exclusive", ErrInvalidConfig)
	}
	if c.PollInterval < 0 || c.ReadyTimeout < 0 {
		return fmt.Errorf("%w: durations cannot be negative", ErrInvalidConfig)
	}
	for key, value := range c.Tags {
		if !validTagPart(key) || !validTagPart(value) {
			return fmt.Errorf("%w: invalid tag %q", ErrInvalidConfig, key)
		}
		if isReservedTag(key) {
			return fmt.Errorf("%w: reserved ownership tag %q cannot be overridden", ErrInvalidConfig, key)
		}
	}
	return nil
}

func validTagPart(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\r\n")
}

// Provider provisions one EC2 instance per ephemeral runner.
type Provider struct {
	client EC2API
	config Config
	now    func() time.Time
	newID  func() (string, error)
}

var _ providers.Provider = (*Provider)(nil)

func New(client EC2API, config Config) (*Provider, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: EC2 client is required", ErrInvalidConfig)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.PollInterval == 0 {
		config.PollInterval = 2 * time.Second
	}
	if config.ReadyTimeout == 0 {
		config.ReadyTimeout = 10 * time.Minute
	}
	return &Provider{client: client, config: cloneConfig(config), now: time.Now, newID: newRunnerID}, nil
}

func NewProvider(client EC2API, config Config) (*Provider, error) { return New(client, config) }

func (p *Provider) Provision(ctx context.Context, spec providers.RunnerSpec) (providers.RunnerInstance, error) {
	if err := contextErr(ctx); err != nil {
		return providers.RunnerInstance{}, err
	}
	runnerID, err := stableRunnerID(spec, p.newID)
	if err != nil {
		return providers.RunnerInstance{}, fmt.Errorf("create runner ID: %w", err)
	}
	input := &ec2.RunInstancesInput{
		MinCount: awsv2.Int32(1), MaxCount: awsv2.Int32(1),
		LaunchTemplate: &types.LaunchTemplateSpecification{
			LaunchTemplateId:   optionalString(p.config.LaunchTemplateID),
			LaunchTemplateName: optionalString(p.config.LaunchTemplateName),
			Version:            optionalString(p.config.LaunchTemplateVersion),
		},
		MetadataOptions:   &types.InstanceMetadataOptionsRequest{HttpEndpoint: types.InstanceMetadataEndpointStateEnabled, HttpTokens: types.HttpTokensStateRequired},
		TagSpecifications: []types.TagSpecification{{ResourceType: types.ResourceTypeInstance, Tags: p.tags(runnerID, spec)}},
	}
	if p.config.SubnetID != "" {
		input.SubnetId = awsv2.String(p.config.SubnetID)
	}
	if len(p.config.SecurityGroupIDs) > 0 {
		input.SecurityGroupIds = append([]string(nil), p.config.SecurityGroupIDs...)
	}
	if p.config.InstanceProfileARN != "" || p.config.InstanceProfileName != "" {
		input.IamInstanceProfile = &types.IamInstanceProfileSpecification{Arn: optionalString(p.config.InstanceProfileARN), Name: optionalString(p.config.InstanceProfileName)}
	}
	if p.config.InstanceType != "" {
		input.InstanceType = types.InstanceType(p.config.InstanceType)
	}
	if token, err := providerClientToken(spec); err != nil {
		return providers.RunnerInstance{}, err
	} else if token != "" {
		input.ClientToken = awsv2.String(token)
	}
	userData, err := renderUserData(p.config.UserData, spec.Metadata[MetadataJITConfig])
	if err != nil {
		return providers.RunnerInstance{}, err
	}
	if userData != "" {
		input.UserData = awsv2.String(userData)
	}
	out, err := p.client.RunInstances(ctx, input)
	if err != nil {
		return providers.RunnerInstance{}, fmt.Errorf("run EC2 instance: %w", err)
	}
	if len(out.Instances) != 1 || out.Instances[0].InstanceId == nil || *out.Instances[0].InstanceId == "" {
		return providers.RunnerInstance{}, fmt.Errorf("run EC2 instance: %w", ErrInstanceMissing)
	}
	instanceID := awsv2.ToString(out.Instances[0].InstanceId)
	created := p.now().UTC()
	// The JIT value is deliberately not retained in the provider-owned copy.
	return providers.RunnerInstance{ID: runnerID, ProviderID: instanceID, Provider: "aws", Status: providers.StatusProvisioning, CreatedAt: created, ExpiresAt: spec.ExpiresAt, Spec: cloneSpecWithoutSecrets(spec)}, nil
}

func stableRunnerID(spec providers.RunnerSpec, fallback func() (string, error)) (string, error) {
	if key := strings.TrimSpace(spec.Metadata[MetadataProviderAttemptKey]); key != "" {
		digest := sha256.Sum256([]byte("leo-runners/runner-id/" + key))
		return "runner-" + hex.EncodeToString(digest[:])[:32], nil
	}
	return fallback()
}

func providerClientToken(spec providers.RunnerSpec) (string, error) {
	key, ok := spec.Metadata[MetadataProviderAttemptKey]
	if !ok {
		// Keep Phase 2 callers compatible. Production callers should always set
		// this key from their durable provisioning-attempt identity.
		return "", nil
	}
	if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n") {
		return "", fmt.Errorf("%w: %s is empty or contains a newline", ErrInvalidBootstrap, MetadataProviderAttemptKey)
	}
	digest := sha256.Sum256([]byte("leo-runners/provider-attempt/" + key))
	return hex.EncodeToString(digest[:]), nil
}

// renderUserData substitutes the one-time JIT value into a user-data
// template. UserData is raw script content; EC2 requires the final value to
// be base64 encoded. The replacement is shell-quoted and is never logged.
func renderUserData(template, encodedJIT string) (string, error) {
	if encodedJIT == "" {
		return template, nil
	}
	if template == "" {
		return "", fmt.Errorf("%w: JIT config requires user-data template", ErrInvalidBootstrap)
	}
	count := strings.Count(template, UserDataJITPlaceholder)
	if count != 1 {
		return "", fmt.Errorf("%w: expected exactly one %s placeholder, found %d", ErrInvalidBootstrap, UserDataJITPlaceholder, count)
	}
	// GitHub's encoded JIT value is base64, whose alphabet cannot contain a
	// single quote. Keep this check explicit so the shell assignment remains
	// safe if the upstream contract ever changes.
	if strings.Contains(encodedJIT, "'") || strings.ContainsAny(encodedJIT, "\r\n") {
		return "", fmt.Errorf("%w: JIT config contains unsafe shell characters", ErrInvalidBootstrap)
	}
	replacement := "'" + encodedJIT + "'"
	rendered := strings.Replace(template, UserDataJITPlaceholder, replacement, 1)
	return base64.StdEncoding.EncodeToString([]byte(rendered)), nil
}

func (p *Provider) WaitReady(ctx context.Context, instance providers.RunnerInstance) error {
	waitCtx := ctx
	if p.config.ReadyTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, p.config.ReadyTimeout)
		defer cancel()
	}
	for {
		status, err := p.Status(waitCtx, instance)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("%w: %v", ErrNotReady, err)
			}
			return err
		}
		switch status {
		case providers.StatusReady:
			return nil
		case providers.StatusTerminated, providers.StatusTerminating, providers.StatusFailed:
			return fmt.Errorf("%w: instance %s status is %s", ErrNotReady, instance.ProviderID, status)
		}
		if err := sleepContext(waitCtx, p.config.PollInterval); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("%w: %v", ErrNotReady, err)
			}
			return err
		}
	}
}

func (p *Provider) Status(ctx context.Context, instance providers.RunnerInstance) (providers.RunnerStatus, error) {
	if err := contextErr(ctx); err != nil {
		return providers.StatusUnknown, err
	}
	if instance.ProviderID == "" {
		return providers.StatusUnknown, ErrInstanceMissing
	}
	out, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instance.ProviderID}})
	if err != nil {
		if isNotFound(err) {
			return providers.StatusUnknown, ErrInstanceMissing
		}
		return providers.StatusUnknown, fmt.Errorf("describe EC2 instance: %w", err)
	}
	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 || out.Reservations[0].Instances[0].State == nil {
		return providers.StatusUnknown, ErrInstanceMissing
	}
	switch out.Reservations[0].Instances[0].State.Name {
	case types.InstanceStateNamePending:
		return providers.StatusProvisioning, nil
	case types.InstanceStateNameRunning:
		return providers.StatusReady, nil
	case types.InstanceStateNameShuttingDown:
		return providers.StatusTerminating, nil
	case types.InstanceStateNameTerminated:
		return providers.StatusTerminated, nil
	case types.InstanceStateNameStopping, types.InstanceStateNameStopped:
		return providers.StatusFailed, nil
	default:
		return providers.StatusUnknown, nil
	}
}

func (p *Provider) Terminate(ctx context.Context, instance providers.RunnerInstance) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if instance.ProviderID == "" {
		return nil
	}
	_, err := p.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{instance.ProviderID}})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("terminate EC2 instance: %w", err)
	}
	return nil
}

func (p *Provider) tags(runnerID string, spec providers.RunnerSpec) []types.Tag {
	values := map[string]string{managedByTag: "leo-runners", runnerIDTag: runnerID}
	for key, tagKey := range map[string]string{"job_id": jobIDTag, "owner": ownerTag, "repository": repositoryTag, "run_id": runIDTag, "workflow": workflowTag, "created_at": createdAtTag, "expires_at": expiresAtTag} {
		if value := spec.Metadata[key]; value != "" {
			values[tagKey] = value
		}
	}
	for key, value := range p.config.Tags {
		values[key] = value
	}
	tags := make([]types.Tag, 0, len(values))
	for key, value := range values {
		tags = append(tags, types.Tag{Key: awsv2.String(key), Value: awsv2.String(value)})
	}
	return tags
}

func isReservedTag(key string) bool {
	switch key {
	case managedByTag, runnerIDTag, jobIDTag, ownerTag, repositoryTag, runIDTag:
		return true
	default:
		return false
	}
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "InvalidInstanceID.NotFound" || apiErr.ErrorCode() == "InvalidInstanceID"
	}
	return strings.Contains(err.Error(), "InvalidInstanceID.NotFound")
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return awsv2.String(value)
}

func newRunnerID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "runner-" + hex.EncodeToString(b), nil
}

func cloneConfig(config Config) Config {
	config.SecurityGroupIDs = append([]string(nil), config.SecurityGroupIDs...)
	config.Tags = mapsClone(config.Tags)
	return config
}
func mapsClone(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for k, v := range input {
		output[k] = v
	}
	return output
}
func cloneSpec(spec providers.RunnerSpec) providers.RunnerSpec {
	spec.Labels = append([]string(nil), spec.Labels...)
	spec.Metadata = mapsClone(spec.Metadata)
	return spec
}

func cloneSpecWithoutSecrets(spec providers.RunnerSpec) providers.RunnerSpec {
	spec = cloneSpec(spec)
	delete(spec.Metadata, MetadataJITConfig)
	return spec
}
