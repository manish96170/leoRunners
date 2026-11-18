package state

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryRepository is a concurrency-safe repository intended for tests and
// local controller runs. Values are copied at the boundary to prevent callers
// from mutating stored slices or event payloads without a revision check.
type MemoryRepository struct {
	mu      sync.RWMutex
	jobs    map[string]Job
	runners map[string]Runner
	leases  map[string]Lease
	events  map[string]LifecycleEvent
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{jobs: make(map[string]Job), runners: make(map[string]Runner), leases: make(map[string]Lease), events: make(map[string]LifecycleEvent)}
}

func (r *MemoryRepository) CreateJob(ctx context.Context, v Job) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if v.ID == "" {
		return ErrInvalidRecord
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[v.ID]; ok {
		return ErrAlreadyExists
	}
	v.Revision = 1
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	v.UpdatedAt = v.CreatedAt
	r.jobs[v.ID] = cloneJob(v)
	return nil
}
func (r *MemoryRepository) GetJob(ctx context.Context, id string) (Job, error) {
	if err := checkContext(ctx); err != nil {
		return Job{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return cloneJob(v), nil
}
func (r *MemoryRepository) SaveJob(ctx context.Context, v Job, expected int64) (Job, error) {
	if err := checkContext(ctx); err != nil {
		return Job{}, err
	}
	if v.ID == "" {
		return Job{}, ErrInvalidRecord
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.jobs[v.ID]
	if !ok {
		if expected != 0 {
			return Job{}, ErrNotFound
		}
		v.Revision = 1
	} else {
		if expected != old.Revision {
			return Job{}, ErrRevisionConflict
		}
		v.Revision = old.Revision + 1
	}
	v.UpdatedAt = time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = old.CreatedAt
	}
	r.jobs[v.ID] = cloneJob(v)
	return cloneJob(v), nil
}
func (r *MemoryRepository) ListJobs(ctx context.Context, states ...JobState) ([]Job, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Job, 0, len(r.jobs))
	for _, v := range r.jobs {
		if matchesJob(v.State, states) {
			out = append(out, cloneJob(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *MemoryRepository) ListExpiredJobs(ctx context.Context, now time.Time) ([]Job, error) {
	all, err := r.ListJobs(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, v := range all {
		if !v.ExpiresAt.IsZero() && !v.ExpiresAt.After(now) && v.State != JobCompleted && v.State != JobCancelled && v.State != JobFailed {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *MemoryRepository) CreateRunner(ctx context.Context, v Runner) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if v.ID == "" {
		return ErrInvalidRecord
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runners[v.ID]; ok {
		return ErrAlreadyExists
	}
	v.Revision = 1
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	v.UpdatedAt = v.CreatedAt
	r.runners[v.ID] = cloneRunner(v)
	return nil
}
func (r *MemoryRepository) GetRunner(ctx context.Context, id string) (Runner, error) {
	if err := checkContext(ctx); err != nil {
		return Runner{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.runners[id]
	if !ok {
		return Runner{}, ErrNotFound
	}
	return cloneRunner(v), nil
}
func (r *MemoryRepository) SaveRunner(ctx context.Context, v Runner, expected int64) (Runner, error) {
	if err := checkContext(ctx); err != nil {
		return Runner{}, err
	}
	if v.ID == "" {
		return Runner{}, ErrInvalidRecord
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.runners[v.ID]
	if !ok {
		if expected != 0 {
			return Runner{}, ErrNotFound
		}
		v.Revision = 1
	} else {
		if expected != old.Revision {
			return Runner{}, ErrRevisionConflict
		}
		v.Revision = old.Revision + 1
	}
	v.UpdatedAt = time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = old.CreatedAt
	}
	r.runners[v.ID] = cloneRunner(v)
	return cloneRunner(v), nil
}
func (r *MemoryRepository) ListRunners(ctx context.Context, states ...RunnerState) ([]Runner, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Runner, 0, len(r.runners))
	for _, v := range r.runners {
		if matchesRunner(v.State, states) {
			out = append(out, cloneRunner(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *MemoryRepository) ListExpiredRunners(ctx context.Context, now time.Time) ([]Runner, error) {
	all, err := r.ListRunners(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, v := range all {
		if !v.ExpiresAt.IsZero() && !v.ExpiresAt.After(now) && v.State != RunnerTerminated {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *MemoryRepository) CreateLease(ctx context.Context, v Lease) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if v.ID == "" {
		return ErrInvalidRecord
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.leases[v.ID]; ok {
		return ErrAlreadyExists
	}
	v.Revision = 1
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	v.UpdatedAt = v.CreatedAt
	r.leases[v.ID] = v
	return nil
}
func (r *MemoryRepository) GetLease(ctx context.Context, id string) (Lease, error) {
	if err := checkContext(ctx); err != nil {
		return Lease{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.leases[id]
	if !ok {
		return Lease{}, ErrNotFound
	}
	return v, nil
}
func (r *MemoryRepository) SaveLease(ctx context.Context, v Lease, expected int64) (Lease, error) {
	if err := checkContext(ctx); err != nil {
		return Lease{}, err
	}
	if v.ID == "" {
		return Lease{}, ErrInvalidRecord
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.leases[v.ID]
	if !ok {
		if expected != 0 {
			return Lease{}, ErrNotFound
		}
		v.Revision = 1
	} else {
		if expected != old.Revision {
			return Lease{}, ErrRevisionConflict
		}
		v.Revision = old.Revision + 1
	}
	v.UpdatedAt = time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = old.CreatedAt
	}
	r.leases[v.ID] = v
	return v, nil
}
func (r *MemoryRepository) ListLeases(ctx context.Context, states ...LeaseState) ([]Lease, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Lease, 0, len(r.leases))
	for _, v := range r.leases {
		if matchesLease(v.State, states) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *MemoryRepository) ListExpiredLeases(ctx context.Context, now time.Time) ([]Lease, error) {
	all, err := r.ListLeases(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, v := range all {
		if !v.ExpiresAt.IsZero() && !v.ExpiresAt.After(now) && v.State != LeaseTerminated {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *MemoryRepository) InsertLifecycleEvent(ctx context.Context, v LifecycleEvent) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}
	key := v.IdempotencyKey
	if key == "" {
		key = v.ID
	}
	if key == "" || v.Type == "" {
		return false, ErrInvalidRecord
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.events[key]
	if ok {
		if sameEvent(old, v) {
			return false, nil
		}
		return false, ErrEventConflict
	}
	v.IdempotencyKey = key
	v.Data = cloneMap(v.Data)
	r.events[key] = v
	return true, nil
}

func (r *MemoryRepository) DeleteLifecycleEvent(ctx context.Context, key string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.events, key)
	return nil
}
func (r *MemoryRepository) ListLifecycleEvents(ctx context.Context, aggregateID string, since time.Time) ([]LifecycleEvent, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LifecycleEvent, 0)
	for _, v := range r.events {
		if (aggregateID == "" || v.JobID == aggregateID || v.LeaseID == aggregateID || v.RunnerID == aggregateID) && (since.IsZero() || !v.OccurredAt.Before(since)) {
			v.Data = cloneMap(v.Data)
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

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
func matchesJob(s JobState, wanted []JobState) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, x := range wanted {
		if s == x {
			return true
		}
	}
	return false
}
func matchesRunner(s RunnerState, wanted []RunnerState) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, x := range wanted {
		if s == x {
			return true
		}
	}
	return false
}
func matchesLease(s LeaseState, wanted []LeaseState) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, x := range wanted {
		if s == x {
			return true
		}
	}
	return false
}
func cloneJob(v Job) Job          { v.Labels = append([]string(nil), v.Labels...); return v }
func cloneRunner(v Runner) Runner { v.Labels = append([]string(nil), v.Labels...); return v }
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
func sameEvent(a, b LifecycleEvent) bool {
	return a.ID == b.ID && a.IdempotencyKey == b.IdempotencyKey && a.Type == b.Type && a.JobID == b.JobID && a.LeaseID == b.LeaseID && a.RunnerID == b.RunnerID && a.Provider == b.Provider && a.OccurredAt.Equal(b.OccurredAt) && mapsEqual(a.Data, b.Data)
}
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
