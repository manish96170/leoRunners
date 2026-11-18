// Package security evaluates workload trust and capability requests before a
// runner is provisioned. It is deliberately independent of providers, AI,
// and persistence so authorization remains deterministic and auditable.
package security

import (
	"errors"
	"fmt"
	"strings"
)

// TrustClass describes the provenance of the code being executed.
type TrustClass string

const (
	TrustedBranch TrustClass = "trusted-branch"
	InternalPR    TrustClass = "internal-pr"
	ForkPR        TrustClass = "fork-pr"
	Untrusted     TrustClass = "untrusted"
)

// SecurityProfile describes the isolation contract requested by a workload.
type SecurityProfile string

const (
	Isolated   SecurityProfile = "isolated"
	Privileged SecurityProfile = "privileged"
)

// Inputs is the complete authorization input. A true capability means the
// workload is requesting that capability, not that it has already received it.
// No model, AI, or recommendation field belongs in this contract.
type Inputs struct {
	TrustClass       TrustClass
	SecurityProfile  SecurityProfile
	Secrets          bool
	CloudCredentials bool
	DockerPrivileged bool
	HostMounts       bool
	NetworkAccess    bool
	IAM              bool
}

// Request is an alias kept for callers that prefer policy terminology.
type Request = Inputs

// Decision is the stable result of evaluating Inputs. Reasons are ordered by
// a fixed policy order and contain only safe, non-secret text.
type Decision struct {
	Allowed         bool
	Reasons         []string
	TrustClass      TrustClass
	SecurityProfile SecurityProfile
}

var (
	ErrInvalidInput = errors.New("invalid security policy input")
	ErrDenied       = errors.New("security policy denied workload")
)

var validTrust = map[TrustClass]struct{}{
	TrustedBranch: {}, InternalPR: {}, ForkPR: {}, Untrusted: {},
}

var validProfile = map[SecurityProfile]struct{}{
	Isolated: {}, Privileged: {},
}

// Validate checks the enum portion of an authorization request. Capability
// booleans need no validation because false means "not requested".
func (input Inputs) Validate() error {
	if _, ok := validTrust[input.TrustClass]; !ok {
		return fmt.Errorf("%w: unknown trust class %q", ErrInvalidInput, input.TrustClass)
	}
	if _, ok := validProfile[input.SecurityProfile]; !ok {
		return fmt.Errorf("%w: unknown security profile %q", ErrInvalidInput, input.SecurityProfile)
	}
	return nil
}

// Evaluate returns the same decision for the same input and never consults
// external state. A decision is denied whenever validation fails or a rule is
// violated.
func Evaluate(input Inputs) Decision {
	decision := Decision{Allowed: true, TrustClass: input.TrustClass, SecurityProfile: input.SecurityProfile}
	if err := input.Validate(); err != nil {
		return deny(decision, strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": "))
	}

	// Reasons are appended in this order intentionally. Do not reorder them
	// based on input map iteration or capability discovery.
	if input.SecurityProfile == Privileged && input.TrustClass != TrustedBranch {
		decision = deny(decision, "privileged profile requires trusted branch code")
	}
	if input.Secrets && input.TrustClass != TrustedBranch && input.TrustClass != InternalPR {
		decision = deny(decision, "secrets are unavailable to fork and untrusted code")
	}
	if input.CloudCredentials && input.TrustClass != TrustedBranch {
		decision = deny(decision, "cloud credentials require trusted branch code")
	}
	if input.DockerPrivileged && input.SecurityProfile != Privileged {
		decision = deny(decision, "Docker privileged mode requires the privileged profile")
	}
	if input.DockerPrivileged && input.TrustClass != TrustedBranch {
		decision = deny(decision, "Docker privileged mode requires trusted branch code")
	}
	if input.HostMounts && input.SecurityProfile != Privileged {
		decision = deny(decision, "host mounts require the privileged profile")
	}
	if input.HostMounts && input.TrustClass != TrustedBranch {
		decision = deny(decision, "host mounts require trusted branch code")
	}
	if input.IAM && input.TrustClass != TrustedBranch {
		decision = deny(decision, "IAM access requires trusted branch code")
	}
	if input.IAM && input.SecurityProfile != Privileged {
		decision = deny(decision, "IAM access requires the privileged profile")
	}
	if input.NetworkAccess && input.SecurityProfile == Privileged && input.TrustClass != TrustedBranch {
		decision = deny(decision, "privileged network access requires trusted branch code")
	}
	return decision
}

// Authorize is the error-returning form for provisioning call sites.
func Authorize(input Inputs) error {
	if err := input.Validate(); err != nil {
		return err
	}
	decision := Evaluate(input)
	if !decision.Allowed {
		return fmt.Errorf("%w: %s", ErrDenied, strings.Join(decision.Reasons, "; "))
	}
	return nil
}

func deny(decision Decision, reason string) Decision {
	decision.Allowed = false
	decision.Reasons = append(decision.Reasons, reason)
	return decision
}
