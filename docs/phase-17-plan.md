# Phase 17: Safe Dependency Cache Foundation

## Purpose

Phase 17 defines a cache contract for ephemeral runners without making cache
availability part of scheduling correctness. Caches may reduce dependency
installation and build time, but every job must be able to run from an empty
cache. The first rollout is observe-only: collect evidence, measure cost and
benefit, and do not restore or save shared cache data until the policy gates
have passed.

## Scope

The initial capability inventory covers npm, pnpm, yarn, Cargo, Go modules,
Docker layers, and Terraform plugin or module data. The contract is runtime
neutral and provider neutral. It describes identity, telemetry, retention,
and security boundaries; it does not prescribe a particular CI action or
package-manager implementation.

## Deliverables

1. Document cache key, invalidation, privacy, and S3 lifecycle rules in
   [`docs/caching.md`](caching.md).
2. Publish the versioned default policy in [`cache/config.v1.yaml`](../cache/config.v1.yaml).
3. Keep cache configuration separate from runner scheduling and provider
   capacity decisions.
4. Record cache observations in lifecycle telemetry without putting cache keys,
   repository names, object paths, or secrets into metric labels.

## Rollout stages

### 17.0 Observe-only

- Detect cache opportunities from the workload manifest and workflow evidence.
- Record hit, miss, restore, save, size, and invalidation observations.
- Do not require a cache for assignment, readiness, or job success.
- Do not grant the controller access to cache objects.
- Use synthetic or repository-approved fixtures when validating instrumentation.

### 17.1 Isolated opt-in

- Enable one ecosystem for one approved repository and immutable revision.
- Use a dedicated namespace and prefix for that repository and trust boundary.
- Verify encryption, access logs, retention, deletion, and key invalidation.
- Compare repeated baseline and candidate samples before expanding scope.

### 17.2 Controlled expansion

- Enable only ecosystems with measured benefit and an owner-approved policy.
- Keep cache capacity, object size, and retention bounded.
- Revert to observe-only when hit rate, restore latency, cost, or security
  signals exceed the configured limits.

## Acceptance gates

- A cache miss never prevents a runner from starting or a job from running.
- Cache keys are deterministic, scoped, and derived from immutable inputs.
- A dependency, toolchain, image, or lockfile change invalidates the relevant
  entry rather than silently reusing incompatible data.
- No token, credential, JIT configuration, environment dump, or customer
  secret is uploaded or written into telemetry.
- The S3 bucket is private, encrypted, versioned where required, and has an
  explicit expiration policy and deletion owner.
- Restore and save operations have bounded timeouts and fail open to an empty
  cache for non-security errors.
- Scheduler, lease, provider, and cleanup paths work when the cache service is
  unavailable.
- A redacted evidence record proves policy mode, identity inputs, timings,
  size, outcome, cost classification, and cleanup behavior.

## Non-goals

- No warm runner pool or scheduler reservation based on cache availability.
- No cache correctness guarantee for mutable branches or floating dependency
  versions.
- No cross-tenant cache sharing.
- No automatic S3 bucket, KMS key, IAM role, or cross-account setup.
- No use of cached artifacts as a substitute for source checkout or artifact
  verification.

## Exit evidence

Store a redacted report containing the policy version, workload and commit
identity, ecosystem, mode, key fingerprint, hit or miss outcome, restore and
save durations, size, invalidation reason, provider and region, and cleanup
result. Retain the report according to the platform evidence policy; never
retain raw cache keys or object contents in logs.
