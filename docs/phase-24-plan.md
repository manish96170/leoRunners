# Phase 24 Plan

Phase 24 defines how optional extensions are deployed and operated without
turning registration into implicit authorization. Runtime registration,
tenant enablement, and cloud validation remain separate controls. The
controller stays authoritative for scheduling, provisioning, security,
credentials, lifecycle state, and cleanup.

## Objectives

- Keep extension registration disabled by default at both global and tenant
  scope.
- Require exact tenant allowlists and reviewed configuration changes.
- Provide an observe-only rollout before any production advisory output is
  acted upon.
- Make deployment configuration, limits, ownership, and rollback explicit.
- Validate manifests and policies offline before any live cloud operation.
- Keep live AWS, GCP, and GitHub validation opt-in, bounded, and independent
  from runtime extension registration.

## Registration defaults

Every extension must be present in a versioned registry with a stable ID,
contract version, owner, source or image digest, capabilities, limits, and
runbook. Registration alone does not subscribe the extension to events.

The following defaults are mandatory:

- global extension registration is disabled unless explicitly enabled;
- every tenant starts with an empty extension allowlist;
- observe-only mode is enabled before actionable advisory consumption;
- provider, IAM, GitHub, credential, network, and lifecycle access is absent;
- unknown fields, duplicate IDs, unsupported versions, and wildcard grants
  fail validation.

An enablement change must identify the extension, tenant, approver, reason,
effective time, digest, limits, observation window, and rollback target.

## Tenant allowlists

Tenant policy must contain exact extension IDs and must be evaluated together
with the global policy. An extension receives an event only when both gates
are enabled and the event has a validated tenant identity. Repository, branch,
workflow, job, or arbitrary payload fields must not be used to infer tenant
authorization.

Allowlist changes are reviewed configuration changes, not runtime commands.
They must be auditable, reversible, and tested with a negative tenant-isolation
case. No wildcard tenant or extension grants are permitted.

## Observe-only rollout

Roll out each extension in stages:

1. Register the manifest while global and tenant enablement remain disabled.
2. Validate schema, digest, capabilities, redaction, retention, and bounds
   offline; record the registry hash and configuration diff.
3. Enable one approved tenant in observe-only mode.
4. Send synthetic events and confirm bounded queues, metrics, redacted logs,
   tenant isolation, timeout and panic isolation, and advisory policy results.
5. Confirm that no provider, credential, security, lifecycle, or cleanup
   behavior changes when the extension is enabled.
6. Expand only after the observation window and notification checks pass.

Observe-only means extension output may be recorded as an advisory, but it
cannot authorize or directly perform lifecycle, provider, security, or
credential actions. The core controller must produce the same authoritative
decision with the extension disabled and enabled.

## Configuration variables

Deployment configuration should expose explicit, reviewable values for:

- global extension enablement and observe-only mode;
- the exact extension registry location and expected registry hash;
- tenant-to-extension allowlists;
- queue size, handler timeout, report buffer, shutdown timeout, output count,
  and retention bounds;
- metrics namespace, log destination, environment, and notification routes;
- validation mode, cloud scope, cleanup policy, and live-validation opt-in.

Unset enablement values must resolve to disabled. Secrets, tokens, raw webhook
payloads, and bootstrap material must not be placed in registry or tenant
configuration. Values should be supplied through the deployment secret
mechanism and redacted from logs and validation reports.

## Validation boundary

Runtime registration validation proves that the controller can parse and
authorize a bounded extension configuration. It is an offline or deployment
configuration check and must not provision resources.

Live cloud validation proves connectivity and provider behavior against an
explicit disposable scope. It is a separate, opt-in operation requiring its
own confirmation, credentials, region/project, resource limits, and cleanup
evidence. Passing runtime registration validation does not prove AWS, GCP, or
GitHub access; passing cloud validation does not enable an extension or grant
it controller authority.

Validation must cover duplicate and unsupported registrations, disabled
defaults, exact tenant allowlists, negative tenant isolation, observe-only
policy, bounded dimensions, secrets, queue saturation, timeout, panic,
shutdown, redaction, and rollback configuration. Live validation must never be
required for ordinary unit tests or safe startup.

## Rollback

Disable the tenant allowlist first, then disable the global extension gate if
needed. Restore the last known-good registry, digest, and policy snapshot
through the normal deployment mechanism. Preserve the configuration diff,
registry hash, bounded metrics, redacted logs, advisory decisions, and
validation evidence.

After rollback, verify that no subscription remains active, core scheduling
and cleanup continue, and a bounded synthetic event produces no forbidden
side effect. Any orphaned cloud resource must be handled by the controller's
normal idempotent cleanup path or the separately approved cloud-validation
cleanup path; an extension must never terminate resources directly.

## Exit criteria

- Disabled-by-default behavior is tested at global and tenant scope.
- Exact tenant allowlists and negative isolation tests are recorded.
- Observe-only rollout evidence shows no authoritative lifecycle change.
- Configuration variables, limits, owners, and rollback targets are documented.
- Offline validation is repeatable and contains no cloud side effects.
- Live cloud validation is explicitly gated, bounded, and independently
  evidenced.
- Rollback has been exercised without interrupting core reconciliation or
  cleanup.
