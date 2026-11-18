// Package gcp implements the runner provider using Google Compute Engine.
package gcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2"
	providers "github.com/leo-runners/ci-platform/controller/internal/providers"
)

var (
	ErrInvalidConfig   = errors.New("invalid GCP provider configuration")
	ErrNotReady        = errors.New("GCE runner did not become ready")
	ErrInstanceMissing = providers.ErrInstanceNotFound
)

const (
	MetadataAttemptKey = "provider_attempt_key"
	managedByLabel     = "platform"
	repositoryLabel    = "repository"
	workflowLabel      = "workflow"
	runLabel           = "run"
	jobLabel           = "job"
	runnerLabel        = "runner"
	createdLabel       = "created"
	expiryLabel        = "expiry"
)

var labelValuePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$`)

// Config contains the GCP-specific settings needed to launch one VM.
// InstanceTemplate must be a fully qualified or same-project template URL.
type Config struct {
	Project          string
	Zone             string
	InstanceTemplate string
	NamePrefix       string
	Labels           map[string]string
	PollInterval     time.Duration
	ReadyTimeout     time.Duration
	OperationTimeout time.Duration
	UserData         string
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Project) == "" {
		return fmt.Errorf("%w: project is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(c.Zone) == "" {
		return fmt.Errorf("%w: zone is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(c.InstanceTemplate) == "" {
		return fmt.Errorf("%w: instance template is required", ErrInvalidConfig)
	}
	if c.PollInterval < 0 || c.ReadyTimeout < 0 || c.OperationTimeout < 0 {
		return fmt.Errorf("%w: durations cannot be negative", ErrInvalidConfig)
	}
	if c.NamePrefix == "" {
		c.NamePrefix = "leo-runner"
	}
	if !validNamePart(c.NamePrefix) {
		return fmt.Errorf("%w: invalid name prefix", ErrInvalidConfig)
	}
	for key, value := range c.Labels {
		if !validLabelKey(key) || !validLabelValue(value) || isReservedLabel(key) {
			return fmt.Errorf("%w: invalid or reserved label %q", ErrInvalidConfig, key)
		}
	}
	return nil
}

// InstancesAPI is the mockable subset of the Compute Engine Instances API.
type InstancesAPI interface {
	Insert(context.Context, *computepb.InsertInstanceRequest, ...gax.CallOption) (Operation, error)
	Get(context.Context, *computepb.GetInstanceRequest, ...gax.CallOption) (*computepb.Instance, error)
	Delete(context.Context, *computepb.DeleteInstanceRequest, ...gax.CallOption) (Operation, error)
}

// Operation is the completion boundary for a GCE zonal operation.
type Operation interface {
	Wait(context.Context, ...gax.CallOption) error
}

type computeClient struct{ client *compute.InstancesClient }

func (c computeClient) Insert(ctx context.Context, req *computepb.InsertInstanceRequest, opts ...gax.CallOption) (Operation, error) {
	return c.client.Insert(ctx, req, opts...)
}
func (c computeClient) Get(ctx context.Context, req *computepb.GetInstanceRequest, opts ...gax.CallOption) (*computepb.Instance, error) {
	return c.client.Get(ctx, req, opts...)
}
func (c computeClient) Delete(ctx context.Context, req *computepb.DeleteInstanceRequest, opts ...gax.CallOption) (Operation, error) {
	return c.client.Delete(ctx, req, opts...)
}

// Provider provisions one Compute Engine VM per ephemeral runner.
type Provider struct {
	client InstancesAPI
	config Config
	now    func() time.Time
	newID  func() (string, error)
}

var _ providers.Provider = (*Provider)(nil)

// New constructs a provider using the official Compute Engine client.
func New(client *compute.InstancesClient, config Config) (*Provider, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: Compute Engine client is required", ErrInvalidConfig)
	}
	return NewWithClient(computeClient{client: client}, config)
}

// NewWithClient is intended for tests and custom transports.
func NewWithClient(client InstancesAPI, config Config) (*Provider, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: Compute Engine client is required", ErrInvalidConfig)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.NamePrefix == "" {
		config.NamePrefix = "leo-runner"
	}
	if config.PollInterval == 0 {
		config.PollInterval = 2 * time.Second
	}
	if config.ReadyTimeout == 0 {
		config.ReadyTimeout = 10 * time.Minute
	}
	if config.OperationTimeout == 0 {
		config.OperationTimeout = 10 * time.Minute
	}
	return &Provider{client: client, config: cloneConfig(config), now: time.Now, newID: newRunnerID}, nil
}

func NewProvider(client *compute.InstancesClient, config Config) (*Provider, error) {
	return New(client, config)
}

func (p *Provider) Provision(ctx context.Context, spec providers.RunnerSpec) (providers.RunnerInstance, error) {
	if err := contextErr(ctx); err != nil {
		return providers.RunnerInstance{}, err
	}
	runnerID, err := stableRunnerID(spec, p.newID)
	if err != nil {
		return providers.RunnerInstance{}, fmt.Errorf("create runner ID: %w", err)
	}
	name := p.config.NamePrefix + "-" + strings.TrimPrefix(runnerID, "runner-")
	if len(name) > 63 {
		name = name[:63]
		name = strings.TrimRight(name, "-")
	}
	labels, err := p.labels(runnerID, spec)
	if err != nil {
		return providers.RunnerInstance{}, err
	}
	requestID, err := providerRequestID(spec)
	if err != nil {
		return providers.RunnerInstance{}, err
	}
	resource := &computepb.Instance{Name: stringPtr(name), Labels: labels}
	jitConfig := spec.Metadata["github_jit_config"]
	if jitConfig != "" && p.config.UserData == "" {
		return providers.RunnerInstance{}, fmt.Errorf("%w: JIT config requires startup-script user data", ErrInvalidConfig)
	}
	if p.config.UserData != "" {
		startup, err := renderUserData(p.config.UserData, jitConfig)
		if err != nil {
			return providers.RunnerInstance{}, err
		}
		resource.Metadata = &computepb.Metadata{Items: []*computepb.Items{{Key: stringPtr("startup-script"), Value: stringPtr(startup)}}}
	}
	req := &computepb.InsertInstanceRequest{Project: p.config.Project, Zone: p.config.Zone, RequestId: stringPtr(requestID), SourceInstanceTemplate: stringPtr(p.config.InstanceTemplate), InstanceResource: resource}
	opCtx, cancel := operationContext(ctx, p.config.OperationTimeout)
	defer cancel()
	op, err := p.client.Insert(opCtx, req)
	if err != nil {
		return providers.RunnerInstance{}, fmt.Errorf("insert GCE instance: %w", err)
	}
	if op == nil {
		return providers.RunnerInstance{}, fmt.Errorf("insert GCE instance: %w", ErrInstanceMissing)
	}
	if err := op.Wait(opCtx); err != nil {
		return providers.RunnerInstance{}, fmt.Errorf("wait for GCE insert: %w", err)
	}
	created := p.now().UTC()
	return providers.RunnerInstance{ID: runnerID, ProviderID: name, Provider: "gcp", Status: providers.StatusProvisioning, CreatedAt: created, ExpiresAt: spec.ExpiresAt, Spec: cloneSpec(spec)}, nil
}

func stableRunnerID(spec providers.RunnerSpec, fallback func() (string, error)) (string, error) {
	if key := strings.TrimSpace(spec.Metadata[MetadataAttemptKey]); key != "" {
		digest := sha256.Sum256([]byte("leo-runners/runner-id/" + key))
		return "runner-" + hex.EncodeToString(digest[:])[:32], nil
	}
	return fallback()
}

func renderUserData(template, encodedJIT string) (string, error) {
	if encodedJIT == "" {
		return template, nil
	}
	const placeholder = "{{GITHUB_JIT_CONFIG_B64}}"
	if strings.Count(template, placeholder) != 1 || strings.ContainsAny(encodedJIT, "'\r\n") {
		return "", fmt.Errorf("%w: startup script must contain one safe JIT placeholder", ErrInvalidConfig)
	}
	return strings.Replace(template, placeholder, "'"+encodedJIT+"'", 1), nil
}

func (p *Provider) WaitReady(ctx context.Context, instance providers.RunnerInstance) error {
	waitCtx, cancel := operationContext(ctx, p.config.ReadyTimeout)
	defer cancel()
	for {
		status, err := p.Status(waitCtx, instance)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("%w: %v", ErrNotReady, err)
			}
			return err
		}
		if status == providers.StatusReady {
			return nil
		}
		if status == providers.StatusTerminated || status == providers.StatusTerminating || status == providers.StatusFailed {
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
	got, err := p.client.Get(ctx, &computepb.GetInstanceRequest{Project: p.config.Project, Zone: p.config.Zone, Instance: instance.ProviderID})
	if err != nil {
		if isNotFound(err) {
			return providers.StatusUnknown, ErrInstanceMissing
		}
		return providers.StatusUnknown, fmt.Errorf("get GCE instance: %w", err)
	}
	if got == nil {
		return providers.StatusUnknown, ErrInstanceMissing
	}
	switch got.GetStatus() {
	case "RUNNING":
		return providers.StatusReady, nil
	case "PROVISIONING", "STAGING":
		return providers.StatusProvisioning, nil
	case "STOPPING", "SUSPENDING":
		return providers.StatusTerminating, nil
	case "TERMINATED":
		return providers.StatusTerminated, nil
	case "SUSPENDED":
		return providers.StatusFailed, nil
	case "REPAIRING":
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
	opCtx, cancel := operationContext(ctx, p.config.OperationTimeout)
	defer cancel()
	op, err := p.client.Delete(opCtx, &computepb.DeleteInstanceRequest{Project: p.config.Project, Zone: p.config.Zone, Instance: instance.ProviderID})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("delete GCE instance: %w", err)
	}
	if err == nil && op != nil {
		if err := op.Wait(opCtx); err != nil && !isNotFound(err) {
			return fmt.Errorf("wait for GCE delete: %w", err)
		}
	}
	return nil
}

func (p *Provider) labels(runnerID string, spec providers.RunnerSpec) (map[string]string, error) {
	labels := map[string]string{managedByLabel: "leo-runners", runnerLabel: runnerID, createdLabel: "t" + p.now().UTC().Format("20060102t150405z"), expiryLabel: expiryValue(spec.ExpiresAt)}
	for metadata, label := range map[string]string{"repository": repositoryLabel, "workflow": workflowLabel, "run_id": runLabel, "job_id": jobLabel} {
		if value := spec.Metadata[metadata]; value != "" {
			labels[label] = normalizeLabelValue(value)
		}
	}
	for key, value := range p.config.Labels {
		labels[key] = value
	}
	for key, value := range labels {
		if !validLabelKey(key) || !validLabelValue(value) {
			return nil, fmt.Errorf("%w: label %q cannot be represented by GCE", ErrInvalidConfig, key)
		}
	}
	return labels, nil
}

func expiryValue(value time.Time) string {
	if value.IsZero() {
		return "none"
	}
	return "t" + value.UTC().Format("20060102t150405z")
}

// providerRequestID is the Compute Engine idempotency key. Compute Engine
// requires a UUID-shaped request ID, so derive one deterministically from the
// durable attempt key.
func providerRequestID(spec providers.RunnerSpec) (string, error) {
	key := spec.Metadata[MetadataAttemptKey]
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("%w: %s is required for idempotent insert", ErrInvalidConfig, MetadataAttemptKey)
	}
	if strings.ContainsAny(key, "\r\n") {
		return "", fmt.Errorf("%w: %s contains a newline", ErrInvalidConfig, MetadataAttemptKey)
	}
	digest := sha256.Sum256([]byte("leo-runners/gcp-insert/" + key))
	digest[6] = (digest[6] & 0x0f) | 0x50
	digest[8] = (digest[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", digest[0:4], digest[4:6], digest[6:8], digest[8:10], digest[10:16]), nil
}
func validLabelKey(value string) bool {
	return value != "" && len(value) <= 63 && labelValuePattern.MatchString(value)
}
func validLabelValue(value string) bool {
	return value != "" && value == strings.ToLower(value) && len(value) <= 63 && labelValuePattern.MatchString(value)
}
func validNamePart(value string) bool {
	return value != "" && len(value) <= 30 && regexp.MustCompile(`^[a-z]([a-z0-9-]*[a-z0-9])?$`).MatchString(value)
}
func isReservedLabel(key string) bool {
	switch key {
	case managedByLabel, repositoryLabel, workflowLabel, runLabel, jobLabel, runnerLabel, createdLabel, expiryLabel:
		return true
	}
	return false
}
func isNotFound(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "not found") || strings.Contains(text, "404") || strings.Contains(text, "notfound")
}
func normalizeLabelValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !allowed || (r == '-' && lastDash) {
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
			continue
		}
		b.WriteRune(r)
		lastDash = r == '-'
	}
	result := strings.Trim(b.String(), "-_")
	if result == "" {
		return "unknown"
	}
	if len(result) > 63 {
		result = strings.TrimRight(result[:63], "-_")
	}
	if result == "" {
		return "unknown"
	}
	return result
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
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func operationContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}
func stringPtr(value string) *string { return &value }
func newRunnerID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "runner-" + hex.EncodeToString(b), nil
}
func cloneConfig(config Config) Config {
	if config.Labels != nil {
		config.Labels = mapClone(config.Labels)
	}
	return config
}
func mapClone(input map[string]string) map[string]string {
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
	spec.Metadata = mapClone(spec.Metadata)
	return spec
}
