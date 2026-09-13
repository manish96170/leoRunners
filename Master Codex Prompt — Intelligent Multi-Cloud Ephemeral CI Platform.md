# Build an Extensible Multi-Cloud Ephemeral CI Platform

You are working on a new project to build an **ephemeral CI execution platform** that can eventually operate in two modes:

1. **Self-hosted mode** — customers run the platform/runners inside their own AWS/GCP infrastructure.
2. **Managed mode** — we operate the infrastructure and provide runners as a service, similar to managed CI runner platforms such as Blacksmith/GetMac.
3. **Hybrid mode** — customers can use some of their own infrastructure and some managed capacity.

The core platform must work perfectly without AI.

AI is an optional extension that can be introduced later and must never become a critical dependency for runner provisioning, scheduling, job execution, cancellation, cleanup, or security.

---

# 1. Product Vision

Do not think of this project as simply:

> "Build our own Blacksmith."

The larger vision is:

> Build an extensible, multi-cloud, ephemeral CI execution platform where the same control plane can operate customer-owned infrastructure or our managed infrastructure, while exposing clean extension points for caching, analytics, observability, cost optimization, AI, and other future capabilities.

The platform should eventually be able to:

```text
                    GitHub
                       |
                       v
                CI Control Plane
                       |
                    Scheduler
                       |
        +--------------+---------------+
        |              |               |
        v              v               v
     AWS EC2        GCP Compute     Managed Capacity
        |              |               |
        +--------------+---------------+
                       |
                Ephemeral Runner
                       |
                    CI Job
                       |
                       v
             Events / Metrics / Logs
                       |
             +---------+----------+
             |                    |
        Core Platform       Optional Extensions
                                  |
                         +--------+---------+
                         |        |         |
                        AI     Analytics   Cost
```

The same architecture should support:

- Customer AWS
- Customer GCP
- Our AWS
- Our GCP
- Future cloud providers
- Future macOS providers
- Future dedicated/managed hardware providers
- Potentially external runner providers

---

# 2. Core Principle

The platform must remain useful without AI.

The core execution path is:

```text
GitHub
  ↓
Webhook/Event
  ↓
Controller
  ↓
Scheduler
  ↓
Provider
  ↓
Ephemeral Runner
  ↓
GitHub Job
  ↓
Completion
  ↓
Cleanup
```

AI must NOT be required anywhere in this path.

If the AI service is:

- unavailable
- disabled
- misconfigured
- too slow
- out of quota
- running out of memory
- using a local model that crashes

CI must continue normally.

---

# 3. Modes of Operation

Design the platform from the beginning so it can operate in three modes.

## Mode A — Self-hosted

Customer deploys:

```text
Customer AWS/GCP
    |
    +-- Controller
    +-- State
    +-- Runner infrastructure
    +-- Observability
```

Their GitHub organization/repository uses our runner platform.

They control:

- cloud account
- networking
- IAM
- data
- runner capacity
- security policies

---

## Mode B — Managed

We operate:

```text
Our Cloud Infrastructure
       |
       +-- Control Plane
       +-- Scheduler
       +-- Runner Capacity
       +-- Observability
       +-- Managed storage
```

Customers simply configure GitHub to use our runners.

They should not need to manage:

- EC2
- VMs
- runner registration
- AMIs
- networking
- lifecycle
- cleanup
- autoscaling

This should provide a service comparable to managed ephemeral CI runner platforms.

---

## Mode C — Hybrid

For example:

```text
                Scheduler
                   |
       +-----------+-----------+
       |                       |
Customer AWS                Our AWS
private workloads           overflow capacity
```

Or:

```text
Customer GCP
     |
     +-- normal workloads

Our managed capacity
     |
     +-- burst workloads
```

The scheduler must treat these as providers/capacity pools rather than hard-code assumptions about ownership.

---

# 4. Initial Implementation

Start with:

> GitHub Actions + ephemeral AWS EC2 runners.

Do not initially implement every cloud/provider.

The first working path should be:

```text
GitHub workflow
      ↓
workflow_job event
      ↓
Controller
      ↓
Scheduler
      ↓
AWS EC2
      ↓
JIT/ephemeral runner
      ↓
GitHub assigns exactly one job
      ↓
Job completes
      ↓
EC2 terminated
```

This must be production-quality before expanding.

---

# 5. GitHub Integration

Use current official GitHub mechanisms for ephemeral/JIT self-hosted runners.

Investigate the current GitHub APIs/documentation before implementation.

Support:

- workflow_job queued
- JIT runner configuration
- runner registration
- runner labels
- runner groups where appropriate
- job assignment
- job execution
- job completion
- cancellation
- workflow reruns
- provisioning failure
- runner failure
- duplicate events
- missing events
- stale jobs
- GitHub API retries
- controller restart
- runner restart
- cleanup

Do not use long-lived GitHub credentials inside AMIs.

Prefer:

- GitHub App authentication where appropriate
- short-lived credentials
- secure secret handling

---

# 6. Ephemeral Runner Requirement

Every CI job should get a fresh runner.

Desired lifecycle:

```text
Job queued
    ↓
Runner provisioned
    ↓
Runner registers
    ↓
GitHub assigns job
    ↓
Job executes
    ↓
Runner becomes disposable
    ↓
Infrastructure terminated
```

A runner must never accidentally execute another unrelated job.

The platform should assume:

> Runner = disposable compute lease.

---

# 7. Concurrency

If 20 jobs arrive:

```text
20 jobs
   ↓
20 independent runner leases
   ↓
20 runners
```

Do not serialize jobs unnecessarily.

The scheduler should understand:

- available capacity
- cloud capacity
- provider limits
- account quotas
- instance types
- startup time
- estimated job duration
- cost
- security requirements

---

# 8. Cancellation and Reruns

Cancellation is first-class.

Example:

```text
GitHub job
   ↓
Runner provisioning
   ↓
User cancels workflow
   ↓
Controller detects cancellation
   ↓
Runner terminated
```

Rerun:

```text
Old runner
    ↓
disposed

New workflow attempt
    ↓
new runner
```

Never reuse the old runner for the rerun.

---

# 9. Orphan Protection

This is critical.

Every runner should have enough metadata to identify:

- repository
- workflow
- workflow run
- job
- runner ID
- provider
- creation time
- expiration time
- controller ownership

For AWS, use tags such as:

```text
Platform=ci-runner
ManagedBy=<platform>
Repository=<repo>
WorkflowRunId=<id>
JobId=<id>
RunnerId=<id>
CreatedAt=<timestamp>
ExpiresAt=<timestamp>
```

Implement reconciliation/reaper logic.

The reaper should identify:

- runner exists but GitHub job no longer exists
- runner is stuck
- EC2 exists too long
- runner registration failed
- controller crashed
- GitHub lost runner state
- job completed but EC2 remains
- provisioning partially succeeded

Every runner must have a maximum lifetime.

---

# 10. Persistence

Do not rely entirely on process memory.

Design persistent state around actual access patterns.

Potential state:

```text
Job
Runner
ProviderInstance
LifecycleEvent
Lease
RunnerImage
ProviderCapacity
```

DynamoDB is a strong initial candidate for AWS, but do not blindly design the database around assumptions.

Determine:

- primary access patterns
- idempotency requirements
- conditional writes
- lifecycle queries
- reconciliation queries
- retention
- cost

---

# 11. Provider Abstraction

Do NOT make the controller AWS-specific.

Create a real provider boundary.

Conceptually:

```go
type RunnerProvider interface {
    Provision(ctx context.Context, spec RunnerSpec) (*RunnerInstance, error)
    WaitReady(ctx context.Context, instance *RunnerInstance) error
    GetStatus(ctx context.Context, instance *RunnerInstance) (RunnerStatus, error)
    Terminate(ctx context.Context, instance *RunnerInstance) error
}
```

Improve this interface if investigation shows a better design.

Initial implementation:

```text
providers/
    aws/
```

Future:

```text
providers/
    aws/
    gcp/
    azure/
    macos/
    external/
```

Do not create meaningless abstractions just for theoretical future support. Abstract the real provider boundary.

---

# 12. RunnerSpec

The scheduler should express requirements, not cloud-specific machine names.

Conceptually:

```go
type RunnerSpec struct {
    CPU             int
    MemoryGB        int
    Architecture    string
    OS              string
    DiskGB          int
    GPU             bool
    Image           string
    Labels          []string
    DockerRequired  bool
    NodeVersion     string
    SecurityProfile string
}
```

Improve the design if necessary.

The scheduler should say:

```text
Linux
x86_64
8 CPU
16 GB RAM
Docker
Node 22
Chrome
```

The provider decides:

```text
AWS EC2 instance type
AMI
subnet
security group
storage
```

For GCP:

```text
Compute Engine machine type
image
network
disk
```

---

# 13. AWS Implementation

For MVP:

DO NOT use EC2 Fleet.

Use:

```text
Launch Template
     +
RunInstances
```

Reasons:

- simpler
- easier to debug
- easier to understand
- sufficient for MVP
- provider abstraction keeps future options open

Future possibilities:

- EC2 Fleet
- Auto Scaling
- warm pools
- Spot
- capacity reservations

Only introduce them when measurements justify them.

---

# 14. Multi-Cloud

After AWS works reliably, implement GCP.

Architecture:

```text
Scheduler
   |
   +---- AWS Provider
   |
   +---- GCP Provider
```

The scheduler should eventually evaluate:

- startup latency
- cost
- capacity
- architecture
- CPU
- memory
- disk
- region
- cache locality
- reliability
- interruption risk
- security policy

The developer's GitHub workflow should not care whether the runner came from AWS or GCP.

---

# 15. macOS / External Managed Providers

Do not assume every runner must be EC2.

The provider model should eventually allow:

```text
AWS EC2
GCP Compute
Managed macOS provider
Managed Linux provider
Customer-owned provider
```

For example:

```text
Scheduler
    |
    +-- AWS
    +-- GCP
    +-- Managed Mac
    +-- Customer infrastructure
```

This allows the platform to eventually provide both:

> "Run this on infrastructure you own"

and:

> "Run this using infrastructure we provide."

Do not implement macOS in Phase 1 unless investigation shows it is necessary.

---

# 16. Runner Images

Blazing-fast startup is a major requirement.

Do NOT install the entire development environment during EC2 user-data.

Use immutable pre-baked images.

Investigate:

- Packer
- EC2 Image Builder
- AMI versioning
- validation
- rollout
- rollback

Likely initial tooling:

- Git
- curl
- jq
- Node.js
- npm/pnpm/yarn as needed
- Chrome/Chromium
- Cypress dependencies
- AWS CLI
- Terraform
- Serverless
- Docker if required

Do not blindly install everything.

Inspect actual workflows and repositories first.

Potential image profiles:

```text
base-linux-x64
node-linux-x64
node-browser-linux-x64
rust-linux-x64
go-linux-x64
```

But avoid creating dozens of images prematurely.

---

# 17. Real Workload Validation

Use a real repository/workload rather than only synthetic benchmarks.

The workload includes technologies such as:

- Node.js
- JavaScript
- React
- Redux
- Serverless Framework
- AppSync GraphQL
- Lambda
- SQS
- Terraform
- VTL
- DynamoDB
- S3
- VPC
- NAT Gateway
- Redshift
- EC2
- frontend builds
- Cypress
- Vitest
- Biome
- ESLint
- TypeScript
- AWS CLI
- Docker
- Rust
- Go

Do not couple the platform to Node.js.

During experimentation, GitLab may remain the source of truth while a temporary GitHub copy is periodically synchronized for testing.

The GitHub copy should be used to validate the complete runner lifecycle.

---

# 18. Performance

The goal is to build a very fast platform.

Do not claim "faster than Blacksmith/GetMac" without measurement.

Measure:

```text
GitHub queue time
Controller processing
Scheduling
Provider provisioning
EC2 launch
Boot
Runner registration
Runner ready
Job start
Checkout
Cache restore
Dependency installation
Build
Tests
Cypress
Artifacts
Cleanup
Total runtime
```

The biggest likely bottleneck is runner startup rather than Go controller performance.

Optimize:

- AMI size
- boot time
- bootstrap
- runner registration
- instance type
- image preparation
- dependency caches
- browser availability
- Docker layers
- network
- possible warm capacity

Only introduce warm pools after measuring cold-start pain.

---

# 19. Caching

Caching should be treated as a platform capability.

Potential caches:

```text
npm
pnpm
yarn
pip
Cargo
Go modules
Docker layers
Terraform
Cypress
Playwright
browser binaries
```

Do not implement every cache initially.

First collect telemetry:

```text
cache hit
cache miss
cache size
restore duration
save duration
invalidation reason
```

Future:

```text
workflow
   ↓
predict dependencies
   ↓
prefetch cache
   ↓
runner starts
   ↓
dependencies already available
```

Caching should be extensible rather than tightly coupled to the scheduler.

---

# 20. Networking

Investigate before deciding between:

```text
Public subnet + restrictive outbound security
```

and:

```text
Private subnet + NAT
```

Consider:

- GitHub connectivity
- npm/package registries
- third-party APIs
- AWS APIs
- Docker registries
- security
- NAT cost
- startup performance
- operational complexity

Document the decision.

---

# 21. Security

Security is a hard requirement.

Implement:

- least privilege IAM
- separate controller IAM
- separate runner IAM
- minimal runner permissions
- IMDSv2
- short-lived credentials
- GitHub OIDC where appropriate
- no credentials baked into AMIs
- no GitHub PAT baked into images
- restricted security groups
- secure JIT registration
- secure secret handling
- fork PR protection
- untrusted code isolation
- Docker security considerations

Never expose privileged AWS credentials to arbitrary pull requests.

Security policy must be deterministic.

AI may recommend/explain security issues later, but AI must never override security policy.

---

# 22. Observability

Make observability a first-class platform capability.

GitHub remains responsible for:

- job logs
- job status
- workflow UI
- cancellation
- reruns

Our platform should produce:

- controller logs
- provisioning logs
- lifecycle events
- runner metrics
- provider metrics
- timing metrics
- cost metrics
- errors

Potential destinations:

```text
CloudWatch
DynamoDB
S3
Postgres
OpenSearch
Vector storage
Future adapters
```

Do not permanently lock the architecture to one storage engine.

---

# 23. CI Data/Event Layer

This is extremely important for future extensions.

The core system should emit structured events such as:

```text
JobQueued
RunnerProvisioning
RunnerReady
JobStarted
JobCompleted
JobFailed
JobCancelled
RunnerTerminated
CacheHit
CacheMiss
ProvisioningFailed
RunnerTimeout
```

Also capture structured metadata:

```text
repository
workflow
branch
commit
PR
job
runner
provider
instance type
image
duration
CPU
memory
disk
network
cache
failure reason
cost
```

This becomes the foundation for future analytics and AI.

---

# 24. Extension Architecture

Design the platform so additional capabilities can be added without modifying the core runner lifecycle unnecessarily.

Potential extensions:

```text
extensions/
    analytics
    caching
    cost
    notifications
    ai
    security
    performance
```

Do not build all of them now.

Define sensible extension boundaries.

An extension should be able to consume platform events/data and optionally produce:

```text
insights
recommendations
annotations
actions
```

However, actions that affect security or lifecycle must go through deterministic platform policy.

---

# 25. AI Architecture — OPTIONAL

AI is NOT part of the core MVP.

Create a conceptual boundary for future AI:

```text
internal/
    intelligence/
```

or an equivalent extension/plugin architecture.

AI should consume CI data:

```text
CI Events
   ↓
Data/Log Layer
   ↓
AI Extension
   ↓
Model Provider
```

Potential AI tasks:

1. CI failure classification
2. "Why did this job fail?"
3. "Why was this job slow?"
4. CI log summarization
5. Flaky test detection
6. Resource prediction
7. Runner recommendation
8. Cache prediction
9. Cost optimization
10. Workflow optimization
11. Security analysis
12. Runner image optimization

Do NOT implement all of these initially.

Start with the simplest useful capabilities.

---

# 26. AI Must Be Provider-Agnostic

Do not hard-code one AI vendor.

Potential model providers:

```text
Ollama
OpenAI
AWS Bedrock
local model
other hosted model
future providers
```

Conceptually:

```go
type ModelProvider interface {
    Generate(ctx context.Context, request ModelRequest) (ModelResponse, error)
}
```

Improve the interface based on actual requirements.

---

# 27. Small Local Models

Support the possibility of running small local models.

For example:

```text
Ollama
   ↓
3B/4B-ish model
   ↓
CI log analysis
```

Do not assume a large GPU model is required.

Many CI tasks can first be reduced to structured context:

```text
Raw 100 MB log
      ↓
parser/filter
      ↓
errors + warnings + timings
      ↓
small context
      ↓
small model
```

This makes CPU-based or low-resource AI practical.

The platform should allow users to choose:

```text
No AI
Local AI
Managed AI
Specific model
Specific provider
```

---

# 28. AI Default OFF

AI must be disabled by default.

Possible configuration:

```yaml
extensions:
  ai:
    enabled: false
```

Users can enable it:

```yaml
extensions:
  ai:
    enabled: true
```

Or selectively:

```text
repository
workflow
branch
PR
label
tag
event
```

Examples:

```text
label: ai-debug
label: ai-review
```

or:

```text
workflow failure → AI analysis
```

or:

```text
specific PR → AI analysis
```

or:

```text
all failed jobs → AI analysis
```

The exact mechanism should be designed after investigating the cleanest architecture.

---

# 29. AI as Another Application

AI does not necessarily need to run inside the controller.

It may eventually be:

```text
Option A:

Controller
   |
   +-- AI extension
```

or:

```text
Option B:

Controller
   |
   +-- Event/Data Store
          |
          +-- Separate AI Service
```

or:

```text
Option C:

Controller
   |
   +-- Plugin/event
          |
          +-- External AI application
```

All should be possible.

This is important because customers may want:

- AI inside their own environment
- our managed AI
- their own OpenAI/Bedrock account
- Ollama
- no AI

---

# 30. AI Data Storage

Do not prematurely choose one database.

Evaluate:

```text
CloudWatch
DynamoDB
S3
DynamoDB vector capabilities
DAX
OpenSearch
Postgres/pgvector
dedicated vector DB
```

Separate the concepts:

```text
Raw logs
Structured CI events
Aggregated metrics
Searchable data
Vector embeddings
AI context
```

For example:

```text
S3
 └── raw logs / long-term data

DynamoDB
 └── jobs / runners / lifecycle state

CloudWatch
 └── operational logs / metrics

Vector store
 └── optional semantic retrieval
```

But make these replaceable through storage interfaces.

Do not introduce a vector database merely because AI might need one someday.

---

# 31. AI Does Not Control Critical Lifecycle

Never allow an AI model to directly decide:

```text
terminate arbitrary runner
grant IAM permission
expose secrets
approve unsafe code
bypass security
```

AI may produce:

```text
recommendation
classification
prediction
```

Then deterministic policy decides whether anything happens.

Example:

```text
AI:
"Runner probably needs 16 GB."

Scheduler:
"Allowed runner sizes are 8/16/32 GB."

Policy:
"16 GB is allowed."

Scheduler:
"Provision 16 GB."
```

---

# 32. Intelligence Roadmap

Do not build this now.

Future phases can include:

### Phase A

Historical/statistical intelligence:

- average duration
- resource usage
- failure rates
- cache hit rate

### Phase B

Prediction:

- expected duration
- CPU/memory requirement
- runner type
- cache requirements

### Phase C

Optimization:

- provider selection
- runner sizing
- caching
- prewarming
- cost

### Phase D

AI:

- log analysis
- failure explanation
- flaky tests
- workflow recommendations
- security analysis

---

# 33. Managed Service Architecture

The same core should eventually be deployable as:

```text
                    SaaS Control Plane
                           |
                 +---------+---------+
                 |                   |
          Managed Capacity     Customer Capacity
                 |                   |
              AWS/GCP             AWS/GCP
```

This means the platform itself should distinguish:

```text
Provider
Capacity Pool
Ownership
Region
Security Profile
Availability
Pricing
```

Do not hard-code "our AWS" into the scheduler.

---

# 34. Cost Model

Eventually managed mode needs cost visibility.

Track:

```text
provider
region
instance
duration
storage
network
cache
estimated cost
```

Future scheduler can optimize:

```text
fastest
cheapest
balanced
customer-owned-first
managed-fallback
```

Do not implement sophisticated cost optimization before collecting reliable data.

---

# 35. Control Plane Deployment

Investigate:

```text
Lambda
ECS/Fargate
EC2
Kubernetes
```

Choose based on:

- webhook handling
- concurrency
- long-running reconciliation
- GitHub API interactions
- startup
- cost
- reliability
- operational complexity

Do not introduce Kubernetes unless it is actually justified.

Do not choose a technology because it is popular.

---

# 36. Suggested Repository Structure

Start approximately with:

```text
ci-platform/
├── controller/
│   ├── cmd/
│   │   └── runner-controller/
│   ├── internal/
│   │   ├── github/
│   │   ├── scheduler/
│   │   ├── lifecycle/
│   │   ├── runners/
│   │   ├── providers/
│   │   │   ├── aws/
│   │   │   └── gcp/
│   │   ├── state/
│   │   ├── telemetry/
│   │   ├── policy/
│   │   ├── extensions/
│   │   ├── intelligence/
│   │   └── config/
│   ├── go.mod
│   └── go.sum
│
├── bootstrap/
│   ├── runner/
│   └── scripts/
│
├── ami/
│   ├── scripts/
│   ├── profiles/
│   └── README.md
│
├── terraform/
│   ├── modules/
│   │   ├── controller/
│   │   ├── runner-iam/
│   │   ├── runner-network/
│   │   ├── runner-ec2/
│   │   ├── runner-state/
│   │   └── observability/
│   └── environments/
│       └── dev/
│
├── workflows/
│
├── docs/
│   ├── architecture.md
│   ├── lifecycle.md
│   ├── scheduler.md
│   ├── providers.md
│   ├── self-hosted-mode.md
│   ├── managed-mode.md
│   ├── hybrid-mode.md
│   ├── security.md
│   ├── performance.md
│   ├── runner-images.md
│   ├── github-integration.md
│   ├── multi-cloud.md
│   ├── observability.md
│   ├── extensions.md
│   ├── intelligence.md
│   ├── operations.md
│   └── troubleshooting.md
│
└── README.md
```

Adjust this if investigation shows a better structure.

Do not create empty packages merely to satisfy this tree.

---

# 37. Phase 0 — Investigation Only

IMPORTANT:

Do NOT immediately start coding.

First investigate the current environment, requirements, architecture, and GitHub APIs.

Phase 0 should answer:

### GitHub

- Current ephemeral/JIT runner mechanisms
- APIs
- authentication
- cancellation
- reruns
- runner groups
- labels
- lifecycle
- webhook/event semantics

### AWS

- EC2 provisioning
- Launch Templates
- AMIs
- networking
- IAM
- IMDSv2
- startup time
- quotas
- instance options

### GCP

- equivalent Compute Engine capabilities
- image model
- lifecycle
- future provider design

### Managed mode

Determine how the architecture can support:

```text
Customer infrastructure
+
Our infrastructure
```

without duplicating the entire platform.

### Real repository

Inspect actual CI workflows.

Identify:

- required tools
- Node versions
- browsers
- Docker
- Terraform
- AWS CLI
- caches
- test systems
- Rust
- Go
- environment requirements

### Performance

Determine likely bottlenecks.

### Security

Identify risks, especially:

- fork PRs
- credentials
- Docker
- AWS access
- JIT registration
- network access

### Observability

Determine what data should be collected from day one.

### Extension architecture

Determine the minimum clean event/data/plugin boundaries needed for:

- caching
- analytics
- AI later

Do not implement AI in Phase 0.

---

# 38. Phase 0 Deliverables

At the end of Phase 0 produce:

```text
docs/architecture.md
docs/lifecycle.md
docs/scheduler.md
docs/providers.md
docs/security.md
docs/performance.md
docs/github-integration.md
docs/observability.md
docs/extensions.md
docs/multi-cloud.md
docs/self-hosted-mode.md
docs/managed-mode.md
docs/hybrid-mode.md
docs/intelligence.md
```

Also produce:

```text
Phase 1 implementation plan
```

Include:

- architecture decisions
- alternatives considered
- reasons for decisions
- risks
- estimated complexity
- files/packages to create
- test strategy
- performance measurements
- security requirements

---

# 39. Phase 1

Implement only the core controller.

Requirements:

- Go
- GitHub integration
- event handling
- job state
- lifecycle state machine
- scheduler
- provider abstraction
- fake provider
- persistence
- idempotency
- tests

No real EC2 yet unless necessary for the phase.

---

# 40. Phase 2

AWS provider.

Implement:

```text
Launch Template
RunInstances
Tags
Status
WaitReady
Terminate
```

Test provisioning independently.

---

# 41. Phase 3

Real ephemeral GitHub runner.

Implement:

```text
JIT configuration
Bootstrap
Registration
Job execution
Completion
Cleanup
```

Validate concurrency.

---

# 42. Phase 4

Reliability.

Implement:

- idempotency
- retries
- persistence
- reconciliation
- reaper
- cancellation
- controller restart recovery
- runner timeout
- provisioning failure recovery

---

# 43. Phase 5

Runner image optimization.

Build pre-baked AMI.

Measure:

```text
EC2 launch
boot
runner registration
ready
job start
```

Optimize the critical path.

---

# 44. Phase 6

Real repository workload.

Run the complete real CI workload.

Collect telemetry.

Fix compatibility issues.

---

# 45. Phase 7

Benchmark.

Compare objectively against current CI infrastructure / Blacksmith / other relevant providers where possible.

Measure:

- startup
- job start
- dependency install
- cache
- build
- tests
- Cypress
- Docker
- total runtime
- cost

Never make marketing claims without measurements.

---

# 46. Phase 8

GCP provider.

Add GCP without changing GitHub-facing workflow semantics.

---

# 47. Phase 9

Managed + Hybrid architecture.

Support:

```text
customer-owned capacity
our managed capacity
```

and allow scheduler policies such as:

```text
customer-first
managed-first
customer-only
managed-only
fallback
```

---

# 48. Phase 10 — Intelligence

Only after reliable telemetry exists.

Start with deterministic/statistical intelligence:

```text
job history
resource history
duration
failure rate
cache
provider performance
```

Then introduce optional AI.

---

# 49. AI MVP

The first AI feature should be something with obvious value and low risk.

Prefer:

> Analyze failed CI logs and explain the likely failure.

Flow:

```text
Job failed
   ↓
Event
   ↓
AI extension
   ↓
Retrieve relevant logs
   ↓
Filter/structure logs
   ↓
Model
   ↓
Classification + explanation
   ↓
GitHub comment / UI / report
```

AI should not affect whether the job is considered failed.

---

# 50. AI Triggering

Support eventual triggers such as:

```text
All failures
Specific repository
Specific workflow
Specific branch
Specific PR
Specific GitHub label
Specific tag
Manual command
```

Examples:

```text
label: ai-debug
```

or:

```text
workflow_failure + ai_enabled
```

AI remains OFF unless explicitly enabled.

---

# 51. Long-Term Product Direction

Do not prematurely build the following, but keep the architecture capable of supporting them:

```text
Multi-cloud scheduling
Managed runners
Customer-owned runners
Hybrid capacity
macOS runners
Predictive resource allocation
Predictive caching
Warm pools
Cost optimization
Failure intelligence
Flaky test detection
Workflow optimization
Security analysis
Runner image optimization
AI-powered CI debugging
```

The core platform should be useful even if every AI feature is disabled.

---

# 52. Important Engineering Rules

Follow these rules throughout implementation:

1. Investigate before implementing.
2. Do not over-engineer.
3. Use interfaces only at real boundaries.
4. Keep GitHub lifecycle independent from cloud providers.
5. Keep scheduler independent from AWS/GCP implementation.
6. Keep managed/self-hosted ownership separate from provider implementation.
7. Make runner lifecycle deterministic.
8. Make reconciliation mandatory.
9. Treat runners as disposable.
10. Never sacrifice security for startup speed.
11. Measure before optimizing.
12. Do not add Kubernetes unless justified.
13. Do not add EC2 Fleet unless justified.
14. Do not add AI to the critical path.
15. Do not add a vector database just because AI may exist later.
16. Do not force customers to use AI.
17. Keep model providers replaceable.
18. Keep storage providers replaceable where practical.
19. Collect useful structured telemetry from day one.
20. Prefer simple working architecture over theoretical abstractions.

---

# 53. After Every Phase

Report:

```text
## Investigation
What was learned.

## Implementation
What was implemented.

## Architecture
What changed.

## Files
Files created/modified.

## Tests
Tests added and results.

## Performance
Measurements.

## Security
Security considerations.

## Risks
Known problems.

## TODO
Remaining work.

## Recommendation
Recommended next phase.
```

Do not silently skip architectural decisions.

---

# 54. Start Now

Start with **Phase 0 only**.

Do not implement the runner yet.

Inspect the repository, environment, GitHub requirements, actual workflows, AWS/GCP options, security model, performance bottlenecks, and extension architecture.

Then produce the Phase 0 architecture and Phase 1 implementation plan.

The ultimate goal is:

> A fast, reliable, extensible ephemeral CI platform that can operate as customer-owned infrastructure, our managed CI service, or a hybrid of both — with multi-cloud provider support and optional AI/analytics extensions that can be enabled whenever the customer wants them.

Do not assume AI is the product.

AI is one optional capability built on top of a strong CI execution and data platform.