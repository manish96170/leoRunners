# TODO

## Phase 0 follow-up

- [ ] Choose the initial persistence implementation: SQLite versus DynamoDB-backed adapter.
- [ ] Confirm GitHub App installation scope and runner-group policy.
- [ ] Confirm webhook deployment target and secret-management service.
- [ ] Inspect and record the first real repository's workflow tool requirements.
- [ ] Decide initial AWS region, subnet model, AMI ownership, and network egress policy.

## Phase 1

- [x] Create `go.mod` and the controller command.
- [x] Implement domain state machine and transition tests.
- [x] Implement durable state interface and local adapter.
- [x] Implement event idempotency and replay handling.
- [x] Implement provider interface and configurable fake provider.
- [x] Implement scheduler capacity and policy checks.
- [x] Implement webhook signature verification and normalized events.
- [x] Implement reconciliation and reaper interfaces.
- [x] Add structured lifecycle events and timing fields.
- [x] Add controller integration tests for duplicate queued delivery.
- [ ] Add controller integration tests for cancellation, completion, and reaper paths.

## Later phases

- [x] AWS provider with Launch Template, RunInstances, tags, status, readiness, and termination.
- [x] AWS provider mocked API tests.
- [ ] AWS provider independent provisioning tests in a real account.
- [x] GitHub JIT API client and mocked HTTP tests.
- [x] Secure runner bootstrap contract and shell validation.
- [x] Runner assignment service with registration/cleanup contracts.
- [x] Independent high-reasoning code review and confirmed-finding fixes.
- [x] Atomic file-backed state and restart recovery.
- [x] Bounded GitHub API retries and cancellation-aware backoff.
- [x] Reconciliation/reaper runtime loop and expiry recovery.
- [x] Immutable AMI Packer template and base image scripts.
- [x] Runner image profile and versioning/rollback rules.
- [x] Offline startup benchmark harness and guarded real-AWS procedure.
- [x] Runtime-neutral workload manifest and evidence validation.
- [x] Manifest-driven runner preflight checker.
- [x] Repeatable workload command/evidence report harness.
- [x] Versioned benchmark evidence schema and comparability validator.
- [x] Baseline/candidate statistics and regression gate.
- [x] Transparent cost estimator with explicit assumptions.
- [x] GCP Compute Engine provider with mocked lifecycle tests.
- [x] Opt-in GCP controller wiring and deployment documentation.
- [x] Capacity-pool registry with ownership, limits, reserve/release, and deterministic selection.
- [x] Managed/hybrid scheduler integration and policy tests.
- [x] Deterministic historical/statistical intelligence.
- [x] Provider-agnostic AI failure-analysis boundary with deterministic policy gate.
- [x] AI-disabled/local/hosted config fixtures and redaction validation.
- [ ] Real telemetry-backed intelligence evaluation.
- [ ] Shared production capacity registry and multi-replica reservation leases.
- [ ] Real GCP VM lifecycle validation with ADC.
- [ ] Real JIT bootstrap and ephemeral runner validation.
- [ ] Derive a manifest from an approved real repository and fixed commit.
- [ ] Run candidate versus baseline workload on AWS/GitHub.
- [ ] Real Packer validation and AMI bake.
- [ ] Cold-start measurement on the exact published AMI.
- [ ] AMI pipeline and startup measurements.
- [ ] Real workload validation and benchmark report.
- [ ] GCP provider.
- [ ] Managed and hybrid capacity policies.
- [ ] Optional AI failure analysis, disabled by default.
- [x] Typed production runtime configuration and secret-safe diagnostics.
- [x] Graceful shutdown/readiness and offline smoke harness.
- [x] Non-root container and CI workflow assets.
- [x] Plain Kubernetes deployment assets with no RBAC and single-replica state guidance.
- [ ] Docker build and non-root image verification.
- [ ] Kubernetes client dry-run and disposable-cluster validation.
- [x] DynamoDB shared-state adapter with conditional writes and event idempotency.
- [x] Distributed controller lease/fencing-token coordination.
- [x] Opt-in DynamoDB runtime backend and shared-state documentation.
- [x] Terraform DynamoDB table/index module with encryption, TTL, recovery, and deletion protection.
- [x] Terraform controller/runner IAM scaffolding with policy-generation workflow.
- [ ] Terraform provider validation in the target architecture/environment.
- [ ] IAM policy refinement and simulation using real resource ARNs.
- [ ] DynamoDB table/index deployment and real two-replica failover test.
- [x] Terraform dev composition with explicit reviewed policy input.
- [x] Read-only cloud preflight and explicitly guarded live validation harness.
- [x] Cloud validation runbook and redacted evidence report path.
- [x] Backend-neutral structured observability events and secret-safe logger.
- [x] Prometheus exporter and controller `/metrics` endpoint.
- [x] CloudWatch EMF exporter with limits and dimension allowlist.
- [x] Provider-neutral cache interface, safe keys, TTL, invalidation, and telemetry.
- [x] In-memory cache backend with observe-only/disabled/enabled modes.
- [x] Immutable checksum-verified S3 cache backend with mocked tests.
- [ ] Real workload cache telemetry and S3 namespace validation.
- [x] Provider-neutral network profiles and reachability requirements.
- [x] AWS runner-network module with opt-in SG, endpoints, and NAT.
- [x] Network security/cost documentation and offline validation.
- [ ] Real VPC reachability and NAT versus endpoint measurement.
- [x] Deterministic fork/untrusted-workload security policy.
- [x] Local credential, privilege, RBAC, and AI-default security scanner.
- [x] Security policy fixtures and hardening documentation.
- [ ] Scan built image and rendered manifests in deployment environment.
- [ ] Configure production scraping, CloudWatch routing, alarms, and retention.
- [ ] Run real read-only AWS/GCP/GitHub preflight.
- [ ] Run one approved live validation and confirm cleanup.
