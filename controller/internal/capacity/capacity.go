// Package capacity contains the controller's provider-independent capacity
// pool registry. Cloud providers implement runner lifecycle; this package
// describes where that lifecycle is allowed to run.
package capacity

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type Ownership string

const (
	CustomerOwned Ownership = "customer-owned"
	Managed       Ownership = "managed"
	Customer      Ownership = CustomerOwned
)

type SelectionMode string

const (
	CustomerFirst SelectionMode = "customer-first"
	ManagedFirst  SelectionMode = "managed-first"
	CustomerOnly  SelectionMode = "customer-only"
	ManagedOnly   SelectionMode = "managed-only"
	Fallback      SelectionMode = "fallback"
)

type Availability string

const (
	Available   Availability = "available"
	Draining    Availability = "draining"
	Unavailable Availability = "unavailable"
)

type Capacity struct {
	MaxRunners   int `json:"max_runners"`
	MaxCPU       int `json:"max_cpu"`
	MaxMemoryGB  int `json:"max_memory_gb"`
	MaxGPU       int `json:"max_gpu"`
	UsedRunners  int `json:"used_runners"`
	UsedCPU      int `json:"used_cpu"`
	UsedMemoryGB int `json:"used_memory_gb"`
	UsedGPU      int `json:"used_gpu"`
}

// CapacityLimits is a descriptive alias for callers that deal with limits
// and usage rather than the registry itself.
type CapacityLimits = Capacity

type Pricing struct {
	Currency      string    `json:"currency"`
	PerRunnerHour float64   `json:"per_runner_hour"`
	PerCPUHour    float64   `json:"per_cpu_hour"`
	StartupCost   float64   `json:"startup_cost"`
	Evidence      string    `json:"evidence"`
	EffectiveAt   time.Time `json:"effective_at"`
}

type Startup struct {
	ExpectedSeconds int    `json:"expected_seconds"`
	P95Seconds      int    `json:"p95_seconds"`
	ImageID         string `json:"image_id"`
	ImageVersion    string `json:"image_version"`
}

type Pool struct {
	ID              string            `json:"id"`
	Ownership       Ownership         `json:"ownership"`
	Provider        string            `json:"provider"`
	Region          string            `json:"region"`
	SecurityProfile string            `json:"security_profile"`
	Availability    Availability      `json:"availability"`
	Labels          []string          `json:"labels"`
	Capacity        Capacity          `json:"capacity"`
	Pricing         Pricing           `json:"pricing"`
	Startup         Startup           `json:"startup"`
	Metadata        map[string]string `json:"metadata"`
}

type Request struct {
	Mode            SelectionMode
	Ownership       Ownership
	Provider        string
	Region          string
	SecurityProfile string
	Labels          []string
	CPU             int
	MemoryGB        int
	GPU             int
}

var (
	ErrInvalidPool    = errors.New("invalid capacity pool")
	ErrPoolExists     = errors.New("capacity pool already exists")
	ErrPoolNotFound   = errors.New("capacity pool not found")
	ErrNoCapacity     = errors.New("no capacity pool available")
	ErrReservation    = errors.New("capacity reservation failed")
	ErrStaleReplica   = errors.New("capacity replica fencing token is stale")
	ErrInvalidReplica = errors.New("invalid capacity replica")
)

func (p Pool) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Provider) == "" || strings.TrimSpace(p.Region) == "" {
		return fmt.Errorf("%w: id, provider, and region are required", ErrInvalidPool)
	}
	if p.Ownership != CustomerOwned && p.Ownership != Managed {
		return fmt.Errorf("%w: ownership must be %q or %q", ErrInvalidPool, CustomerOwned, Managed)
	}
	if p.Availability == "" {
		return fmt.Errorf("%w: availability is required", ErrInvalidPool)
	}
	if p.Availability != Available && p.Availability != Draining && p.Availability != Unavailable {
		return fmt.Errorf("%w: unsupported availability %q", ErrInvalidPool, p.Availability)
	}
	if strings.TrimSpace(p.SecurityProfile) == "" {
		return fmt.Errorf("%w: security profile is required", ErrInvalidPool)
	}
	if err := validateNonNegative("capacity", p.Capacity.MaxRunners, p.Capacity.MaxCPU, p.Capacity.MaxMemoryGB, p.Capacity.MaxGPU, p.Capacity.UsedRunners, p.Capacity.UsedCPU, p.Capacity.UsedMemoryGB, p.Capacity.UsedGPU); err != nil {
		return err
	}
	if err := validateUsage(p.Capacity); err != nil {
		return err
	}
	if p.Startup.ExpectedSeconds < 0 || p.Startup.P95Seconds < 0 || (p.Startup.P95Seconds > 0 && p.Startup.ExpectedSeconds > p.Startup.P95Seconds) {
		return fmt.Errorf("%w: invalid startup metadata", ErrInvalidPool)
	}
	if p.Pricing.PerRunnerHour < 0 || p.Pricing.PerCPUHour < 0 || p.Pricing.StartupCost < 0 || math.IsNaN(p.Pricing.PerRunnerHour) || math.IsNaN(p.Pricing.PerCPUHour) || math.IsNaN(p.Pricing.StartupCost) || math.IsInf(p.Pricing.PerRunnerHour, 0) || math.IsInf(p.Pricing.PerCPUHour, 0) || math.IsInf(p.Pricing.StartupCost, 0) {
		return fmt.Errorf("%w: pricing values cannot be negative", ErrInvalidPool)
	}
	seen := make(map[string]struct{}, len(p.Labels))
	for _, label := range p.Labels {
		label = strings.TrimSpace(label)
		if label == "" {
			return fmt.Errorf("%w: labels cannot be empty", ErrInvalidPool)
		}
		if _, ok := seen[label]; ok {
			return fmt.Errorf("%w: duplicate label %q", ErrInvalidPool, label)
		}
		seen[label] = struct{}{}
	}
	return nil
}

func validateNonNegative(name string, values ...int) error {
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("%w: %s values cannot be negative", ErrInvalidPool, name)
		}
	}
	return nil
}

func validateUsage(c Capacity) error {
	if c.MaxRunners > 0 && c.UsedRunners > c.MaxRunners || c.MaxCPU > 0 && c.UsedCPU > c.MaxCPU || c.MaxMemoryGB > 0 && c.UsedMemoryGB > c.MaxMemoryGB || c.MaxGPU > 0 && c.UsedGPU > c.MaxGPU {
		return fmt.Errorf("%w: usage exceeds capacity limit", ErrInvalidPool)
	}
	return nil
}

type Registry struct {
	mu           sync.RWMutex
	pools        map[string]Pool
	reservations map[string]Reservation
	activeOwner  string
	activeToken  uint64
}

// Reservation records the exact resources consumed by one runner. ID must be
// stable across retries; the ledger makes a repeated reserve idempotent.
type Reservation struct {
	ID           string `json:"id"`
	PoolID       string `json:"pool_id"`
	Owner        string `json:"owner"`
	FencingToken uint64 `json:"fencing_token"`
	CPU          int    `json:"cpu"`
	MemoryGB     int    `json:"memory_gb"`
	GPU          int    `json:"gpu"`
}

// ReservationEvidence is a deterministic, redaction-free recovery snapshot
// suitable for local validation and operator evidence.
type ReservationEvidence struct {
	ActiveOwner  string        `json:"active_owner"`
	ActiveToken  uint64        `json:"active_token"`
	Reservations []Reservation `json:"reservations"`
}

func NewRegistry() *Registry {
	return &Registry{pools: make(map[string]Pool), reservations: make(map[string]Reservation)}
}

// Replica is a controller view backed by the registry's shared reservation
// ledger. A newer fencing token invalidates every older replica.
type Replica struct {
	registry *Registry
	owner    string
	token    uint64
}

func NewReplica(registry *Registry, owner string, token uint64) (*Replica, error) {
	if registry == nil || strings.TrimSpace(owner) == "" || token == 0 {
		return nil, ErrInvalidReplica
	}
	registry.mu.Lock()
	if token > registry.activeToken {
		registry.activeToken = token
		registry.activeOwner = owner
	}
	registry.mu.Unlock()
	return &Replica{registry: registry, owner: owner, token: token}, nil
}

func (r *Replica) fencedLocked() error {
	if r == nil || r.registry == nil || r.token == 0 || r.owner == "" {
		return ErrInvalidReplica
	}
	if r.token != r.registry.activeToken || r.owner != r.registry.activeOwner {
		return ErrStaleReplica
	}
	return nil
}

func (r *Registry) Register(pool Pool) error {
	if err := pool.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.pools[pool.ID]; exists {
		return fmt.Errorf("%w: %s", ErrPoolExists, pool.ID)
	}
	r.pools[pool.ID] = clonePool(pool)
	return nil
}

func (r *Registry) Upsert(pool Pool) error {
	if err := pool.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pools[pool.ID] = clonePool(pool)
	return nil
}

func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pools[id]; !ok {
		return fmt.Errorf("%w: %s", ErrPoolNotFound, id)
	}
	delete(r.pools, id)
	return nil
}

func (r *Registry) Get(id string) (Pool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.pools[id]
	if !ok {
		return Pool{}, fmt.Errorf("%w: %s", ErrPoolNotFound, id)
	}
	return clonePool(p), nil
}

func (r *Registry) List() []Pool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Pool, 0, len(r.pools))
	for _, p := range r.pools {
		result = append(result, clonePool(p))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// Select returns the lexicographically first eligible pool. Ownership mode is
// evaluated before provider/region tie-breakers, making selection reproducible.
func (r *Registry) Select(request Request) (Pool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]Pool, 0, len(r.pools))
	for _, p := range r.pools {
		if matches(p, request) {
			all = append(all, clonePool(p))
		}
	}
	for _, ownership := range ownershipOrder(request.Mode, request.Ownership) {
		candidates := make([]Pool, 0, len(all))
		for _, p := range all {
			if ownership != "" && p.Ownership != ownership {
				continue
			}
			candidates = append(candidates, p)
		}
		sort.Slice(candidates, func(i, j int) bool { return less(candidates[i], candidates[j], request) })
		if len(candidates) > 0 {
			return candidates[0], true
		}
	}
	return Pool{}, false
}

func ownershipOrder(mode SelectionMode, requested Ownership) []Ownership {
	if requested != "" {
		return []Ownership{requested}
	}
	switch mode {
	case CustomerFirst:
		return []Ownership{CustomerOwned, Managed}
	case ManagedFirst:
		return []Ownership{Managed, CustomerOwned}
	case CustomerOnly:
		return []Ownership{CustomerOwned}
	case ManagedOnly:
		return []Ownership{Managed}
	default:
		return []Ownership{""}
	}
}

func matches(p Pool, r Request) bool {
	if p.Availability != Available || (r.Provider != "" && p.Provider != r.Provider) || (r.Region != "" && p.Region != r.Region) || (r.SecurityProfile != "" && p.SecurityProfile != r.SecurityProfile) {
		return false
	}
	if r.CPU < 0 || r.MemoryGB < 0 || r.GPU < 0 || !hasLabels(p.Labels, r.Labels) || !fits(p.Capacity, r.CPU, r.MemoryGB, r.GPU) {
		return false
	}
	return true
}

func fits(c Capacity, cpu, memory, gpu int) bool {
	return (c.MaxRunners == 0 || c.UsedRunners < c.MaxRunners) && (c.MaxCPU == 0 || c.UsedCPU+cpu <= c.MaxCPU) && (c.MaxMemoryGB == 0 || c.UsedMemoryGB+memory <= c.MaxMemoryGB) && (c.MaxGPU == 0 || c.UsedGPU+gpu <= c.MaxGPU)
}

func hasLabels(pool, requested []string) bool {
	set := make(map[string]struct{}, len(pool))
	for _, label := range pool {
		set[label] = struct{}{}
	}
	for _, label := range requested {
		if _, ok := set[label]; !ok {
			return false
		}
	}
	return true
}

func less(a, b Pool, request Request) bool {
	if request.Provider != "" && a.Provider != b.Provider {
		return a.Provider < b.Provider
	}
	if a.Startup.ExpectedSeconds != b.Startup.ExpectedSeconds {
		return a.Startup.ExpectedSeconds < b.Startup.ExpectedSeconds
	}
	if a.Pricing.PerRunnerHour != b.Pricing.PerRunnerHour {
		return a.Pricing.PerRunnerHour < b.Pricing.PerRunnerHour
	}
	if a.Region != b.Region {
		return a.Region < b.Region
	}
	return a.ID < b.ID
}

// Reserve atomically consumes capacity for one runner request.
func (r *Registry) Reserve(id string, cpu, memoryGB, gpu int) error {
	if cpu < 0 || memoryGB < 0 || gpu < 0 {
		return fmt.Errorf("%w: negative reservation", ErrReservation)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pools[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPoolNotFound, id)
	}
	if p.Availability != Available || !fits(p.Capacity, cpu, memoryGB, gpu) {
		return fmt.Errorf("%w: %s", ErrNoCapacity, id)
	}
	p.Capacity.UsedRunners++
	p.Capacity.UsedCPU += cpu
	p.Capacity.UsedMemoryGB += memoryGB
	p.Capacity.UsedGPU += gpu
	r.pools[id] = p
	return nil
}

// ReserveFenced consumes capacity through a replica lease. The reservation ID
// provides retry idempotency and prevents a failover from double-counting work.
func (r *Replica) ReserveFenced(reservationID, poolID string, cpu, memoryGB, gpu int) error {
	if r == nil || r.registry == nil || strings.TrimSpace(reservationID) == "" || strings.TrimSpace(poolID) == "" || cpu < 0 || memoryGB < 0 || gpu < 0 {
		return ErrReservation
	}
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	if err := r.fencedLocked(); err != nil {
		return err
	}
	if existing, ok := r.registry.reservations[reservationID]; ok {
		if existing.Owner == r.owner && existing.FencingToken == r.token && existing.PoolID == poolID && existing.CPU == cpu && existing.MemoryGB == memoryGB && existing.GPU == gpu {
			return nil
		}
		return ErrReservation
	}
	p, ok := r.registry.pools[poolID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPoolNotFound, poolID)
	}
	if p.Availability != Available || !fits(p.Capacity, cpu, memoryGB, gpu) {
		return fmt.Errorf("%w: %s", ErrNoCapacity, poolID)
	}
	p.Capacity.UsedRunners++
	p.Capacity.UsedCPU += cpu
	p.Capacity.UsedMemoryGB += memoryGB
	p.Capacity.UsedGPU += gpu
	r.registry.pools[poolID] = p
	if r.registry.reservations == nil {
		r.registry.reservations = make(map[string]Reservation)
	}
	r.registry.reservations[reservationID] = Reservation{ID: reservationID, PoolID: poolID, Owner: r.owner, FencingToken: r.token, CPU: cpu, MemoryGB: memoryGB, GPU: gpu}
	return nil
}

// ReleaseFenced returns exactly one recorded reservation. A stale replica
// cannot release capacity after a newer owner has taken over.
func (r *Replica) ReleaseFenced(reservationID string) error {
	if strings.TrimSpace(reservationID) == "" {
		return ErrReservation
	}
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	if err := r.fencedLocked(); err != nil {
		return err
	}
	reservation, ok := r.registry.reservations[reservationID]
	if !ok {
		return nil
	}
	p, ok := r.registry.pools[reservation.PoolID]
	if !ok || p.Capacity.UsedRunners < 1 || p.Capacity.UsedCPU < reservation.CPU || p.Capacity.UsedMemoryGB < reservation.MemoryGB || p.Capacity.UsedGPU < reservation.GPU {
		return ErrReservation
	}
	p.Capacity.UsedRunners--
	p.Capacity.UsedCPU -= reservation.CPU
	p.Capacity.UsedMemoryGB -= reservation.MemoryGB
	p.Capacity.UsedGPU -= reservation.GPU
	r.registry.pools[reservation.PoolID] = p
	delete(r.registry.reservations, reservationID)
	return nil
}

// Evidence returns a stable snapshot for recovery validation.
func (r *Registry) Evidence() ReservationEvidence {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reservations := make([]Reservation, 0, len(r.reservations))
	for _, reservation := range r.reservations {
		reservations = append(reservations, reservation)
	}
	sort.Slice(reservations, func(i, j int) bool { return reservations[i].ID < reservations[j].ID })
	return ReservationEvidence{ActiveOwner: r.activeOwner, ActiveToken: r.activeToken, Reservations: reservations}
}

// Release returns capacity. Releasing more than currently reserved is rejected.
func (r *Registry) Release(id string, cpu, memoryGB, gpu int) error {
	if cpu < 0 || memoryGB < 0 || gpu < 0 {
		return fmt.Errorf("%w: negative release", ErrReservation)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pools[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPoolNotFound, id)
	}
	if p.Capacity.UsedRunners < 1 || p.Capacity.UsedCPU < cpu || p.Capacity.UsedMemoryGB < memoryGB || p.Capacity.UsedGPU < gpu {
		return fmt.Errorf("%w: release exceeds usage", ErrReservation)
	}
	p.Capacity.UsedRunners--
	p.Capacity.UsedCPU -= cpu
	p.Capacity.UsedMemoryGB -= memoryGB
	p.Capacity.UsedGPU -= gpu
	r.pools[id] = p
	return nil
}

func clonePool(p Pool) Pool {
	p.Labels = append([]string(nil), p.Labels...)
	if p.Metadata != nil {
		metadata := make(map[string]string, len(p.Metadata))
		for k, v := range p.Metadata {
			metadata[k] = v
		}
		p.Metadata = metadata
	}
	return p
}
