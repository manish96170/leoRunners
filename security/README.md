# Security Policy Fixture

`policy.v1.yaml` is the versioned, offline-reviewable security contract for
ephemeral runners. It does not create IAM roles, OIDC providers, firewall
rules, Kubernetes policies, Docker configuration, or cloud resources. Apply
those controls only through separately reviewed infrastructure changes.

## Policy shape

The fixture defines:

- trusted, internal, and untrusted-fork trust boundaries;
- exact GitHub event and repository classification rules;
- controller and runner identity boundaries;
- GitHub App and JIT secret handling;
- AWS and GCP OIDC constraints;
- private network and metadata-service requirements;
- container, Docker, host, and device privilege decisions;
- image and dependency scanning gates;
- deterministic AI advisory restrictions; and
- redacted incident evidence and retention requirements.

Unknown values, missing required fields, expired exceptions, and failed scans
are denied. Security policy is evaluated before capacity selection and cannot
be changed by a workflow, provider, cache backend, or AI response.

## Offline validation

The four Phase 19 files are intentionally self-contained. A portable smoke
check from the repository root is:

```sh
set -eu
test -s security/policy.v1.yaml
test "$(sed -n '1p' security/policy.v1.yaml)" = "apiVersion: security.leorunners.io/v1"
if command -v yq >/dev/null; then yq 'type == "!!map"' security/policy.v1.yaml; fi
! rg -n 'privileged: true|allowAll(IPv4|IPv6): true|0\.0\.0\.0/0|StringLike.*\*' security/policy.v1.yaml
! rg -n 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|BEGIN [A-Z ]*PRIVATE KEY' security/policy.v1.yaml
rg -q 'untrusted-fork' security/policy.v1.yaml
rg -q 'encoded_jit_config' security/policy.v1.yaml
rg -q 'advisory-only' security/policy.v1.yaml
git diff --check -- security docs/security-hardening.md
```

The `yq` check is optional so the smoke command remains useful on a minimal
machine; CI should install and pin a YAML parser and validate the complete
schema. The negative `rg` checks intentionally inspect the policy text for
obvious unsafe literals, not arbitrary generated or external files.

## Review workflow

1. Run the offline checks and inspect the complete diff.
2. Analyze controller and Terraform IAM actions with the approved IAM policy
   tooling; do not infer least privilege from this fixture alone.
3. Validate image digest, scanner evidence, provenance, secret scan, and
   admission policy for the exact release artifact.
4. Run a read-only cloud preflight and one approved workload with cleanup.
5. Store only the redacted evidence fields defined by the policy.

No live validation, policy attachment, credential creation, or resource
mutation is performed by this directory.
