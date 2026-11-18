// Package network contains provider-neutral network requirements and
// capability resolution for ephemeral runners.
package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Profile describes the network exposure and egress contract of a runner.
// It intentionally contains no subnet, firewall, route, or provider fields.
type Profile string

const (
	Public  Profile = "public"
	Private Profile = "private"
)

// EgressMode controls which outbound destinations a runner may reach.
type EgressMode string

const (
	EgressNone         EgressMode = "none"
	EgressRestricted   EgressMode = "restricted"
	EgressInternet     EgressMode = "internet"
	EgressUnrestricted EgressMode = "unrestricted"
)

// Protocol is the transport protocol used by an endpoint requirement.
type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// Endpoint is a network service a workload may require. Host may be a DNS
// name or an IP address; cloud-specific endpoint identifiers do not belong in
// this contract.
type Endpoint struct {
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Protocol Protocol `json:"protocol"`
	Required bool     `json:"required"`
}

// SecurityPosture captures runner exposure requirements independently of how
// a provider implements them.
type SecurityPosture struct {
	InboundPolicy      string `json:"inbound_policy"`
	PublicAddress      bool   `json:"public_address"`
	AllowUntrustedJobs bool   `json:"allow_untrusted_jobs"`
	IsolatedWorkload   bool   `json:"isolated_workload"`
	RequireTLS         bool   `json:"require_tls"`
}

// ProfileSpec is a named, reusable network capability offered to the
// scheduler. Endpoints are copied when registered and returned.
type ProfileSpec struct {
	ID        string            `json:"id"`
	Profile   Profile           `json:"profile"`
	Egress    EgressMode        `json:"egress"`
	Endpoints []Endpoint        `json:"endpoints"`
	Security  SecurityPosture   `json:"security"`
	Metadata  map[string]string `json:"metadata"`
}

// Requirement is the minimum network contract for a runner workload.
type Requirement struct {
	Profile   Profile
	Egress    EgressMode
	Endpoints []Endpoint
	Security  SecurityPosture
}

var (
	ErrInvalidProfile  = errors.New("invalid network profile")
	ErrProfileExists   = errors.New("network profile already exists")
	ErrProfileNotFound = errors.New("network profile not found")
	ErrNoProfile       = errors.New("no network profile satisfies requirement")
	ErrEndpointMissing = errors.New("required network endpoint unavailable")
)

func (p ProfileSpec) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidProfile)
	}
	if p.Profile != Public && p.Profile != Private {
		return fmt.Errorf("%w: profile must be %q or %q", ErrInvalidProfile, Public, Private)
	}
	if !validEgress(p.Egress) {
		return fmt.Errorf("%w: unsupported egress mode %q", ErrInvalidProfile, p.Egress)
	}
	if err := validateSecurity(p.Profile, p.Security, true); err != nil {
		return err
	}
	if err := validateEndpoints(p.Endpoints); err != nil {
		return err
	}
	return nil
}

func (r Requirement) Validate() error {
	if r.Profile != "" && r.Profile != Public && r.Profile != Private {
		return fmt.Errorf("%w: unsupported profile %q", ErrInvalidProfile, r.Profile)
	}
	if r.Egress != "" && !validEgress(r.Egress) {
		return fmt.Errorf("%w: unsupported egress mode %q", ErrInvalidProfile, r.Egress)
	}
	if err := validateSecurity(r.Profile, r.Security, false); err != nil {
		return err
	}
	return validateEndpoints(r.Endpoints)
}

func validEgress(mode EgressMode) bool {
	switch mode {
	case EgressNone, EgressRestricted, EgressInternet, EgressUnrestricted:
		return true
	default:
		return false
	}
}

func validateSecurity(profile Profile, security SecurityPosture, requireInboundPolicy bool) error {
	if requireInboundPolicy && strings.TrimSpace(security.InboundPolicy) == "" {
		return fmt.Errorf("%w: inbound policy is required", ErrInvalidProfile)
	}
	if profile == Private && security.PublicAddress {
		return fmt.Errorf("%w: private profile cannot require a public address", ErrInvalidProfile)
	}
	if security.AllowUntrustedJobs && !security.IsolatedWorkload {
		return fmt.Errorf("%w: untrusted jobs require an isolated workload", ErrInvalidProfile)
	}
	return nil
}

func validateEndpoints(endpoints []Endpoint) error {
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		if strings.TrimSpace(endpoint.Name) == "" || strings.TrimSpace(endpoint.Host) == "" {
			return fmt.Errorf("%w: endpoint name and host are required", ErrInvalidProfile)
		}
		if endpoint.Port < 1 || endpoint.Port > 65535 {
			return fmt.Errorf("%w: endpoint %q has invalid port", ErrInvalidProfile, endpoint.Name)
		}
		if endpoint.Protocol != TCP && endpoint.Protocol != UDP {
			return fmt.Errorf("%w: endpoint %q has unsupported protocol", ErrInvalidProfile, endpoint.Name)
		}
		if net.ParseIP(endpoint.Host) == nil && strings.ContainsAny(endpoint.Host, " /\t\n") {
			return fmt.Errorf("%w: endpoint %q has invalid host", ErrInvalidProfile, endpoint.Name)
		}
		key := strings.ToLower(strings.TrimSpace(endpoint.Name))
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate endpoint %q", ErrInvalidProfile, endpoint.Name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// Resolver selects a profile that can satisfy a workload requirement.
type Resolver interface {
	Resolve(context.Context, Requirement) (ProfileSpec, error)
}

// FakeResolver is a deterministic, concurrency-safe resolver for scheduler
// and provider tests. It models profile availability and endpoint reachability
// without making DNS or cloud API calls.
type FakeResolver struct {
	mu        sync.RWMutex
	profiles  map[string]ProfileSpec
	available map[string]bool
	reachable map[string]bool
	delay     time.Duration
}

func NewFakeResolver(profiles ...ProfileSpec) (*FakeResolver, error) {
	r := &FakeResolver{
		profiles:  make(map[string]ProfileSpec),
		available: make(map[string]bool),
		reachable: make(map[string]bool),
	}
	for _, profile := range profiles {
		if err := r.Register(profile); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *FakeResolver) Register(profile ProfileSpec) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[profile.ID]; ok {
		return fmt.Errorf("%w: %s", ErrProfileExists, profile.ID)
	}
	r.profiles[profile.ID] = cloneProfile(profile)
	r.available[profile.ID] = true
	for _, endpoint := range profile.Endpoints {
		r.reachable[endpointKey(endpoint)] = true
	}
	return nil
}

func (r *FakeResolver) SetAvailable(id string, available bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[id]; !ok {
		return fmt.Errorf("%w: %s", ErrProfileNotFound, id)
	}
	r.available[id] = available
	return nil
}

func (r *FakeResolver) SetEndpointReachable(endpoint Endpoint, reachable bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reachable[endpointKey(endpoint)] = reachable
}

func (r *FakeResolver) SetDelay(delay time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delay = delay
}

func (r *FakeResolver) Resolve(ctx context.Context, requirement Requirement) (ProfileSpec, error) {
	if err := ctx.Err(); err != nil {
		return ProfileSpec{}, err
	}
	if err := requirement.Validate(); err != nil {
		return ProfileSpec{}, err
	}
	r.mu.RLock()
	delay := r.delay
	r.mu.RUnlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ProfileSpec{}, ctx.Err()
		case <-timer.C:
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.profiles))
	for id := range r.profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		profile := r.profiles[id]
		if r.available[id] && satisfies(profile, requirement, r.reachable) {
			return cloneProfile(profile), nil
		}
	}
	return ProfileSpec{}, ErrNoProfile
}

func satisfies(profile ProfileSpec, requirement Requirement, reachable map[string]bool) bool {
	if requirement.Profile != "" && profile.Profile != requirement.Profile {
		return false
	}
	if requirement.Egress != "" && !egressSatisfies(profile.Egress, requirement.Egress) {
		return false
	}
	if requirement.Security.PublicAddress && !profile.Security.PublicAddress {
		return false
	}
	if requirement.Security.AllowUntrustedJobs && !profile.Security.AllowUntrustedJobs {
		return false
	}
	if requirement.Security.IsolatedWorkload && !profile.Security.IsolatedWorkload {
		return false
	}
	if requirement.Security.RequireTLS && !profile.Security.RequireTLS {
		return false
	}
	available := make(map[string]Endpoint, len(profile.Endpoints))
	for _, endpoint := range profile.Endpoints {
		available[endpointKey(endpoint)] = endpoint
	}
	for _, required := range requirement.Endpoints {
		if !required.Required {
			continue
		}
		candidate, ok := available[endpointKey(required)]
		if !ok || !reachable[endpointKey(candidate)] {
			return false
		}
	}
	return true
}

func egressSatisfies(actual, required EgressMode) bool {
	if actual == required {
		return true
	}
	if required == EgressNone {
		return actual == EgressNone
	}
	if actual == EgressUnrestricted {
		return required == EgressInternet
	}
	return false
}

func endpointKey(endpoint Endpoint) string {
	return fmt.Sprintf("%s|%s|%d|%s", strings.ToLower(strings.TrimSpace(endpoint.Name)), strings.ToLower(strings.TrimSpace(endpoint.Host)), endpoint.Port, endpoint.Protocol)
}

func cloneProfile(profile ProfileSpec) ProfileSpec {
	clone := profile
	clone.Endpoints = append([]Endpoint(nil), profile.Endpoints...)
	if profile.Metadata != nil {
		clone.Metadata = make(map[string]string, len(profile.Metadata))
		for key, value := range profile.Metadata {
			clone.Metadata[key] = value
		}
	}
	return clone
}
