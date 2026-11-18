package security

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTrustedBranchPrivilegedCapabilitiesAllowed(t *testing.T) {
	decision := Evaluate(Inputs{
		TrustClass: TrustedBranch, SecurityProfile: Privileged,
		Secrets: true, CloudCredentials: true, DockerPrivileged: true,
		HostMounts: true, NetworkAccess: true, IAM: true,
	})
	if !decision.Allowed || len(decision.Reasons) != 0 {
		t.Fatalf("trusted privileged workload denied: %+v", decision)
	}
}

func TestInternalPRAllowsSecretsButNotHighRiskCapabilities(t *testing.T) {
	decision := Evaluate(Inputs{TrustClass: InternalPR, SecurityProfile: Isolated, Secrets: true, NetworkAccess: true})
	if !decision.Allowed {
		t.Fatalf("internal PR with secrets should be allowed: %+v", decision)
	}
	decision = Evaluate(Inputs{TrustClass: InternalPR, SecurityProfile: Isolated, CloudCredentials: true, IAM: true})
	if decision.Allowed || len(decision.Reasons) != 3 {
		t.Fatalf("internal PR high-risk capabilities were not denied: %+v", decision)
	}
}

func TestForkAndUntrustedAreDeniedSensitiveCapabilities(t *testing.T) {
	for _, trust := range []TrustClass{ForkPR, Untrusted} {
		decision := Evaluate(Inputs{TrustClass: trust, SecurityProfile: Isolated, Secrets: true, CloudCredentials: true, DockerPrivileged: true, HostMounts: true, IAM: true})
		if decision.Allowed || len(decision.Reasons) != 8 {
			t.Errorf("trust %q decision = %+v", trust, decision)
		}
	}
}

func TestPrivilegedProfileRequiresTrustedBranch(t *testing.T) {
	decision := Evaluate(Inputs{TrustClass: InternalPR, SecurityProfile: Privileged})
	if decision.Allowed || !contains(decision.Reasons, "privileged profile requires trusted branch code") {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestIsolatedProfileRejectsDockerAndHostMounts(t *testing.T) {
	decision := Evaluate(Inputs{TrustClass: TrustedBranch, SecurityProfile: Isolated, DockerPrivileged: true, HostMounts: true})
	if decision.Allowed || len(decision.Reasons) != 2 {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestUnknownValuesFailClosed(t *testing.T) {
	for _, input := range []Inputs{{TrustClass: "future", SecurityProfile: Isolated}, {TrustClass: TrustedBranch, SecurityProfile: "future"}} {
		decision := Evaluate(input)
		if decision.Allowed || len(decision.Reasons) != 1 {
			t.Errorf("decision = %+v", decision)
		}
		if err := Authorize(input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Authorize error = %v", err)
		}
	}
}

func TestReasonsAreDeterministic(t *testing.T) {
	input := Inputs{TrustClass: ForkPR, SecurityProfile: Privileged, Secrets: true, CloudCredentials: true, DockerPrivileged: true, HostMounts: true, NetworkAccess: true, IAM: true}
	want := Evaluate(input)
	for i := 0; i < 100; i++ {
		if got := Evaluate(input); !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d decision changed: got %+v want %+v", i, got, want)
		}
	}
}

func TestAuthorizeErrorContainsAllSafeReasons(t *testing.T) {
	err := Authorize(Inputs{TrustClass: ForkPR, SecurityProfile: Isolated, Secrets: true, CloudCredentials: true})
	if err == nil || !errors.Is(err, ErrDenied) || !strings.Contains(err.Error(), "secrets") || !strings.Contains(err.Error(), "cloud credentials") {
		t.Fatalf("error = %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
