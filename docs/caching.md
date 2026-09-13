# Ephemeral Runner Caching

Caching is an optimization for ephemeral runners, not a prerequisite for
correctness. Every runner must be able to perform a clean checkout, install
dependencies, build, and test with an empty cache. A cache outage, miss, stale
entry, or rejected restore must not block scheduling, lease acquisition,
runner registration, or cleanup.

## Policy

The default policy in [`cache/config.v1.yaml`](../cache/config.v1.yaml) is
`observe-only`. In this mode the platform records whether a cache would have
been useful, but it does not restore or save shared objects. Enabling a cache
requires an approved repository, immutable revision or equivalent trust
boundary, an owner, a retention limit, and evidence that the optimization is
worth its operational and storage cost.

Cache configuration is advisory workload data. The scheduler may use measured
cache locality as a tie-breaker only after resource, label, ownership, region,
security, and availability constraints are satisfied. It must never reject a
pool because a preferred cache is unavailable.

## Supported ecosystems

The initial vocabulary is:

- `npm`: npm package download and metadata cache.
- `pnpm`: pnpm content-addressable store.
- `yarn`: Yarn download and metadata cache.
- `cargo`: Cargo registry and Git checkout cache.
- `go-modules`: Go module and build caches.
- `docker-layers`: explicitly approved Docker layer or build cache data.
- `terraform`: provider plugins and module download cache.

The platform must not assume that a repository uses a particular ecosystem.
Enable an entry only when workflow, lockfile, configuration, or measured job
evidence supports it. A cache entry is not permission to install tools into an
image or to copy a complete dependency file into telemetry.

## Keying and invalidation

Cache identity must include the smallest set of immutable inputs needed to
prevent incompatible reuse. A recommended logical key contains:

```text
platform-version / tenant-boundary / repository-scope / ecosystem /
lockfile-or-manifest-digest / toolchain-digest / architecture / cache-schema
```

Use a cryptographic digest or opaque fingerprint in telemetry. Do not log the
raw key, repository URL, branch name, workflow token, or object path.

Invalidate or bypass an entry when any relevant input changes, including:

- lockfile, dependency manifest, provider lockfile, or build definition;
- package-manager, compiler, Terraform, Docker, or base-image version;
- operating system, architecture, ABI, or cache schema;
- trust boundary, repository ownership, or encryption key policy;
- explicit operator purge, suspected poisoning, restore failure, or expiry.

Mutable branch names are selectors, not cache identity. Pull requests and
forks must not share a cache namespace with a trusted repository unless an
owner-approved policy explicitly defines the boundary. Never restore untrusted
data into a trusted build without the workload's normal integrity checks.

## Restore and save behavior

Restore is best effort. Use bounded timeouts, size limits, checksum or archive
integrity checks, and a clear invalidation reason. On a miss or recoverable
restore error, continue with a clean workspace. Save only after the job has
reached the policy's save condition, and avoid saving failed, cancelled, or
untrusted outputs unless the ecosystem policy explicitly permits it.

Cache operations must not hold the scheduler lease longer than the normal job
lifecycle requires. They must not decide provider selection, runner capacity,
job admission, or termination. Cleanup must release compute and leases even
when restore or save is slow or unavailable.

## Telemetry

For each observed cache operation, record only bounded, non-sensitive fields:

```text
cache_id, policy_version, mode, outcome, hit_or_miss,
restore_duration_ms, save_duration_ms, size_bytes,
invalidation_reason, provider, region, architecture
```

Use `CacheHit`, `CacheMiss`, and lifecycle timing fields as operational events.
Metric labels must be low cardinality and allowlisted. Keep job, runner,
request, delivery, and raw key correlation IDs in structured logs or events,
not metric dimensions. Redact authorization headers, tokens, JIT values,
credentials, signed URLs, environment variables, and dependency contents.

Distinguish `not_observed`, `disabled`, `miss`, `hit`, `restore_failed`,
`save_failed`, `expired`, and `invalidated`; a missing observation is not a
cache miss. Record estimated cache storage or transfer cost separately from
cloud billing truth.

## Privacy and tenant isolation

Cache data can reveal dependency choices, source structure, private package
names, and build outputs. Treat it as customer data. Use separate namespaces
for tenants and repositories, least-privilege access, private endpoints where
available, and short-lived workload credentials. The controller should receive
policy and telemetry only; runner-side cache access must be scoped to the
approved namespace.

Do not place secrets in cache archives. Exclude credential files, `.env` files,
temporary tokens, SSH material, cloud configuration, and generated logs unless
the workload owner has explicitly reviewed the contents. A cache purge must be
available for suspected secret exposure or poisoning.

## S3 security and lifecycle

When S3 is selected as a cache object store:

- Use a dedicated private bucket or an explicitly isolated prefix with a clear
  data owner; never use a public bucket.
- Block public access, require TLS, and deny requests that do not use secure
  transport.
- Require server-side encryption. Use SSE-S3 by default or a customer-managed
  KMS key where ownership, rotation, and audit requirements demand it.
- Scope runner access to the approved bucket prefix and required object verbs.
  Do not grant the controller broad object read or write access.
- Deny object access outside the tenant/repository boundary. Keep bucket and
  KMS administration outside the runtime roles.
- Enable versioning only when the retention and purge procedure can delete old
  versions; otherwise stale versions can defeat an apparent purge.
- Configure lifecycle expiration for current and noncurrent versions, abort
  incomplete multipart uploads, and bound storage growth.
- Use checksums, content-type and size limits, and immutable key components;
  object names are not an authorization boundary by themselves.
- Capture access and lifecycle evidence without recording credentials,
  presigned URLs, or full object paths in general telemetry.

S3 is an optional cache backend. Its availability must not be required for
job admission, scheduling, runner readiness, lease renewal, or termination.
Use a separate artifact policy when data must be retained as a build output;
cache expiration must not delete required artifacts.

## Observe-only rollout

1. Publish a new cache policy version and keep every ecosystem in
   `observe-only` mode.
2. Run approved workloads with cache observations enabled and collect repeated
   baseline samples using the same image, revision, provider, and region.
3. Review hit rate, restore/save duration, size, failure rate, transfer/storage
   estimate, invalidation causes, and any privacy findings.
4. Enable one ecosystem for one trust boundary only after owner approval and
   policy review.
5. Compare candidate and baseline evidence, then expand or revert to
   `observe-only` using the same versioned configuration process.

Rollback is a configuration change: disable restore/save and keep clean-build
behavior. Do not delete shared objects as an automatic response; use the
approved purge procedure for security incidents or explicit retention work.
