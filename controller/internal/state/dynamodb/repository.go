// Package dynamodb implements state.Repository on an AWS DynamoDB table.
package dynamodb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

const (
	defaultOperationTimeout = 30 * time.Second
	defaultPageSize         = 100
	entityJob               = "job"
	entityRunner            = "runner"
	entityLease             = "lease"
	entityEvent             = "event"
	metaSK                  = "META"
	stateIndex              = "gsi1"
	expiryIndex             = "gsi2"
)

// API is the subset of the AWS SDK client used by Repository. It makes all
// network behavior mockable without introducing an AWS dependency in callers.
type API interface {
	GetItem(context.Context, *awsdynamodb.GetItemInput, ...func(*awsdynamodb.Options)) (*awsdynamodb.GetItemOutput, error)
	PutItem(context.Context, *awsdynamodb.PutItemInput, ...func(*awsdynamodb.Options)) (*awsdynamodb.PutItemOutput, error)
	DeleteItem(context.Context, *awsdynamodb.DeleteItemInput, ...func(*awsdynamodb.Options)) (*awsdynamodb.DeleteItemOutput, error)
	Query(context.Context, *awsdynamodb.QueryInput, ...func(*awsdynamodb.Options)) (*awsdynamodb.QueryOutput, error)
	Scan(context.Context, *awsdynamodb.ScanInput, ...func(*awsdynamodb.Options)) (*awsdynamodb.ScanOutput, error)
	TransactWriteItems(context.Context, *awsdynamodb.TransactWriteItemsInput, ...func(*awsdynamodb.Options)) (*awsdynamodb.TransactWriteItemsOutput, error)
}

// Config describes the table and its two sparse indexes. The index names are
// configurable because existing tables often use deployment-specific names.
type Config struct {
	TableName        string
	StateIndexName   string
	ExpiryIndexName  string
	OperationTimeout time.Duration
	PageSize         int32
}

func (c Config) validate() error {
	if strings.TrimSpace(c.TableName) == "" {
		return errors.New("dynamodb table name is required")
	}
	if c.StateIndexName == "" {
		return errors.New("dynamodb state index name is required")
	}
	if c.ExpiryIndexName == "" {
		return errors.New("dynamodb expiry index name is required")
	}
	if c.OperationTimeout < 0 {
		return errors.New("dynamodb operation timeout cannot be negative")
	}
	if c.PageSize < 0 {
		return errors.New("dynamodb page size cannot be negative")
	}
	return nil
}

func (c *Config) defaults() {
	if c.StateIndexName == "" {
		c.StateIndexName = stateIndex
	}
	if c.ExpiryIndexName == "" {
		c.ExpiryIndexName = expiryIndex
	}
	if c.OperationTimeout == 0 {
		c.OperationTimeout = defaultOperationTimeout
	}
	if c.PageSize == 0 {
		c.PageSize = defaultPageSize
	}
}

// Repository is safe for concurrent use. DynamoDB's SDK retryer handles
// throttling and transient transport failures; this adapter adds no blind
// retries that could duplicate conditional mutations.
type Repository struct {
	client API
	config Config
}

func New(client API, config Config) (*Repository, error) {
	if client == nil {
		return nil, errors.New("dynamodb client is required")
	}
	config.defaults()
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Repository{client: client, config: config}, nil
}

var _ state.Repository = (*Repository)(nil)

func (r *Repository) operation(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	operationCtx, cancel := context.WithTimeout(ctx, r.config.OperationTimeout)
	return operationCtx, cancel, nil
}

type item struct {
	PK string `dynamodbav:"pk"`
	SK string `dynamodbav:"sk"`

	Entity   string `dynamodbav:"entity,omitempty"`
	ID       string `dynamodbav:"id,omitempty"`
	State    string `dynamodbav:"state,omitempty"`
	Revision int64  `dynamodbav:"revision,omitempty"`

	Repository string   `dynamodbav:"repository,omitempty"`
	Workflow   string   `dynamodbav:"workflow,omitempty"`
	RunID      int64    `dynamodbav:"run_id,omitempty"`
	JobNumber  int64    `dynamodbav:"job_number,omitempty"`
	Attempt    int      `dynamodbav:"attempt,omitempty"`
	Labels     []string `dynamodbav:"labels,omitempty"`

	JobID            string `dynamodbav:"job_id,omitempty"`
	LeaseID          string `dynamodbav:"lease_id,omitempty"`
	RunnerID         string `dynamodbav:"runner_id,omitempty"`
	Provider         string `dynamodbav:"provider,omitempty"`
	ProviderInstance string `dynamodbav:"provider_instance_id,omitempty"`
	CapacityPool     string `dynamodbav:"capacity_pool_id,omitempty"`
	CPU              int    `dynamodbav:"cpu,omitempty"`
	MemoryGB         int    `dynamodbav:"memory_gb,omitempty"`
	GPU              int    `dynamodbav:"gpu,omitempty"`
	Region           string `dynamodbav:"region,omitempty"`
	ControllerOwner  string `dynamodbav:"controller_owner,omitempty"`

	CreatedAt string `dynamodbav:"created_at,omitempty"`
	UpdatedAt string `dynamodbav:"updated_at,omitempty"`
	ExpiresAt int64  `dynamodbav:"expires_at,omitempty"`

	IdempotencyKey string            `dynamodbav:"idempotency_key,omitempty"`
	Type           string            `dynamodbav:"type,omitempty"`
	OccurredAt     string            `dynamodbav:"occurred_at,omitempty"`
	Data           map[string]string `dynamodbav:"data,omitempty"`
	Fingerprint    string            `dynamodbav:"fingerprint,omitempty"`
	AggregateIDs   []string          `dynamodbav:"aggregate_ids,omitempty"`
	EventSKs       []string          `dynamodbav:"event_sks,omitempty"`

	GSI1PK string `dynamodbav:"gsi1pk,omitempty"`
	GSI1SK string `dynamodbav:"gsi1sk,omitempty"`
	GSI2PK string `dynamodbav:"gsi2pk,omitempty"`
	GSI2SK string `dynamodbav:"gsi2sk,omitempty"`
}

func timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func expiryUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().Unix()
}

func unixTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}

func parseTimestamp(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, v)
}
func key(entity, id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: entity + "#" + id}, "sk": &types.AttributeValueMemberS{Value: metaSK}}
}
func expiryKey(t time.Time, suffix string) string {
	return fmt.Sprintf("%020d#%s", t.UTC().Unix(), suffix)
}

func (r *Repository) put(ctx context.Context, v item, condition string, values map[string]types.AttributeValue) error {
	av, err := attributevalue.MarshalMap(v)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &awsdynamodb.PutItemInput{TableName: aws.String(r.config.TableName), Item: av, ConditionExpression: aws.String(condition), ExpressionAttributeValues: values})
	return mapDynamoError(err)
}

func (r *Repository) get(ctx context.Context, entity, id string) (item, error) {
	out, err := r.client.GetItem(ctx, &awsdynamodb.GetItemInput{TableName: aws.String(r.config.TableName), Key: key(entity, id), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return item{}, mapDynamoError(err)
	}
	if len(out.Item) == 0 {
		return item{}, state.ErrNotFound
	}
	var v item
	if err := attributevalue.UnmarshalMap(out.Item, &v); err != nil {
		return item{}, fmt.Errorf("decode %s %q: %w", entity, id, err)
	}
	return v, nil
}

func (r *Repository) create(ctx context.Context, v item) error {
	av, err := attributevalue.MarshalMap(v)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &awsdynamodb.PutItemInput{TableName: aws.String(r.config.TableName), Item: av, ConditionExpression: aws.String("attribute_not_exists(pk)")})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ConditionalCheckFailedException" {
			return state.ErrAlreadyExists
		}
		return mapDynamoError(err)
	}
	return nil
}

func (r *Repository) save(ctx context.Context, v item, expected int64) error {
	condition := "attribute_not_exists(pk)"
	values := map[string]types.AttributeValue(nil)
	if expected != 0 {
		condition = "attribute_exists(pk) AND revision = :expected"
		values = map[string]types.AttributeValue{":expected": &types.AttributeValueMemberN{Value: strconv.FormatInt(expected, 10)}}
	}
	av, err := attributevalue.MarshalMap(v)
	if err != nil {
		return err
	}
	_, err = r.client.PutItem(ctx, &awsdynamodb.PutItemInput{TableName: aws.String(r.config.TableName), Item: av, ConditionExpression: aws.String(condition), ExpressionAttributeValues: values})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ConditionalCheckFailedException" {
		if _, getErr := r.get(ctx, v.Entity, v.ID); errors.Is(getErr, state.ErrNotFound) {
			return state.ErrNotFound
		}
		return state.ErrRevisionConflict
	}
	return mapDynamoError(err)
}

func mapDynamoError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ConditionalCheckFailedException":
			return state.ErrRevisionConflict
		case "ResourceNotFoundException":
			return state.ErrNotFound
		}
	}
	return err
}

func validateID(id string) error {
	if strings.TrimSpace(id) == "" {
		return state.ErrInvalidRecord
	}
	return nil
}
func normalizeTimes(created, updated, expires time.Time, creating bool) (time.Time, time.Time, error) {
	if creating && created.IsZero() {
		created = time.Now().UTC()
	}
	if updated.IsZero() {
		if creating {
			updated = created
		} else {
			updated = time.Now().UTC()
		}
	}
	if !expires.IsZero() && !created.IsZero() && expires.Before(created) {
		return time.Time{}, time.Time{}, state.ErrInvalidRecord
	}
	return created.UTC(), updated.UTC(), nil
}

func jobItem(v state.Job) (item, error) {
	if err := validateID(v.ID); err != nil {
		return item{}, err
	}
	created, updated, err := normalizeTimes(v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.Revision == 0)
	if err != nil {
		return item{}, err
	}
	return item{PK: "job#" + v.ID, SK: metaSK, Entity: entityJob, ID: v.ID, State: string(v.State), Revision: v.Revision, Repository: v.Repository, Workflow: v.Workflow, RunID: v.RunID, JobNumber: v.JobID, Attempt: v.Attempt, Labels: append([]string(nil), v.Labels...), ControllerOwner: v.ControllerOwner, CreatedAt: timestamp(created), UpdatedAt: timestamp(updated), ExpiresAt: expiryUnix(v.ExpiresAt), GSI1PK: "job#state#" + string(v.State), GSI1SK: v.ID, GSI2PK: expiryPK(entityJob, v.ExpiresAt), GSI2SK: expiryKey(v.ExpiresAt, v.ID)}, nil
}
func runnerItem(v state.Runner) (item, error) {
	if err := validateID(v.ID); err != nil {
		return item{}, err
	}
	created, updated, err := normalizeTimes(v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.Revision == 0)
	if err != nil {
		return item{}, err
	}
	return item{PK: "runner#" + v.ID, SK: metaSK, Entity: entityRunner, ID: v.ID, JobID: v.JobID, LeaseID: v.LeaseID, Provider: v.Provider, ProviderInstance: v.ProviderInstanceID, CapacityPool: v.CapacityPoolID, CPU: v.CPU, MemoryGB: v.MemoryGB, GPU: v.GPU, Region: v.Region, Labels: append([]string(nil), v.Labels...), State: string(v.State), ControllerOwner: v.ControllerOwner, CreatedAt: timestamp(created), UpdatedAt: timestamp(updated), ExpiresAt: expiryUnix(v.ExpiresAt), Revision: v.Revision, GSI1PK: "runner#state#" + string(v.State), GSI1SK: v.ID, GSI2PK: expiryPK(entityRunner, v.ExpiresAt), GSI2SK: expiryKey(v.ExpiresAt, v.ID)}, nil
}
func leaseItem(v state.Lease) (item, error) {
	if err := validateID(v.ID); err != nil {
		return item{}, err
	}
	created, updated, err := normalizeTimes(v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.Revision == 0)
	if err != nil {
		return item{}, err
	}
	return item{PK: "lease#" + v.ID, SK: metaSK, Entity: entityLease, ID: v.ID, JobID: v.JobID, RunnerID: v.RunnerID, State: string(v.State), ControllerOwner: v.ControllerOwner, CreatedAt: timestamp(created), UpdatedAt: timestamp(updated), ExpiresAt: expiryUnix(v.ExpiresAt), Revision: v.Revision, GSI1PK: "lease#state#" + string(v.State), GSI1SK: v.ID, GSI2PK: expiryPK(entityLease, v.ExpiresAt), GSI2SK: expiryKey(v.ExpiresAt, v.ID)}, nil
}

func (v item) job() (state.Job, error) {
	created, err := parseTimestamp(v.CreatedAt)
	if err != nil {
		return state.Job{}, err
	}
	updated, err := parseTimestamp(v.UpdatedAt)
	if err != nil {
		return state.Job{}, err
	}
	expires := unixTime(v.ExpiresAt)
	return state.Job{ID: v.ID, Repository: v.Repository, Workflow: v.Workflow, RunID: v.RunID, JobID: v.JobNumber, Attempt: v.Attempt, Labels: append([]string(nil), v.Labels...), State: state.JobState(v.State), ControllerOwner: v.ControllerOwner, CreatedAt: created, UpdatedAt: updated, ExpiresAt: expires, Revision: v.Revision}, nil
}
func (v item) runner() (state.Runner, error) {
	created, err := parseTimestamp(v.CreatedAt)
	if err != nil {
		return state.Runner{}, err
	}
	updated, err := parseTimestamp(v.UpdatedAt)
	if err != nil {
		return state.Runner{}, err
	}
	expires := unixTime(v.ExpiresAt)
	return state.Runner{ID: v.ID, JobID: v.JobID, LeaseID: v.LeaseID, Provider: v.Provider, ProviderInstanceID: v.ProviderInstance, CapacityPoolID: v.CapacityPool, CPU: v.CPU, MemoryGB: v.MemoryGB, GPU: v.GPU, Region: v.Region, Labels: append([]string(nil), v.Labels...), State: state.RunnerState(v.State), ControllerOwner: v.ControllerOwner, CreatedAt: created, UpdatedAt: updated, ExpiresAt: expires, Revision: v.Revision}, nil
}
func (v item) lease() (state.Lease, error) {
	created, err := parseTimestamp(v.CreatedAt)
	if err != nil {
		return state.Lease{}, err
	}
	updated, err := parseTimestamp(v.UpdatedAt)
	if err != nil {
		return state.Lease{}, err
	}
	expires := unixTime(v.ExpiresAt)
	return state.Lease{ID: v.ID, JobID: v.JobID, RunnerID: v.RunnerID, State: state.LeaseState(v.State), ControllerOwner: v.ControllerOwner, CreatedAt: created, UpdatedAt: updated, ExpiresAt: expires, Revision: v.Revision}, nil
}

func (r *Repository) CreateJob(ctx context.Context, v state.Job) error {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	x, err := jobItem(v)
	if err != nil {
		return err
	}
	x.Revision = 1
	return r.create(c, x)
}
func (r *Repository) GetJob(ctx context.Context, id string) (state.Job, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return state.Job{}, err
	}
	defer cancel()
	x, err := r.get(c, entityJob, id)
	if err != nil {
		return state.Job{}, err
	}
	return x.job()
}
func (r *Repository) SaveJob(ctx context.Context, v state.Job, expected int64) (state.Job, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return state.Job{}, err
	}
	defer cancel()
	x, err := jobItem(v)
	if err != nil {
		return state.Job{}, err
	}
	x.Revision = expected + 1
	if expected == 0 {
		x.Revision = 1
	}
	if v.CreatedAt.IsZero() && expected != 0 {
		old, e := r.get(c, entityJob, v.ID)
		if e != nil {
			return state.Job{}, e
		}
		x.CreatedAt = old.CreatedAt
	}
	if err := r.save(c, x, expected); err != nil {
		return state.Job{}, err
	}
	return x.job()
}
func (r *Repository) CreateRunner(ctx context.Context, v state.Runner) error {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	x, err := runnerItem(v)
	if err != nil {
		return err
	}
	x.Revision = 1
	return r.create(c, x)
}
func (r *Repository) GetRunner(ctx context.Context, id string) (state.Runner, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return state.Runner{}, err
	}
	defer cancel()
	x, err := r.get(c, entityRunner, id)
	if err != nil {
		return state.Runner{}, err
	}
	return x.runner()
}
func (r *Repository) SaveRunner(ctx context.Context, v state.Runner, expected int64) (state.Runner, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return state.Runner{}, err
	}
	defer cancel()
	x, err := runnerItem(v)
	if err != nil {
		return state.Runner{}, err
	}
	x.Revision = expected + 1
	if expected == 0 {
		x.Revision = 1
	}
	if v.CreatedAt.IsZero() && expected != 0 {
		old, e := r.get(c, entityRunner, v.ID)
		if e != nil {
			return state.Runner{}, e
		}
		x.CreatedAt = old.CreatedAt
	}
	if err := r.save(c, x, expected); err != nil {
		return state.Runner{}, err
	}
	return x.runner()
}
func (r *Repository) CreateLease(ctx context.Context, v state.Lease) error {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	x, err := leaseItem(v)
	if err != nil {
		return err
	}
	x.Revision = 1
	return r.create(c, x)
}
func (r *Repository) GetLease(ctx context.Context, id string) (state.Lease, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return state.Lease{}, err
	}
	defer cancel()
	x, err := r.get(c, entityLease, id)
	if err != nil {
		return state.Lease{}, err
	}
	return x.lease()
}
func (r *Repository) SaveLease(ctx context.Context, v state.Lease, expected int64) (state.Lease, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return state.Lease{}, err
	}
	defer cancel()
	x, err := leaseItem(v)
	if err != nil {
		return state.Lease{}, err
	}
	x.Revision = expected + 1
	if expected == 0 {
		x.Revision = 1
	}
	if v.CreatedAt.IsZero() && expected != 0 {
		old, e := r.get(c, entityLease, v.ID)
		if e != nil {
			return state.Lease{}, e
		}
		x.CreatedAt = old.CreatedAt
	}
	if err := r.save(c, x, expected); err != nil {
		return state.Lease{}, err
	}
	return x.lease()
}

func (r *Repository) query(ctx context.Context, index, pkAttr, skAttr, pk string, before string) ([]item, error) {
	var out []item
	var start map[string]types.AttributeValue
	for {
		input := &awsdynamodb.QueryInput{TableName: aws.String(r.config.TableName), IndexName: aws.String(index), KeyConditionExpression: aws.String(pkAttr + " = :pk"), ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: pk}}, Limit: aws.Int32(r.config.PageSize), ExclusiveStartKey: start}
		if before != "" {
			input.KeyConditionExpression = aws.String(pkAttr + " = :pk AND " + skAttr + " <= :before")
			input.ExpressionAttributeValues[":before"] = &types.AttributeValueMemberS{Value: before}
		}
		res, err := r.client.Query(ctx, input)
		if err != nil {
			return nil, mapDynamoError(err)
		}
		for _, raw := range res.Items {
			var v item
			if err := attributevalue.UnmarshalMap(raw, &v); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if len(res.LastEvaluatedKey) == 0 {
			break
		}
		start = res.LastEvaluatedKey
	}
	return out, nil
}
func (r *Repository) scanEntities(ctx context.Context, entity string) ([]item, error) {
	var out []item
	var start map[string]types.AttributeValue
	for {
		res, err := r.client.Scan(ctx, &awsdynamodb.ScanInput{TableName: aws.String(r.config.TableName), FilterExpression: aws.String("entity = :entity"), ExpressionAttributeValues: map[string]types.AttributeValue{":entity": &types.AttributeValueMemberS{Value: entity}}, Limit: aws.Int32(r.config.PageSize), ExclusiveStartKey: start})
		if err != nil {
			return nil, mapDynamoError(err)
		}
		for _, raw := range res.Items {
			var v item
			if err := attributevalue.UnmarshalMap(raw, &v); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if len(res.LastEvaluatedKey) == 0 {
			break
		}
		start = res.LastEvaluatedKey
	}
	return out, nil
}
func statesJob(v state.Job, wanted []state.JobState) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, x := range wanted {
		if v.State == x {
			return true
		}
	}
	return false
}
func statesRunner(v state.Runner, w []state.RunnerState) bool {
	if len(w) == 0 {
		return true
	}
	for _, x := range w {
		if v.State == x {
			return true
		}
	}
	return false
}
func statesLease(v state.Lease, w []state.LeaseState) bool {
	if len(w) == 0 {
		return true
	}
	for _, x := range w {
		if v.State == x {
			return true
		}
	}
	return false
}
func (r *Repository) ListJobs(ctx context.Context, wanted ...state.JobState) ([]state.Job, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var raw []item
	if len(wanted) == 0 {
		raw, err = r.scanEntities(c, entityJob)
	} else {
		for _, s := range wanted {
			x, e := r.query(c, r.config.StateIndexName, "gsi1pk", "gsi1sk", "job#state#"+string(s), "")
			if e != nil {
				return nil, e
			}
			raw = append(raw, x...)
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]state.Job, 0, len(raw))
	for _, x := range raw {
		v, e := x.job()
		if e != nil {
			return nil, e
		}
		if statesJob(v, wanted) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *Repository) ListRunners(ctx context.Context, wanted ...state.RunnerState) ([]state.Runner, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var raw []item
	if len(wanted) == 0 {
		raw, err = r.scanEntities(c, entityRunner)
	} else {
		for _, s := range wanted {
			x, e := r.query(c, r.config.StateIndexName, "gsi1pk", "gsi1sk", "runner#state#"+string(s), "")
			if e != nil {
				return nil, e
			}
			raw = append(raw, x...)
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]state.Runner, 0, len(raw))
	for _, x := range raw {
		v, e := x.runner()
		if e != nil {
			return nil, e
		}
		if statesRunner(v, wanted) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *Repository) ListLeases(ctx context.Context, wanted ...state.LeaseState) ([]state.Lease, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var raw []item
	if len(wanted) == 0 {
		raw, err = r.scanEntities(c, entityLease)
	} else {
		for _, s := range wanted {
			x, e := r.query(c, r.config.StateIndexName, "gsi1pk", "gsi1sk", "lease#state#"+string(s), "")
			if e != nil {
				return nil, e
			}
			raw = append(raw, x...)
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]state.Lease, 0, len(raw))
	for _, x := range raw {
		v, e := x.lease()
		if e != nil {
			return nil, e
		}
		if statesLease(v, wanted) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Repository) expired(ctx context.Context, entity string, now time.Time) ([]item, error) {
	return r.query(ctx, r.config.ExpiryIndexName, "gsi2pk", "gsi2sk", expiryPK(entity, time.Time{}), expiryKey(now, "~"))
}
func expiryPK(entity string, t time.Time) string { return "expiry#" + entity }
func (r *Repository) ListExpiredJobs(ctx context.Context, now time.Time) ([]state.Job, error) {
	return r.expiredJobs(ctx, now)
}
func (r *Repository) expiredJobs(ctx context.Context, now time.Time) ([]state.Job, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	raw, err := r.expired(c, entityJob, now)
	if err != nil {
		return nil, err
	}
	out := []state.Job{}
	for _, x := range raw {
		v, e := x.job()
		if e != nil {
			return nil, e
		}
		if v.State != state.JobCompleted && v.State != state.JobCancelled && v.State != state.JobFailed {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *Repository) ListExpiredRunners(ctx context.Context, now time.Time) ([]state.Runner, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	raw, err := r.expired(c, entityRunner, now)
	if err != nil {
		return nil, err
	}
	out := []state.Runner{}
	for _, x := range raw {
		v, e := x.runner()
		if e != nil {
			return nil, e
		}
		if v.State != state.RunnerTerminated {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *Repository) ListExpiredLeases(ctx context.Context, now time.Time) ([]state.Lease, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	raw, err := r.expired(c, entityLease, now)
	if err != nil {
		return nil, err
	}
	out := []state.Lease{}
	for _, x := range raw {
		v, e := x.lease()
		if e != nil {
			return nil, e
		}
		if v.State != state.LeaseTerminated {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func aggregateIDs(v state.LifecycleEvent) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range []string{v.JobID, v.LeaseID, v.RunnerID} {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	sort.Strings(out)
	return out
}
func eventItem(v state.LifecycleEvent, aggregate string) (item, error) {
	key := v.IdempotencyKey
	if key == "" {
		key = v.ID
	}
	if key == "" || v.Type == "" {
		return item{}, state.ErrInvalidRecord
	}
	if v.OccurredAt.IsZero() {
		v.OccurredAt = time.Now().UTC()
	}
	x := item{PK: "aggregate#" + aggregate, SK: "event#" + timestamp(v.OccurredAt) + "#" + key, Entity: entityEvent, ID: v.ID, IdempotencyKey: key, Type: v.Type, JobID: v.JobID, LeaseID: v.LeaseID, RunnerID: v.RunnerID, Provider: v.Provider, OccurredAt: timestamp(v.OccurredAt), Data: cloneMap(v.Data), GSI1PK: "events", GSI1SK: timestamp(v.OccurredAt) + "#" + key}
	b, _ := json.Marshal(x)
	sum := sha256.Sum256(b)
	x.Fingerprint = hex.EncodeToString(sum[:])
	return x, nil
}
func cloneMap(v map[string]string) map[string]string {
	if v == nil {
		return nil
	}
	out := make(map[string]string, len(v))
	for k, x := range v {
		out[k] = x
	}
	return out
}
func (v item) event() (state.LifecycleEvent, error) {
	t, err := parseTimestamp(v.OccurredAt)
	if err != nil {
		return state.LifecycleEvent{}, err
	}
	return state.LifecycleEvent{ID: v.ID, IdempotencyKey: v.IdempotencyKey, Type: v.Type, JobID: v.JobID, LeaseID: v.LeaseID, RunnerID: v.RunnerID, Provider: v.Provider, OccurredAt: t, Data: cloneMap(v.Data)}, nil
}
func (r *Repository) InsertLifecycleEvent(ctx context.Context, v state.LifecycleEvent) (bool, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	key := v.IdempotencyKey
	if key == "" {
		key = v.ID
	}
	if key == "" || v.Type == "" {
		return false, state.ErrInvalidRecord
	}
	if v.OccurredAt.IsZero() {
		v.OccurredAt = time.Now().UTC()
	}
	ids := aggregateIDs(v)
	copies := make([]item, 0, len(ids))
	for _, id := range ids {
		x, e := eventItem(v, id)
		if e != nil {
			return false, e
		}
		copies = append(copies, x)
	}
	first, err := r.getEventMarker(c, key)
	if err == nil {
		if first.Fingerprint == copies[0].Fingerprint {
			return false, nil
		}
		return false, state.ErrEventConflict
	}
	if !errors.Is(err, state.ErrNotFound) {
		return false, err
	}
	eventSKs := make([]string, len(copies))
	for i, x := range copies {
		eventSKs[i] = x.SK
	}
	marker := item{PK: "idempotency#" + key, SK: metaSK, Entity: "event_marker", IdempotencyKey: key, Fingerprint: copies[0].Fingerprint, AggregateIDs: ids, EventSKs: eventSKs}
	writes := []types.TransactWriteItem{{Put: &types.Put{TableName: aws.String(r.config.TableName), Item: mustMarshal(marker), ConditionExpression: aws.String("attribute_not_exists(pk)")}}}
	for _, x := range copies {
		writes = append(writes, types.TransactWriteItem{Put: &types.Put{TableName: aws.String(r.config.TableName), Item: mustMarshal(x), ConditionExpression: aws.String("attribute_not_exists(pk)")}})
	}
	_, err = r.client.TransactWriteItems(c, &awsdynamodb.TransactWriteItemsInput{TransactItems: writes})
	if err == nil {
		return true, nil
	}
	if isConditional(err) {
		again, e := r.getEventMarker(c, key)
		if e == nil && again.Fingerprint == copies[0].Fingerprint {
			return false, nil
		}
		if e == nil {
			return false, state.ErrEventConflict
		}
	}
	return false, mapDynamoError(err)
}
func mustMarshal(v item) map[string]types.AttributeValue {
	out, err := attributevalue.MarshalMap(v)
	if err != nil {
		panic(err)
	}
	return out
}
func (r *Repository) getEventMarker(ctx context.Context, key string) (item, error) {
	out, err := r.client.GetItem(ctx, &awsdynamodb.GetItemInput{TableName: aws.String(r.config.TableName), Key: keyMap("idempotency#" + key), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return item{}, mapDynamoError(err)
	}
	if len(out.Item) == 0 {
		return item{}, state.ErrNotFound
	}
	var v item
	if err := attributevalue.UnmarshalMap(out.Item, &v); err != nil {
		return item{}, err
	}
	return v, nil
}
func keyMap(pk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: pk}, "sk": &types.AttributeValueMemberS{Value: metaSK}}
}
func isConditional(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "TransactionCanceledException"
}
func (r *Repository) DeleteLifecycleEvent(ctx context.Context, key string) error {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	m, err := r.getEventMarker(c, key)
	if errors.Is(err, state.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	writes := []types.TransactWriteItem{{Delete: &types.Delete{TableName: aws.String(r.config.TableName), Key: keyMap("idempotency#" + key), ConditionExpression: aws.String("attribute_exists(pk)")}}}
	for i, id := range m.AggregateIDs {
		if i >= len(m.EventSKs) {
			return state.ErrInvalidRecord
		}
		writes = append(writes, types.TransactWriteItem{Delete: &types.Delete{TableName: aws.String(r.config.TableName), Key: map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: "aggregate#" + id}, "sk": &types.AttributeValueMemberS{Value: m.EventSKs[i]}}}})
	}
	_, err = r.client.TransactWriteItems(c, &awsdynamodb.TransactWriteItemsInput{TransactItems: writes})
	return mapDynamoError(err)
}

func (r *Repository) ListLifecycleEvents(ctx context.Context, aggregateID string, since time.Time) ([]state.LifecycleEvent, error) {
	c, cancel, err := r.operation(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var raw []item
	if aggregateID != "" {
		raw, err = r.queryAggregate(c, aggregateID)
	} else {
		raw, err = r.query(c, r.config.StateIndexName, "gsi1pk", "gsi1sk", "events", "")
	}
	if err != nil {
		return nil, err
	}
	out := []state.LifecycleEvent{}
	seen := map[string]bool{}
	for _, x := range raw {
		v, e := x.event()
		if e != nil {
			return nil, e
		}
		if !since.IsZero() && v.OccurredAt.Before(since) {
			continue
		}
		if !seen[v.IdempotencyKey] {
			seen[v.IdempotencyKey] = true
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].IdempotencyKey < out[j].IdempotencyKey
		}
		return out[i].OccurredAt.Before(out[j].OccurredAt)
	})
	return out, nil
}
func (r *Repository) queryAggregate(ctx context.Context, id string) ([]item, error) {
	var out []item
	var start map[string]types.AttributeValue
	for {
		res, err := r.client.Query(ctx, &awsdynamodb.QueryInput{TableName: aws.String(r.config.TableName), KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :prefix)"), ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: "aggregate#" + id}, ":prefix": &types.AttributeValueMemberS{Value: "event#"}}, Limit: aws.Int32(r.config.PageSize), ExclusiveStartKey: start})
		if err != nil {
			return nil, mapDynamoError(err)
		}
		for _, raw := range res.Items {
			var v item
			if err := attributevalue.UnmarshalMap(raw, &v); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if len(res.LastEvaluatedKey) == 0 {
			break
		}
		start = res.LastEvaluatedKey
	}
	return out, nil
}
