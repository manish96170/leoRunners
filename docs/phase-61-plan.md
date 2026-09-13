# Phase 61: Controlled Workload Validation

## Objective

Define a repeatable, bounded procedure for validating an already approved
workload at a fixed repository commit. The procedure reviews commands before
execution, applies explicit time and output limits, isolates network access,
redacts evidence, and produces a handoff suitable for benchmark comparison.
It does not authorize arbitrary workflow execution, cloud provisioning, or
production scheduling.

## Deliverables

- A validated workload manifest and immutable commit reference.
- A command-review record covering every command that may run.
- Explicit timeout, output, resource, and concurrency limits.
- A network-isolation profile with approved endpoints, if any.
- Redacted execution and cleanup evidence with stable identifiers.
- A benchmark handoff stating comparability and provenance.
- Abort, cleanup, and rollback records for successful and failed attempts.
- A documented boundary for later AWS- and GitHub-backed validation.

## Validation sequence

1. Confirm the approved repository, full commit ID, manifest digest, scope
   file, owner, reviewer, deadline, and rollback record. Recompute the digest
   over the exact manifest bytes that will be used.
2. Review every command, script entry point, working directory, environment
   input, artifact path, and service dependency. Reject commands that are
   dynamically constructed without bounded inputs, contain secrets, bypass
   the declared toolchain, or exceed the approved workload scope.
3. Set hard limits before starting: wall-clock timeout, per-command timeout,
   output byte limit, log line or event limit, CPU, memory, disk, process,
   artifact-size, and concurrency limits. A missing limit is a block, not an
   implicit unlimited value.
4. Start with the least-privileged isolated execution profile. Deny inbound
   access, host access, metadata access, privilege escalation, and undeclared
   mounts. Allow only the reviewed network endpoints and protocols required by
   the manifest; use no-network mode when dependencies are not required.
5. Capture start, command-review, execution, timeout, output-limit, artifact,
   network, and cleanup checkpoints. Store bounded summaries and digests, not
   raw workflow payloads, credentials, unrestricted logs, or complete source
   archives.
6. Stop on timeout, output exhaustion, policy violation, unexpected network
   access, resource exhaustion, secret detection, scope drift, or an
   unrecoverable runner error. Termination must enter cleanup before the
   validation is marked failed or incomplete.
7. Prove cleanup: terminate the workload process tree, remove temporary
   files and credentials, release reservations, verify no runner or provider
   resources remain, and record any residual resource as `BLOCKED`.
8. Validate the evidence package, compare it with the approved manifest and
   baseline, and label synthetic, mock, or real measurements explicitly.
   Hand off only when command, commit, environment, resource, cache, network,
   and pricing assumptions satisfy the comparability review.

## Exit criteria

- The full commit and manifest digest match the approved handoff.
- Every executable command has a reviewed source, bounded inputs, and an
  explicit success or failure condition.
- Time, output, resource, artifact, and concurrency limits were enforced.
- Network access matches the declared isolation profile and endpoint set.
- Evidence is redacted, schema-compatible, owner-controlled, and atomic.
- Cleanup completed and its result is independently observable.
- Measurements have provenance, units, sample counts, and comparability
  status sufficient for benchmark review.
- The final result is clearly `PASS`, `WARN`, `FAIL`, `BLOCKED`, or
  `INCOMPLETE`; missing data is never silently treated as success.

## Abort and cleanup criteria

Abort immediately for an unreviewed command, changed commit or manifest,
secret exposure, unexpected network destination, host or metadata access,
privilege escalation, resource-limit breach, output flood, deadline expiry,
or failure to identify the workload owner. Preserve only a redacted incident
reference and bounded checkpoint evidence.

Cleanup must be attempted on success, failure, timeout, cancellation, and
operator interruption. Verify process-tree termination, temporary workspace
removal, credential removal, cache handling, reservation release, and
provider-resource absence. If cleanup cannot be proven, classify the run as
`INCOMPLETE` or `BLOCKED`, retain the cleanup failure evidence, and do not
hand off benchmark results.

## Benchmark handoff

The handoff contains the fixed commit, manifest digest, command and toolchain
identity, execution profile, cache state, network profile, resource limits,
measurement units, sample counts, synthetic/real provenance, redacted evidence
references, cleanup result, and reviewer decision. It must not contain tokens,
private keys, raw environment values, unrestricted logs, or customer payloads.

Only comparable results may update a baseline. A changed command, image,
architecture, provider, region or zone, cache state, network profile,
resource limit, pricing assumption, or success criterion creates a new
comparison group or requires an explicit non-comparable label.

## Later provider boundaries

Phase 61 may validate the workload locally or against approved mocks. Later
AWS-backed validation may provision an ephemeral runner only after separate
AWS account, region, networking, IAM, launch-template, cost, and cleanup
approval. Later GitHub-backed validation may register a single-use runner
only after repository, organization, label, runner-group, event, and JIT
scope approval. Neither provider path is implied by a successful controlled
workload validation, and neither may bypass the command-review, isolation,
evidence, or cleanup gates defined here.

## Out of scope

This phase does not approve arbitrary repositories or branches, execute
unreviewed GitHub workflows, publish images, grant credentials, mutate cloud
infrastructure, register persistent runners, or change production scheduling.
