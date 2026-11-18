package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const fileStateVersion = 1

// FileRepository persists the complete state repository in one JSON snapshot.
// Mutations are serialized and committed with a temporary-file rename, so a
// reader sees either the previous complete snapshot or the next complete one.
type FileRepository struct {
	mu   sync.Mutex
	path string
	mem  *MemoryRepository
}

type fileSnapshot struct {
	Version int                       `json:"version"`
	Jobs    map[string]Job            `json:"jobs"`
	Runners map[string]Runner         `json:"runners"`
	Leases  map[string]Lease          `json:"leases"`
	Events  map[string]LifecycleEvent `json:"events"`
}

// NewFileRepository opens path, recovering from path+".bak" if the primary
// snapshot is missing or invalid. A missing repository starts empty.
func NewFileRepository(path string) (*FileRepository, error) {
	if path == "" {
		return nil, fmt.Errorf("state file path is empty")
	}
	r := &FileRepository{path: path, mem: NewMemoryRepository()}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *FileRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.path)
	if err == nil {
		s, decodeErr := decodeSnapshot(data)
		if decodeErr == nil {
			if err := os.Chmod(r.path, 0600); err != nil {
				return fmt.Errorf("restrict state file permissions: %w", err)
			}
			r.restore(s)
			return nil
		}
		if backup, backupErr := os.ReadFile(r.path + ".bak"); backupErr == nil {
			if recovered, recoverErr := decodeSnapshot(backup); recoverErr == nil {
				r.restore(recovered)
				return r.writeLocked(backup)
			}
		}
		return fmt.Errorf("load state file %q: %w", r.path, decodeErr)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read state file %q: %w", r.path, err)
	}
	if backup, backupErr := os.ReadFile(r.path + ".bak"); backupErr == nil {
		recovered, recoverErr := decodeSnapshot(backup)
		if recoverErr != nil {
			return fmt.Errorf("recover state file %q: %w", r.path, recoverErr)
		}
		r.restore(recovered)
		return r.writeLocked(backup)
	}
	return r.writeLocked(nil)
}

func decodeSnapshot(data []byte) (fileSnapshot, error) {
	var s fileSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return fileSnapshot{}, err
	}
	if s.Version != fileStateVersion {
		return fileSnapshot{}, fmt.Errorf("unsupported state file version %d", s.Version)
	}
	if s.Jobs == nil {
		s.Jobs = make(map[string]Job)
	}
	if s.Runners == nil {
		s.Runners = make(map[string]Runner)
	}
	if s.Leases == nil {
		s.Leases = make(map[string]Lease)
	}
	if s.Events == nil {
		s.Events = make(map[string]LifecycleEvent)
	}
	for id, v := range s.Jobs {
		if id == "" || v.ID == "" || id != v.ID {
			return fileSnapshot{}, ErrInvalidRecord
		}
	}
	for id, v := range s.Runners {
		if id == "" || v.ID == "" || id != v.ID {
			return fileSnapshot{}, ErrInvalidRecord
		}
	}
	for id, v := range s.Leases {
		if id == "" || v.ID == "" || id != v.ID {
			return fileSnapshot{}, ErrInvalidRecord
		}
	}
	for key, v := range s.Events {
		if key == "" || v.IdempotencyKey == "" || key != v.IdempotencyKey || v.Type == "" {
			return fileSnapshot{}, ErrInvalidRecord
		}
	}
	return s, nil
}

func (r *FileRepository) snapshot() fileSnapshot {
	r.mem.mu.RLock()
	defer r.mem.mu.RUnlock()
	s := fileSnapshot{
		Version: fileStateVersion,
		Jobs:    make(map[string]Job, len(r.mem.jobs)),
		Runners: make(map[string]Runner, len(r.mem.runners)),
		Leases:  make(map[string]Lease, len(r.mem.leases)),
		Events:  make(map[string]LifecycleEvent, len(r.mem.events)),
	}
	for id, v := range r.mem.jobs {
		s.Jobs[id] = cloneJob(v)
	}
	for id, v := range r.mem.runners {
		s.Runners[id] = cloneRunner(v)
	}
	for id, v := range r.mem.leases {
		s.Leases[id] = v
	}
	for key, v := range r.mem.events {
		v.Data = cloneMap(v.Data)
		s.Events[key] = v
	}
	return s
}

func (r *FileRepository) restore(s fileSnapshot) {
	r.mem.mu.Lock()
	defer r.mem.mu.Unlock()
	r.mem.jobs, r.mem.runners, r.mem.leases, r.mem.events = s.Jobs, s.Runners, s.Leases, s.Events
}

func (r *FileRepository) writeLocked(previous []byte) error {
	s := r.snapshot()
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if previous == nil {
		previous, _ = os.ReadFile(r.path)
	}
	if previous != nil {
		if err := atomicWrite(r.path+".bak", previous); err != nil {
			return fmt.Errorf("write state backup: %w", err)
		}
	}
	if err := atomicWrite(r.path, data); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (r *FileRepository) mutate(ctx context.Context, fn func() error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.snapshot()
	if err := fn(); err != nil {
		return err
	}
	if err := r.writeLocked(nil); err != nil {
		r.restore(before)
		return err
	}
	return nil
}

func (r *FileRepository) saveJob(ctx context.Context, fn func() (Job, error)) (Job, error) {
	if err := checkContext(ctx); err != nil {
		return Job{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.snapshot()
	v, err := fn()
	if err != nil {
		return Job{}, err
	}
	if err := r.writeLocked(nil); err != nil {
		r.restore(before)
		return Job{}, err
	}
	return v, nil
}

func (r *FileRepository) saveRunner(ctx context.Context, fn func() (Runner, error)) (Runner, error) {
	if err := checkContext(ctx); err != nil {
		return Runner{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.snapshot()
	v, err := fn()
	if err != nil {
		return Runner{}, err
	}
	if err := r.writeLocked(nil); err != nil {
		r.restore(before)
		return Runner{}, err
	}
	return v, nil
}

func (r *FileRepository) saveLease(ctx context.Context, fn func() (Lease, error)) (Lease, error) {
	if err := checkContext(ctx); err != nil {
		return Lease{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.snapshot()
	v, err := fn()
	if err != nil {
		return Lease{}, err
	}
	if err := r.writeLocked(nil); err != nil {
		r.restore(before)
		return Lease{}, err
	}
	return v, nil
}

func (r *FileRepository) CreateJob(ctx context.Context, v Job) error {
	return r.mutate(ctx, func() error { return r.mem.CreateJob(ctx, v) })
}
func (r *FileRepository) GetJob(ctx context.Context, id string) (Job, error) {
	return r.mem.GetJob(ctx, id)
}
func (r *FileRepository) SaveJob(ctx context.Context, v Job, expected int64) (Job, error) {
	return r.saveJob(ctx, func() (Job, error) { return r.mem.SaveJob(ctx, v, expected) })
}
func (r *FileRepository) ListJobs(ctx context.Context, states ...JobState) ([]Job, error) {
	return r.mem.ListJobs(ctx, states...)
}
func (r *FileRepository) ListExpiredJobs(ctx context.Context, now time.Time) ([]Job, error) {
	return r.mem.ListExpiredJobs(ctx, now)
}

func (r *FileRepository) CreateRunner(ctx context.Context, v Runner) error {
	return r.mutate(ctx, func() error { return r.mem.CreateRunner(ctx, v) })
}
func (r *FileRepository) GetRunner(ctx context.Context, id string) (Runner, error) {
	return r.mem.GetRunner(ctx, id)
}
func (r *FileRepository) SaveRunner(ctx context.Context, v Runner, expected int64) (Runner, error) {
	return r.saveRunner(ctx, func() (Runner, error) { return r.mem.SaveRunner(ctx, v, expected) })
}
func (r *FileRepository) ListRunners(ctx context.Context, states ...RunnerState) ([]Runner, error) {
	return r.mem.ListRunners(ctx, states...)
}
func (r *FileRepository) ListExpiredRunners(ctx context.Context, now time.Time) ([]Runner, error) {
	return r.mem.ListExpiredRunners(ctx, now)
}

func (r *FileRepository) CreateLease(ctx context.Context, v Lease) error {
	return r.mutate(ctx, func() error { return r.mem.CreateLease(ctx, v) })
}
func (r *FileRepository) GetLease(ctx context.Context, id string) (Lease, error) {
	return r.mem.GetLease(ctx, id)
}
func (r *FileRepository) SaveLease(ctx context.Context, v Lease, expected int64) (Lease, error) {
	return r.saveLease(ctx, func() (Lease, error) { return r.mem.SaveLease(ctx, v, expected) })
}
func (r *FileRepository) ListLeases(ctx context.Context, states ...LeaseState) ([]Lease, error) {
	return r.mem.ListLeases(ctx, states...)
}
func (r *FileRepository) ListExpiredLeases(ctx context.Context, now time.Time) ([]Lease, error) {
	return r.mem.ListExpiredLeases(ctx, now)
}

func (r *FileRepository) InsertLifecycleEvent(ctx context.Context, v LifecycleEvent) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.snapshot()
	inserted, err := r.mem.InsertLifecycleEvent(ctx, v)
	if err != nil {
		return false, err
	}
	if inserted {
		if err := r.writeLocked(nil); err != nil {
			r.restore(before)
			return false, err
		}
	}
	return inserted, nil
}
func (r *FileRepository) DeleteLifecycleEvent(ctx context.Context, key string) error {
	return r.mutate(ctx, func() error { return r.mem.DeleteLifecycleEvent(ctx, key) })
}
func (r *FileRepository) ListLifecycleEvents(ctx context.Context, aggregateID string, since time.Time) ([]LifecycleEvent, error) {
	return r.mem.ListLifecycleEvents(ctx, aggregateID, since)
}

var _ Repository = (*FileRepository)(nil)
