# Phase 19: Security Hardening and Evidence

## Purpose

Phase 19 turns the platform's security requirements into a versioned,
deterministic policy. Ephemeral runners execute repository-controlled code, so
the default trust boundary is untrusted. A runner may receive only the
permissions, network paths, and secrets explicitly approved for its workload.
Security decisions are made before scheduling and cannot be overridden by AI,
job metadata, or a provider adapter.

## Deliverables

1. Publish the policy fixture in [`security/policy.v1.yaml`](../security/policy.v1.yaml).
2. Document the operational controls and evidence contract in
   [`docs/security-hardening.md`](security-hardening.md).
3. Keep controller IAM, runner IAM, GitHub permissions, network policy, image
   policy, and AI policy as separate reviewable boundaries.
4. Validate the fixture and security invariants offline before any cloud or
   GitHub operation.

## Stages

### 19.0 Offline policy validation

- Parse the YAML and confirm the API version, policy version, and deny-by-
  default rules.
- Confirm fork pull requests cannot use trusted repositories, shared caches,
  customer networks, privileged containers, or runner-host credentials.
- Confirm every OIDC trust rule has an issuer, audience, and exact subject or
  an explicitly reviewed immutable claim pattern. No bare wildcard subject is
  valid.
- Confirm JIT material is single-use, short-lived, excluded from state,
  telemetry, tags, logs, image layers, and command-line arguments.
- Confirm security exceptions have an owner, reason, expiry, and evidence
  reference. An absent or expired exception is a denial.

### 19.1 Pull-request isolation

- Classify the event and repository trust before selecting a capacity pool.
- Treat fork `pull_request` jobs as untrusted even when the source branch or
  commit contains workflow changes.
- Do not expose secrets to untrusted jobs. Do not use `pull_request_target` as
  a shortcut for running fork code with base-repository privileges.
- Use a separate runner trust boundary, cache namespace, network profile, and
  cloud account or project when the workload requires stronger isolation.

### 19.2 Identity and secrets

- Use a GitHub App installation token or equivalent short-lived controller
  credential for JIT generation; do not use a PAT baked into an image.
- Use workload identity or OIDC for job-specific cloud access. Bind issuer,
  audience, repository, workflow, and ref or environment claims exactly.
- Keep controller IAM separate from runner IAM. Scope `iam:PassRole`, compute
  actions, state access, and secret access to named resources.
- Deliver the encoded JIT configuration through the protected bootstrap
  contract and erase it after `run.sh --jitconfig` consumes it.

### 19.3 Runtime hardening

- Build from a pinned, scanned image and verify provenance before promotion.
- Run the controller and ordinary containers as non-root with a read-only root
  filesystem, dropped capabilities, and no privilege escalation.
- Deny Docker socket mounts, host PID/network/IPC, host paths, device access,
  and privileged mode by default. Rootless or isolated build machinery is
  required for workloads that genuinely need container builds.
- Require private runner networking, no inbound internet access, IMDSv2 on
  AWS, bounded egress, and a hard expiry with reaping.

### 19.4 Incident evidence and response

- Emit a correlation ID across webhook, job, lease, runner, provider, and
  cleanup events.
- Capture policy version, immutable workload identity, image digest, provider,
  region or zone, security profile, decision, actor, timestamps, and cleanup
  result.
- Redact credentials, authorization headers, JIT data, user data, environment
  values, signed URLs, customer payloads, and raw secret-shaped strings.
- Preserve append-only evidence with restricted access and a documented
  retention period. Record the reason and approver for every exception.
- On suspected compromise, stop new assignments for the affected boundary,
  revoke short-lived credentials, terminate affected runners, invalidate cache
  namespaces, preserve redacted evidence, and require human approval to
  resume.

## Acceptance gates

- Unknown or malformed policy input fails closed before scheduling.
- Fork PRs cannot obtain trusted secrets, trusted cache data, customer-network
  access, or privileged cloud identity.
- No role trust policy accepts an unbounded repository or branch pattern.
- JIT configuration is never durable state, generic metadata, logs, metrics,
  tags, image content, or process arguments.
- Docker and host privileges are denied unless an explicit, time-bounded,
  owner-approved exception is present.
- Image scanning, dependency scanning, secret scanning, and provenance checks
  complete before image promotion.
- AI can classify or explain an event, but cannot authorize provisioning,
  secret access, policy exceptions, cancellation, or cleanup decisions.
- Incident evidence is sufficient to reconstruct the decision and cleanup
  path without exposing sensitive payloads.

## Non-goals

- No automatic IAM, OIDC, firewall, Kubernetes, Docker, or cloud-account
  mutation.
- No claim that an ephemeral VM alone makes arbitrary code trusted.
- No storage of raw workflow logs or secrets in the security evidence stream.
- No replacement of provider, GitHub, container, or cloud-native controls with
  an application-level allowlist.

## References

- GitHub OIDC requires deliberate trust conditions for issuer, audience, and
  subject claims: <https://docs.github.com/en/actions/reference/security/oidc>.
- AWS requires conditions that constrain GitHub OIDC role trust and warns
  against broad subjects: <https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html>.
- Docker documents the host impact of privileged mode and Docker daemon access:
  <https://docs.docker.com/engine/security/>.
