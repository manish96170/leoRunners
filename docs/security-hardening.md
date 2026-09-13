# Security Hardening

The security policy in [`security/policy.v1.yaml`](../security/policy.v1.yaml)
is the normative Phase 19 fixture. It is intentionally conservative: a
missing classification, claim, scan, exception, or evidence field is a deny or
preflight failure. Provider implementations may add controls, but they may
not weaken this policy.

## Fork pull requests

Fork code is untrusted code. A `pull_request` from a fork gets a dedicated
untrusted trust boundary, no repository or environment secrets, no trusted
cache restore, no customer-network route, no controller credentials, and no
privileged cloud identity. The source branch, workflow file, and labels are
attacker-controlled inputs and must not change that classification.

Never use `pull_request_target` to run checkouted fork code with base-repository
privileges. If a maintainer needs a privileged validation, use a separately
reviewed workflow and explicit human approval after inspecting the revision.

## IAM and OIDC

The controller role and runner role have different owners and policies. The
controller can perform only the provider, state, and GitHub operations needed
to coordinate runners. A runner receives no controller credentials and no
permission to create IAM policies, pass arbitrary roles, change networking, or
read shared state.

For GitHub Actions OIDC, trust must constrain the issuer, `aud` claim, and an
exact `sub` or an explicitly reviewed immutable subject. Prefer a specific
repository plus protected environment, workflow, and ref. Do not use
`StringLike` with an organization-wide or repository-wide wildcard as a
substitute for review. Scope the resulting role permissions separately from
the trust policy, and use short session durations.

AWS `iam:PassRole` is restricted to named runner role ARNs and the intended
compute service. Any wildcard resource for `iam:PassRole`, unrestricted
`sts:AssumeRole`, or broad state-table access is a release blocker.

## JIT secrets

The GitHub `encoded_jit_config` value is a single-use bootstrap secret. The
controller obtains it just before provisioning, passes it through the
protected file or descriptor contract, and removes the temporary material
after the runner process starts. It must not appear in state records, generic
provider metadata, instance tags, user-data logs, command-line arguments,
metrics, traces, benchmark reports, crash dumps, or support bundles.

Webhook secrets and GitHub App credentials belong in the deployment secret
manager. They are injected at runtime, rotated independently of images, and
redacted recursively in structured logs. A failed launch or registration
attempt must terminate the instance and record only a redacted failure reason.

## Docker and host privileges

The controller image runs as non-root with a read-only root filesystem and
dropped capabilities. For workload containers, deny `--privileged`, Docker
socket mounts, host filesystem mounts, host PID/network/IPC, added Linux
capabilities, device mappings, and host user namespaces by default. A Docker
daemon is a host control surface: access to its socket can be equivalent to
host root.

If a workload truly requires Docker builds, use an isolated builder boundary
with rootless Docker or a separately hardened VM/pool. The exception must
identify the owner, workload, reason, allowed duration, image or digest, and
rollback evidence. It must never silently change fork policy.

## Network and image controls

Runners have no inbound internet access and use the approved network profile
for GitHub, package registries, and container registries. Unknown destinations
fail closed. AWS runners require IMDSv2; metadata access is not a substitute
for workload identity. Private subnets, endpoint or proxy paths, and egress
costs are recorded as evidence.

Images are immutable release inputs. Before promotion, run vulnerability,
dependency, secret, license, and provenance checks; record the image digest,
scanner versions, policy version, and result. Do not bake GitHub credentials,
JIT data, cloud access keys, SSH keys, or customer secrets into an image.

## AI boundary

AI is disabled by default and remains advisory when enabled. It may summarize
redacted failures or suggest a human-reviewed explanation. A deterministic
policy gate rejects AI requests to provision, terminate, retry, rerun, cancel,
grant credentials, alter network or IAM policy, approve an exception, or
declare a security event resolved.

## Incident evidence

Every security decision gets a correlation ID and an append-only redacted
record. Minimum fields are the policy version, workload revision, repository
and workflow fingerprints, event type, trust boundary, provider, region or
zone, image digest, network profile, decision, reason code, actor, timestamps,
exception reference when present, and cleanup result.

Evidence must support reconstruction without retaining raw payloads. Replace
tokens, headers, JIT values, signed URLs, user data, environment values,
customer content, and full private hostnames with typed redaction markers or
one-way fingerprints. Restrict access, define retention, and preserve the
original evidence hash so tampering is detectable.

## Offline checks

From the repository root, these checks require no cloud credentials and must
pass before live validation:

```sh
test "$(sed -n '1p' security/policy.v1.yaml)" = "apiVersion: security.leorunners.io/v1"
rg -n 'fork|oidc|jit|docker|network|scan|ai|incident' security/policy.v1.yaml docs/security-hardening.md
! rg -n 'privileged: true|allowAll(IPv4|IPv6): true|0\.0\.0\.0/0|StringLike.*\*' security/policy.v1.yaml
! rg -n 'AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|BEGIN [A-Z ]*PRIVATE KEY' security/policy.v1.yaml
git diff --check -- security docs/security-hardening.md
```

The checks are guards, not a replacement for YAML parsing, IAM policy
analysis, image scanning, cluster admission policy, or a human security
review. Use the repository's YAML parser or CI validator in addition to these
portable checks.

## References

- <https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-cloud-providers>
- <https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html>
- <https://docs.docker.com/engine/security/>
