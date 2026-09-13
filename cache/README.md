# Cache Policy Fixtures

This directory contains the versioned cache policy contract for ephemeral
CI runners. `config.v1.yaml` is a safe starting point for validation and
observe-only measurement; it does not provision S3, grant IAM permissions, or
change scheduler behavior.

## Contract

- All listed ecosystems start in `observe-only` mode.
- Cache misses and backend failures fall back to a clean job.
- Keys are scoped by tenant, repository, ecosystem, immutable dependency
  inputs, toolchain, architecture, and cache schema.
- Raw keys, object paths, dependency contents, tokens, and credentials are
  prohibited from logs and lifecycle telemetry.
- S3 is optional and must be private, encrypted, prefix-scoped, and bounded by
  lifecycle expiration.
- Cache availability is never a scheduler, lease, readiness, or termination
  dependency.

## Safe update process

1. Copy the current configuration to a new version rather than mutating a
   version used by a benchmark or production run.
2. Add an owner, approved repository scope, evidence references, and retention
   limits before enabling an ecosystem.
3. Validate that key inputs and invalidation triggers cover the package manager,
   lockfile, toolchain, image, architecture, and cache schema.
4. Run observe-only measurements with repeated baseline samples.
5. Review privacy, IAM, S3 lifecycle, cost, and cleanup evidence.
6. Enable one ecosystem and trust boundary at a time, with an immediate
   configuration rollback path.

## Evidence

Record policy version, workload revision, ecosystem, mode, opaque key
fingerprint, outcome, hit or miss, restore/save durations, size, invalidation
reason, provider, region, and cleanup result. Redact raw cache keys, object
URLs, signed requests, environment values, and customer data.

The cache policy is advisory. A workload must remain runnable when the cache is
empty or unavailable.
