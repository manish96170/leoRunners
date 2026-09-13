package network

import (
	"context"
	"errors"
	"testing"
	"time"
)

var testEndpoint = Endpoint{Name: "source-control", Host: "git.example.test", Port: 443, Protocol: TCP, Required: true}

func testProfile(id string, profile Profile, egress EgressMode) ProfileSpec {
	return ProfileSpec{
		ID: id, Profile: profile, Egress: egress, Endpoints: []Endpoint{testEndpoint},
		Security: SecurityPosture{InboundPolicy: "none", IsolatedWorkload: true, RequireTLS: true},
		Metadata: map[string]string{"owner": "test"},
	}
}

func TestProfileValidation(t *testing.T) {
	valid := testProfile("private-restricted", Private, EgressRestricted)
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	cases := []ProfileSpec{
		{},
		func() ProfileSpec { p := valid; p.Profile = "cloud-specific"; return p }(),
		func() ProfileSpec { p := valid; p.Security.InboundPolicy = ""; return p }(),
		func() ProfileSpec { p := valid; p.Security.PublicAddress = true; return p }(),
		func() ProfileSpec { p := valid; p.Endpoints[0].Port = 0; return p }(),
		func() ProfileSpec { p := valid; p.Endpoints = append(p.Endpoints, testEndpoint); return p }(),
	}
	for _, profile := range cases {
		if err := profile.Validate(); err == nil {
			t.Errorf("invalid profile accepted: %+v", profile)
		}
	}
	invalidUntrusted := valid
	invalidUntrusted.Security.AllowUntrustedJobs = true
	invalidUntrusted.Security.IsolatedWorkload = false
	if err := invalidUntrusted.Validate(); err == nil {
		t.Error("untrusted non-isolated profile accepted")
	}
}

func TestFakeResolverSelectionAndEndpointReachability(t *testing.T) {
	public := testProfile("public", Public, EgressInternet)
	private := testProfile("private", Private, EgressRestricted)
	resolver, err := NewFakeResolver(public, private)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, err := resolver.Resolve(ctx, Requirement{Profile: Private, Egress: EgressRestricted, Endpoints: []Endpoint{testEndpoint}, Security: SecurityPosture{InboundPolicy: "none", IsolatedWorkload: true, RequireTLS: true}})
	if err != nil || got.ID != "private" {
		t.Fatalf("resolve = %q, %v", got.ID, err)
	}
	resolver.SetEndpointReachable(testEndpoint, false)
	if _, err := resolver.Resolve(ctx, Requirement{Profile: Private, Endpoints: []Endpoint{testEndpoint}, Security: SecurityPosture{InboundPolicy: "none"}}); !errors.Is(err, ErrNoProfile) {
		t.Fatalf("unreachable endpoint error = %v", err)
	}
}

func TestFakeResolverAvailabilityAndDefensiveCopies(t *testing.T) {
	profile := testProfile("p", Private, EgressRestricted)
	resolver, err := NewFakeResolver(profile)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolver.Resolve(context.Background(), Requirement{})
	if err != nil {
		t.Fatal(err)
	}
	got.Metadata["owner"] = "changed"
	got.Endpoints[0].Host = "changed"
	again, err := resolver.Resolve(context.Background(), Requirement{})
	if err != nil {
		t.Fatal(err)
	}
	if again.Metadata["owner"] != "test" || again.Endpoints[0].Host != testEndpoint.Host {
		t.Fatal("resolver leaked mutable state")
	}
	if err := resolver.SetAvailable("p", false); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background(), Requirement{}); !errors.Is(err, ErrNoProfile) {
		t.Fatalf("unavailable profile error = %v", err)
	}
}

func TestFakeResolverCancellationAndDelay(t *testing.T) {
	resolver, err := NewFakeResolver(testProfile("p", Private, EgressRestricted))
	if err != nil {
		t.Fatal(err)
	}
	resolver.SetDelay(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := resolver.Resolve(ctx, Requirement{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("resolve error = %v", err)
	}
}

func TestFakeResolverConcurrentResolution(t *testing.T) {
	resolver, err := NewFakeResolver(testProfile("p", Private, EgressRestricted))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	results := make(chan error, 32)
	for i := 0; i < cap(results); i++ {
		go func() {
			got, err := resolver.Resolve(ctx, Requirement{})
			if err == nil && got.ID != "p" {
				err = errors.New("wrong profile")
			}
			results <- err
		}()
	}
	for i := 0; i < cap(results); i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}
