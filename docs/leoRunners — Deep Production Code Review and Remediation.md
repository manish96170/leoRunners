You are reviewing the current `main` branch of the leoRunners repository.

The platform is intended to become a production-quality extensible ephemeral CI platform supporting:

- GitHub Actions
- one ephemeral runner per job
- AWS EC2 initially
- GCP Compute Engine
- future managed runner capacity
- customer-owned/self-hosted capacity
- managed + hybrid modes
- durable lifecycle state
- concurrent jobs
- cancellation/reruns
- reconciliation/reaping
- optional extensions
- optional AI, disabled by default

Do NOT assume the current implementation is correct merely because tests pass.

Perform a deep production code review of the entire repository and then implement only confirmed fixes.

The goal is:

> maximum correctness + reliability + concurrency + performance without introducing regressions or unnecessary complexity.

---

# 1. Critical finding: durable webhook processing

Inspect:

- controller/internal/controller
- controller/internal/state
- DynamoDB repository
- webhook handler
- lifecycle event/idempotency implementation

Current concern:

`HandleWorkflowJob()` inserts an idempotency/lifecycle marker before performing long-running JIT/runner provisioning.

If the process crashes after the marker is committed but before processing finishes, GitHub may retry the webhook and the retry may be treated as a duplicate.

This can lose a job.

Design a proper durable processing state.

We need something conceptually like:

```text
event received
    ↓
durably accepted
    ↓
pending
    ↓
processing
    ↓
completed
```

A crash must leave work recoverable.

Requirements:

- duplicate webhook delivery must be safe
- crash after acceptance must not lose work
- controller restart must recover pending work
- two controllers must not process the same work concurrently
- retries must be bounded
- processing must be idempotent
- completed work must not execute twice
- failed work must be retryable
- permanent failures must become visible
- GitHub webhook handler should not perform the entire provisioning lifecycle synchronously

Do NOT simply delete the idempotency marker and call that fixed.

Add tests specifically for:

1. crash after durable acceptance
2. retry after crash
3. duplicate deliveries
4. two controller instances
5. worker restart
6. processing timeout
7. permanent failure
8. successful completion

---

# 2. Decouple webhook ingestion from provisioning

The current HTTP webhook path appears to wait for the full provisioning/JIT/registration lifecycle.

This is not desirable at production scale.

Target:

```text
GitHub webhook
    ↓
verify + validate
    ↓
durably enqueue/accept
    ↓
HTTP 202
    ↓
background worker
    ↓
JIT
    ↓
provider
    ↓
registration
```

The webhook endpoint should return quickly.

Investigate whether the cleanest implementation is:

- durable DynamoDB work queue/state
- internal worker queue
- SQS
- another mechanism

Do not introduce SQS automatically.

Choose based on:

- durability
- retry semantics
- concurrency
- operational complexity
- AWS cost
- ordering requirements
- duplicate handling
- local testing

Keep the abstraction provider-neutral where useful.

---

# 3. Distributed idempotency

The current assignment service has an in-process `inflight` map.

That is insufficient for multiple controller replicas.

Validate that durable state is authoritative.

Test:

```text
Controller A
    ↓
Job X

Controller B
    ↓
Job X
```

Only one may successfully claim/process the assignment.

Do not rely on:

```go
sync.Mutex
sync.Map
in-memory map
```

for cross-process correctness.

Use conditional DynamoDB state/lease operations where appropriate.

---

# 4. Distributed capacity reservations

Review `capacity.Registry`.

Current implementation is in-memory.

That is fine for unit tests and perhaps a single-process development mode, but production multi-replica controllers need a shared capacity reservation mechanism.

Determine whether capacity should be:

```text
DynamoDB authoritative ledger
```

or another appropriate shared mechanism.

Requirements:

- atomic reservation
- idempotent reservation
- release
- crash recovery
- stale reservation recovery
- fencing
- no double counting
- no negative capacity
- concurrent reservation correctness
- multiple controller replicas

Do not remove the current clean provider-independent capacity API unless necessary.

Prefer:

```text
capacity interface
     ↓
memory implementation
DynamoDB implementation
```

if that is justified.

Add concurrency tests representing actual multiple replicas, not just two objects sharing one in-memory registry.

---

# 5. Provider registry

Review reconciliation and cleanup.

Current architecture has persisted:

```text
runner.Provider
runner.ProviderInstanceID
```

but reconciliation appears to have one provider dependency.

This is incompatible with real multi-cloud operation.

Introduce a provider registry/resolver where appropriate:

```text
ProviderRegistry
    ├── aws
    ├── gcp
    └── future providers
```

For every runner:

```text
runner.Provider
    ↓
resolve provider
    ↓
Status / Terminate
```

Ensure:

- AWS runner is never sent to GCP
- GCP runner is never sent to AWS
- unknown provider fails closed
- cleanup still works after controller restart
- provider configuration is not lost

Review ALL cleanup paths, not only normal completion.

---

# 6. Reconciliation performance

Review the current reconciler.

It should not become an O(all historical records) operation every interval.

Investigate:

```text
ListJobs()
ListRunners()
ListLeases()
```

and DynamoDB scans.

Avoid periodic full-table scans for normal operation.

Prefer targeted indexes/queries for:

```text
queued
provisioning
assigned
running
terminating
expired
failed-but-recoverable
```

Use the expiry GSI for expired resources.

Historical/terminal records should not cause reconciliation cost to grow forever.

Define bounded work per reconciliation pass.

Support pagination correctly.

---

# 7. Parallelize reconciliation safely

Provider status checks are independent.

Do not perform hundreds of network calls serially.

Introduce bounded worker concurrency.

Example:

```text
100 runners
     ↓
10 workers
     ↓
parallel provider status calls
```

But do NOT simply launch one goroutine per runner.

Respect:

- AWS API limits
- GCP API limits
- controller CPU
- memory
- context cancellation
- provider-specific limits

Make concurrency configurable.

Add tests for:

- maximum concurrency
- cancellation
- one provider failure not blocking others
- bounded goroutines
- no data races
- deterministic final state

---

# 8. Scheduler contention

Review:

```text
Select()
Reserve()
```

There may be a race where:

```text
Select pool A
     ↓
another request reserves pool A
     ↓
Reserve A fails
```

If other eligible pools exist, scheduler should be able to try the next candidate.

Design this carefully so it doesn't introduce starvation or excessive retries.

Potential model:

```text
eligible candidates
    ↓
rank candidates
    ↓
attempt atomic reservation
    ↓
failure due to contention
    ↓
try next candidate
```

Do not retry arbitrary provider failures as capacity failures.

Distinguish:

- no capacity
- contention
- provider unavailable
- invalid configuration
- security policy rejection

---

# 9. Cleanup correctness

Audit EVERY cleanup path:

- provisioning failure
- readiness timeout
- registration failure
- registration mismatch
- lease creation failure
- job cancellation
- job completion
- controller crash
- controller restart
- reconciliation
- expiration
- orphan detection
- provider-not-found
- provider API failure

For every path answer:

```text
Can EC2/GCP resource leak?
Can GitHub runner remain registered?
Can capacity remain reserved?
Can durable runner state remain wrong?
Can lease remain active?
Can retry cause duplicate termination?
```

Termination must remain idempotent.

Unknown provider must fail closed rather than accidentally using a default provider.

---

# 10. Capacity release correctness

Audit the exact relationship:

```text
Reserve
Provision
Runner persisted
Lease persisted
Job running
Job terminal
Terminate
Release
```

Make sure every successful reservation has exactly one release.

Test crash points:

```text
after reserve
after provider create
after runner persist
after lease persist
after job assignment
after termination
before release
after release
```

The system must recover leaked capacity through reconciliation.

---

# 11. Runner/provider idempotency

Review AWS and GCP idempotency.

AWS:

- ClientToken
- deterministic runner identity
- duplicate RunInstances handling

GCP:

- request ID
- deterministic identity
- retry after timeout

Test the difficult case:

```text
provider successfully creates VM
controller loses response
controller retries
```

There must not be two runners.

Also test:

```text
provider call times out
actual resource exists
controller retries
```

---

# 12. JIT credential/config security

Audit the GitHub JIT configuration flow.

The JIT config is highly sensitive.

Verify it is NEVER:

- logged
- persisted in durable state
- emitted through telemetry
- included in lifecycle events
- written to diagnostics
- exposed in errors
- exposed through metrics
- retained after bootstrap

Check AWS user-data handling and GCP startup-script handling carefully.

Also verify that process crashes cannot accidentally expose it through logs.

---

# 13. Webhook security

Audit:

- signature validation
- body limits
- request timeouts
- replay/duplicate handling
- event ID handling
- repository scope
- organization scope
- runner group
- labels
- fork PRs
- untrusted workloads

Security policy must remain deterministic.

Do not let AI/extensions override admission/security decisions.

---

# 14. Reconciliation race conditions

Look for:

```text
worker:
    runner = Ready

reconciler:
    runner = Terminating

worker:
    runner = Ready
```

or similar stale writes.

Every state transition must use revision/fencing/conditional persistence where required.

Audit all `SaveJob`, `SaveRunner`, and `SaveLease` calls.

Ensure stale controllers cannot resurrect old state.

---

# 15. DynamoDB review

Perform a complete DynamoDB production review.

Check:

- PK/SK design
- GSI design
- hot partitions
- Scan usage
- pagination
- ConsistentRead usage
- conditional writes
- transactions
- TTL
- indexes
- item size
- event retention
- historical growth
- write amplification
- retry behavior
- throttling
- exponential backoff
- idempotency

Especially investigate whether the event architecture creates unnecessary duplicated DynamoDB writes.

Do not optimize prematurely, but identify real scaling problems.

---

# 16. Event bus review

The in-process event bus may intentionally drop events when subscriber buffers are full.

Determine which events are:

```text
best effort
```

versus:

```text
must not be lost
```

Telemetry can be best effort.

Important lifecycle events required for AI/analytics should have a durable source.

Do NOT make AI part of the critical path.

Potential architecture:

```text
durable CI event
       ↓
event bus
       ↓
extensions
```

rather than:

```text
event bus only
```

---

# 17. Extension architecture

Review the current extension implementation.

Requirements:

- extension failure must not break CI
- extension timeout must not block runner lifecycle
- extension panic must not crash controller
- bounded queue
- bounded memory
- tenant isolation
- permissions
- deterministic policy
- disabled-by-default where appropriate

Check whether extension processing is accidentally coupled to webhook latency.

---

# 18. AI architecture

Do not implement new AI functionality during this review unless necessary.

Verify that the architecture supports future:

```text
AI OFF
AI ON globally
AI ON per repository
AI ON per workflow
AI ON per PR
AI ON via label/tag
AI ON for failures only
```

Possible future model providers:

```text
Ollama
OpenAI
AWS Bedrock
local model
other providers
```

AI must be replaceable.

AI must never control:

```text
IAM
secret access
runner security
unsafe workload admission
arbitrary termination
```

---

# 19. Small-model compatibility

The future AI architecture should support small local models.

Example:

```text
CI log
  ↓
parser/filter
  ↓
structured failure context
  ↓
3B/4B model
  ↓
analysis
```

Do not require a large GPU model.

But do NOT introduce Ollama into the core controller.

It should be an optional extension/model provider.

---

# 20. Managed/self-hosted/hybrid architecture

Review whether current code genuinely supports:

```text
customer-owned
managed
hybrid
```

rather than only representing these concepts in structs/configuration.

Test:

```text
Customer AWS
Our AWS
Customer GCP
Our GCP
```

as separate capacity pools.

Ownership must affect scheduling policy but should not leak into provider lifecycle code.

---

# 21. Performance review

Review the whole hot path:

```text
GitHub
 ↓
webhook
 ↓
durable acceptance
 ↓
queue
 ↓
scheduler
 ↓
capacity reservation
 ↓
JIT
 ↓
provider
 ↓
runner readiness
```

Identify every unnecessary serial operation.

Potential improvements:

- parallel independent validation
- connection/client reuse
- bounded worker pools
- avoid unnecessary DynamoDB reads
- avoid duplicate state writes
- provider-specific concurrency
- cache immutable configuration
- avoid repeated GitHub API calls
- efficient polling/backoff
- jitter
- targeted reconciliation

Do not optimize merely for fewer lines of code.

Optimize measured latency and throughput.

---

# 22. Polling review

AWS/GCP readiness polling currently uses fixed polling intervals.

Review:

- exponential backoff
- jitter
- provider-specific limits
- maximum polling frequency
- startup latency
- timeout behavior

Do not make polling so slow that runner startup suffers.

Do not poll so aggressively that API limits become a problem.

---

# 23. Testing review

Add missing tests for real failure modes.

At minimum:

### Concurrency

- 100 simultaneous jobs
- multiple controller replicas
- same job delivered twice
- scheduler contention
- capacity exhaustion

### Failure

- controller crash
- provider timeout
- provider returns success but response is lost
- GitHub API timeout
- registration timeout
- cancellation during provisioning
- cancellation during registration
- termination timeout

### Recovery

- restart
- reconciliation
- orphan recovery
- stale lease
- stale reservation
- fencing

### Security

- fork PR
- unauthorized repository
- unauthorized organization
- unauthorized label
- wrong runner group
- JIT secret leakage

### Multi-cloud

- AWS + GCP concurrently
- AWS cleanup
- GCP cleanup
- unknown provider
- provider unavailable

Run:

```text
go test ./...
go test -race ./...
go vet ./...
go test with stress/concurrency where useful
```

---

# 24. Code quality

Review for:

- unnecessary interfaces
- duplicated logic
- excessive abstraction
- error wrapping
- context propagation
- cancellation
- goroutine leaks
- mutex scope
- data races
- nil handling
- configuration validation
- naming
- package boundaries
- test readability
- deterministic behavior

Do not refactor code merely for style.

Only make changes that improve correctness, maintainability, performance, or security.

---

# 25. Important constraint

Do NOT rewrite the architecture from scratch.

The existing architecture is intentionally layered:

```text
GitHub
Controller
Scheduler
Capacity
Providers
State
Lifecycle
Events
Extensions
Intelligence
```

Preserve those boundaries where they are good.

Only change them when there is a concrete correctness/scaling reason.

---

# 26. Implementation process

Work in stages.

First:

1. inspect all relevant code
2. confirm each suspected issue
3. identify false positives
4. rank findings

Then implement fixes in priority order.

Priority:

```text
P0:
job loss
duplicate execution
security vulnerability
resource leak

P1:
multi-replica correctness
multi-provider cleanup
capacity correctness
reconciliation correctness

P2:
performance/scaling
parallelism
DynamoDB efficiency

P3:
maintainability/refactoring
```

After each group of fixes run the complete test suite.

Never fix one concurrency issue by introducing another.

---

# 27. Required final report

At the end report:

## Confirmed bugs

For every bug:

- severity
- file
- function
- exact failure scenario
- why existing tests did not catch it
- fix

## Performance issues

Include:

- current behavior
- bottleneck
- proposed/implemented optimization
- expected effect
- measurement if available

## Concurrency issues

Include:

- race scenario
- protection mechanism
- tests

## Security

Include:

- confirmed vulnerabilities
- sensitive-data paths
- fork/untrusted workload concerns
- credential handling

## Architecture

Explain:

- what was preserved
- what changed
- why

## Tests

Report exact commands and results.

## Remaining risks

Do not claim production-ready if live AWS/GCP/GitHub validation is still environment-gated.

## Recommended next step

Give one concrete next implementation step.

Most importantly:

> Do not report "tests pass" as proof that distributed correctness is solved. Explicitly test crash/retry/concurrency/provider-loss scenarios.