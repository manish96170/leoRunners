package dynamodb

import (
	"context"
	"errors"
	"testing"
	"time"

	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/leo-runners/ci-platform/controller/internal/state"
)

type fakeDynamo struct {
	items        map[string]map[string]types.AttributeValue
	queries      int
	transactions int
}

func newFake() *fakeDynamo { return &fakeDynamo{items: map[string]map[string]types.AttributeValue{}} }
func itemKey(m map[string]types.AttributeValue) string {
	return m["pk"].(*types.AttributeValueMemberS).Value + "|" + m["sk"].(*types.AttributeValueMemberS).Value
}
func cloneAV(m map[string]types.AttributeValue) map[string]types.AttributeValue {
	out := map[string]types.AttributeValue{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
func conditional() error {
	return &smithy.GenericAPIError{Code: "ConditionalCheckFailedException", Message: "condition failed"}
}

func (f *fakeDynamo) GetItem(_ context.Context, in *awsdynamodb.GetItemInput, _ ...func(*awsdynamodb.Options)) (*awsdynamodb.GetItemOutput, error) {
	v := f.items[itemKey(in.Key)]
	if v == nil {
		return &awsdynamodb.GetItemOutput{}, nil
	}
	return &awsdynamodb.GetItemOutput{Item: cloneAV(v)}, nil
}
func (f *fakeDynamo) PutItem(_ context.Context, in *awsdynamodb.PutItemInput, _ ...func(*awsdynamodb.Options)) (*awsdynamodb.PutItemOutput, error) {
	k := itemKey(in.Item)
	if in.ConditionExpression != nil && *in.ConditionExpression == "attribute_not_exists(pk)" && f.items[k] != nil {
		return nil, conditional()
	}
	if in.ConditionExpression != nil && *in.ConditionExpression != "attribute_not_exists(pk)" {
		old, ok := f.items[k]
		if !ok {
			return nil, conditional()
		}
		want := in.ExpressionAttributeValues[":expected"].(*types.AttributeValueMemberN).Value
		got := old["revision"].(*types.AttributeValueMemberN).Value
		if got != want {
			return nil, conditional()
		}
	}
	f.items[k] = cloneAV(in.Item)
	return &awsdynamodb.PutItemOutput{}, nil
}
func (f *fakeDynamo) DeleteItem(_ context.Context, in *awsdynamodb.DeleteItemInput, _ ...func(*awsdynamodb.Options)) (*awsdynamodb.DeleteItemOutput, error) {
	delete(f.items, itemKey(in.Key))
	return &awsdynamodb.DeleteItemOutput{}, nil
}
func (f *fakeDynamo) Query(_ context.Context, in *awsdynamodb.QueryInput, _ ...func(*awsdynamodb.Options)) (*awsdynamodb.QueryOutput, error) {
	f.queries++
	want := in.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
	out := []map[string]types.AttributeValue{}
	for _, v := range f.items {
		if x, ok := v["gsi1pk"].(*types.AttributeValueMemberS); ok && x.Value == want {
			out = append(out, cloneAV(v))
		}
		if x, ok := v["gsi2pk"].(*types.AttributeValueMemberS); ok && x.Value == want {
			out = append(out, cloneAV(v))
		}
		if x, ok := v["pk"].(*types.AttributeValueMemberS); ok && x.Value == want {
			out = append(out, cloneAV(v))
		}
	}
	return &awsdynamodb.QueryOutput{Items: out}, nil
}
func (f *fakeDynamo) Scan(_ context.Context, in *awsdynamodb.ScanInput, _ ...func(*awsdynamodb.Options)) (*awsdynamodb.ScanOutput, error) {
	entity := in.ExpressionAttributeValues[":entity"].(*types.AttributeValueMemberS).Value
	out := []map[string]types.AttributeValue{}
	for _, v := range f.items {
		if x, ok := v["entity"].(*types.AttributeValueMemberS); ok && x.Value == entity {
			out = append(out, cloneAV(v))
		}
	}
	return &awsdynamodb.ScanOutput{Items: out}, nil
}
func (f *fakeDynamo) TransactWriteItems(_ context.Context, in *awsdynamodb.TransactWriteItemsInput, _ ...func(*awsdynamodb.Options)) (*awsdynamodb.TransactWriteItemsOutput, error) {
	f.transactions++
	for _, w := range in.TransactItems {
		if w.Put != nil && w.Put.ConditionExpression != nil && f.items[itemKey(w.Put.Item)] != nil {
			return nil, &smithy.GenericAPIError{Code: "TransactionCanceledException", Message: "condition failed"}
		}
	}
	for _, w := range in.TransactItems {
		if w.Put != nil {
			f.items[itemKey(w.Put.Item)] = cloneAV(w.Put.Item)
		}
		if w.Delete != nil {
			delete(f.items, itemKey(w.Delete.Key))
		}
	}
	return &awsdynamodb.TransactWriteItemsOutput{}, nil
}

func testRepo(t *testing.T) (*Repository, *fakeDynamo) {
	t.Helper()
	f := newFake()
	r, err := New(f, Config{TableName: "state"})
	if err != nil {
		t.Fatal(err)
	}
	return r, f
}
func TestConfigAndConditionalRevisions(t *testing.T) {
	if _, err := New(newFake(), Config{}); err == nil {
		t.Fatal("expected table validation")
	}
	r, _ := testRepo(t)
	ctx := context.Background()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	j := state.Job{ID: "job-1", State: state.JobQueued, CreatedAt: created, ExpiresAt: created.Add(time.Hour), Labels: []string{"linux"}}
	if err := r.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateJob(ctx, j); !errors.Is(err, state.ErrAlreadyExists) {
		t.Fatalf("duplicate create: %v", err)
	}
	got, err := r.GetJob(ctx, j.ID)
	if err != nil || got.Revision != 1 || got.Labels[0] != "linux" {
		t.Fatalf("get: %+v %v", got, err)
	}
	got.Labels[0] = "mutated"
	again, _ := r.GetJob(ctx, j.ID)
	if again.Labels[0] != "linux" {
		t.Fatal("returned labels were not defensive")
	}
	got.State = state.JobRunning
	saved, err := r.SaveJob(ctx, got, 1)
	if err != nil || saved.Revision != 2 {
		t.Fatalf("save: %+v %v", saved, err)
	}
	if _, err := r.SaveJob(ctx, got, 1); !errors.Is(err, state.ErrRevisionConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if _, err := r.SaveJob(ctx, state.Job{ID: "missing", State: state.JobQueued}, 9); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("missing save: %v", err)
	}
}
func TestExpiryAndStateIndexes(t *testing.T) {
	r, f := testRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for _, s := range []state.RunnerState{state.RunnerReady, state.RunnerTerminated} {
		v := state.Runner{ID: string(s), State: s, CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute)}
		if err := r.CreateRunner(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	expired, err := r.ListExpiredRunners(ctx, now)
	if err != nil || len(expired) != 1 || expired[0].State != state.RunnerReady {
		t.Fatalf("expired: %+v %v", expired, err)
	}
	ready, err := r.ListRunners(ctx, state.RunnerReady)
	if err != nil || len(ready) != 1 {
		t.Fatalf("state query: %+v %v", ready, err)
	}
	if f.queries < 2 {
		t.Fatalf("expected indexed queries, got %d", f.queries)
	}
}
func TestLifecycleEventIdempotencyAndDelete(t *testing.T) {
	r, f := testRepo(t)
	ctx := context.Background()
	v := state.LifecycleEvent{ID: "evt-1", IdempotencyKey: "delivery-1", Type: "runner.ready", JobID: "job-1", LeaseID: "lease-1", OccurredAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Data: map[string]string{"status": "ready"}}
	inserted, err := r.InsertLifecycleEvent(ctx, v)
	if err != nil || !inserted {
		t.Fatalf("insert: %v %v", inserted, err)
	}
	inserted, err = r.InsertLifecycleEvent(ctx, v)
	if err != nil || inserted {
		t.Fatalf("idempotent insert: %v %v", inserted, err)
	}
	events, err := r.ListLifecycleEvents(ctx, "job-1", time.Time{})
	if err != nil || len(events) != 1 || events[0].Data["status"] != "ready" {
		t.Fatalf("events: %+v %v", events, err)
	}
	events[0].Data["status"] = "changed"
	again, _ := r.ListLifecycleEvents(ctx, "job-1", time.Time{})
	if again[0].Data["status"] != "ready" {
		t.Fatal("event data was not defensive")
	}
	if err := r.DeleteLifecycleEvent(ctx, v.IdempotencyKey); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ListLifecycleEvents(ctx, "job-1", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if f.transactions != 2 {
		t.Fatalf("expected insert/delete transactions, got %d", f.transactions)
	}
}
func TestContextCancellation(t *testing.T) {
	r, _ := testRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.GetJob(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
