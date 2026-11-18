# Plan

## Current milestone: Phase 61 controlled workload validation

Status: implementation complete; approved live cloud validation remains operator- and environment-gated

The specification requires investigation and architecture documentation before runner implementation. The initial implementation language is Go.

### Completed

- Confirmed the repository is greenfield and contains no existing CI workflows.
- Confirmed Go for the controller, lifecycle, scheduler, and provider implementations.
- Verified the GitHub JIT runner API and AWS Launch Template/RunInstances direction against official documentation.
- Created the Phase 0 architecture documents under `docs/`.
- Added Go Phase 1 foundations under `controller/`.
- Added event-to-job/lease/provider orchestration and a local HTTP entrypoint.
- Verified formatting, unit tests, and race tests with Go 1.27.1 from `/tmp`.
- Added an AWS EC2 provider using Launch Template plus `RunInstances`, IMDSv2 enforcement, ownership tags, status polling, and idempotent termination.
- Added AWS provider tests with a mocked EC2 API and AWS operational/IAM documentation.
- Added the GitHub JIT API client, secure bootstrap script, runner assignment service, registration verifier contract, and controller adapter.
- Verified shell bootstrap behavior and the full Go module after Phase 3 integration.
- Added atomic file-backed state with restart/backup recovery tests.
- Added bounded GitHub retries and deterministic reconciliation/reaping.
- Wired optional `STATE_PATH` persistence and periodic runtime reconciliation.
- Verified full tests, race tests, vet, build, shell tests, and diff checks.
- Completed an independent `gpt-5.6-sol` high-reasoning code review and incorporated its confirmed findings.
- Re-verified Go tests, race tests, vet, build, shell tests, and whitespace checks after fixes.
- Added immutable Amazon Linux Packer assets, minimal `base-linux-x64` profile, and offline AMI validation.
- Added a standalone benchmark harness with deterministic fake timelines and guarded real-AWS procedure.
- Added a runtime-neutral workload manifest with evidence-driven capabilities and cache observations.
- Added manifest-driven local preflight checks for tools, Docker, browsers, disk, memory, and architecture.
- Added repeatable workload command validation with redacted logs and JSON/Markdown evidence reports.
- Verified manifest, tool tests, race tests, vet, fake reports, and offline checks.
- Added versioned benchmark evidence schema and baseline/candidate comparability validation.
- Added repeated-sample comparison with min/median/p95/max and regression thresholds.
- Added transparent itemized cost estimation with explicit assumptions and estimate labels.
- Verified comparison, cost, schema, controller, workload, and AMI checks.
- Added GCP Compute Engine provider using instance templates, zonal operations, labels, readiness/status mapping, idempotent deletion, and request IDs.
- Wired opt-in GCP provider selection into the controller executable and added GCP deployment documentation.
- Verified the GCP provider with tests, race tests, vet, root build, and root tests.
- Added capacity-pool registry with customer/managed ownership, provider/region/security metadata, limits, pricing/startup metadata, and atomic reserve/release.
- Integrated registry-backed selection and reservations into the scheduler and JIT assignment flow.
- Added managed/hybrid deployment documentation and policy coverage.
- Verified capacity, controller, provider, GCP, and full root tests with race detection.
- Added deterministic historical statistics for timings, failures, caches, providers, and resources.
- Added provider-agnostic AI failure-analysis contracts with disabled-by-default configuration, bounded logs, trigger matching, and deterministic action policy.
- Added redacted AI config/event fixtures and offline validation.
- Verified intelligence tests, race tests, vet, build, fixture validation, and diff checks.
- Added typed runtime configuration with provider/JIT/capacity/AI validation and secret-safe diagnostics.
- Added graceful HTTP shutdown and offline health/webhook smoke validation.
- Added non-root multi-stage Dockerfile, CI workflow, and production-readiness documentation.
- Verified controller tests, race tests, vet, build, smoke tests, and diff checks.
- Added a hardened, plain Kubernetes single-replica deployment with a private
  Service, no RBAC, ConfigMap/Secret references, file-state PVC, probes, and
  resource limits.
- Corrected the CI Docker build to use the repository-root build context.
- Re-verified Go tests, race tests, vet, build, offline smoke, intelligence,
  workload, AMI, and Kubernetes YAML syntax checks.
- Added DynamoDB-backed state with conditional revisions, idempotent lifecycle events, sparse state/expiry indexes, and transaction-safe event markers.
- Added distributed TTL leases with monotonic fencing tokens and stale-owner protection.
- Wired opt-in `DYNAMODB_TABLE_NAME` repository selection into the controller.
- Verified DynamoDB, coordination, controller, and full root suites with race detection, vet, build, and diff checks.
- Added reusable Terraform DynamoDB state module with GSIs, TTL, recovery, encryption, deletion protection, and outputs.
- Added controller IAM trust/policy attachment scaffolding and runner instance-profile module.
- Ran `iam-policy-autopilot` against runtime Go sources with Terraform context; generated policy remains review-only.
- Added DynamoDB runtime fields to typed configuration and corrected TTL naming to `expires_at`.
- Added Terraform dev composition for state and IAM/runtime modules with explicit reviewed-policy input.
- Added non-destructive AWS/GCP/GitHub preflight and guarded live-validation harness.
- Added cloud validation runbook, cleanup safeguards, redacted reports, and phase handoff.
- Verified cloud-validation mocks, Terraform static checks, full Go suites, and existing offline checks.
- Added structured observability events, correlation metadata, bounded in-memory metrics, and recursive secret redaction.
- Added Prometheus exposition and CloudWatch EMF exporters with low-cardinality/size safeguards.
- Wired lifecycle metrics into the controller `/metrics` endpoint and fanout sink.
- Verified observability tests, race tests, vet, build, cloud-validation, and existing offline checks.
- Added content-addressed cache keys and a backend-neutral cache interface.
- Added disabled, observe-only, and enabled modes with TTL, size, count, invalidation, and cache telemetry.
- Added a checksum-verified immutable S3 backend with encryption headers and mocked tests.
- Verified cache and S3 tests, race tests, vet, build, and all existing project checks.
- Added provider-neutral network profiles, egress modes, endpoint requirements, security posture validation, and fake reachability resolution.
- Added AWS runner-network Terraform module with reference IDs by default and opt-in security group, VPC endpoint, and NAT resources.
- Added public/private subnet, DNS, egress, cost, and fork-isolation documentation.
- Verified network tests, race tests, vet, build, Terraform formatting/offline guards, and full existing checks.
- Added deterministic trust/profile security policy for trusted branches, internal PRs, fork PRs, and untrusted workloads.
- Added local read-only security scanner for credentials, privileged containers, host access, RBAC, and AI defaults.
- Added security policy fixtures and fork/IAM/JIT/Docker/network hardening documentation.
- Verified security policy, scanner, fixture, race, vet, build, and all existing project checks.
- Added versioned CI event envelopes with deterministic IDs, redaction, validation, deduplication, bounded subscriptions, and backpressure metrics.
- Added durable owner-permission JSONL event archive with replay/filtering and restart-safe append behavior.
- Wired lifecycle telemetry into the event bus and optional `EVENT_ARCHIVE_PATH` runtime sink.
- Verified event bus/archive tests, race tests, vet, build, fixtures, and full existing project checks.
- Added versioned extension metadata and advisory output fixtures with offline validation.
- Added extension registry, bounded dispatch queues, per-extension timeouts, panic/failure isolation, and metrics snapshots.
- Added deterministic advisory action policy and tenant/extension enablement checks.
- Verified extension tests, race tests, vet, build, fixture validation, and full project checks.
- Added the Phase 22 CloudWatch observability Terraform module with retained logs,
  optional KMS encryption, notification-only M-of-N alarms, p99 duration
  monitoring, and an alarm-first dashboard.
- Added versioned low-cardinality alert configuration fixtures and offline
  secret/cardinality/action validation with pass, warning, and failure exits.
- Added the observability operations runbook covering triage, missing data,
  p99 interpretation, escalation, rollback, retention, and cleanup evidence.
- Added read-only analytics, cache, cost, and security extension consumers with
  bounded sinks and advisory-only security output.
- Added dispatcher emission of bounded extension execution, timeout, panic,
  queue-drop, and report-drop metrics through the existing observability sink.
- Added extension failure alert fixtures and validation for notification-only
  actions and low-cardinality dimensions, plus the Phase 23 operations runbook.
- Added typed extension runtime configuration with disabled-by-default global
  enablement, exact tenant allowlists, bounded queue/timeout/report settings,
  and secret-safe diagnostics.
- Added runtime registration for the built-in read-only consumers behind the
  tenant policy adapter, and wired extension metrics into `/metrics`.
- Added versioned runtime policy fixtures and offline validation for wildcard
  grants, known consumers, bounds, capabilities, redaction, and secrets.
- Added the Phase 24 deployment guide and rollout/rollback procedures.
- Added a versioned controlled-validation evidence schema and redacted offline
  validator covering provider checkpoints, observability, cleanup proof,
  timestamps, bounded references, secrets, and raw payloads.
- Added an explicitly opt-in observability module composition to the Terraform
  dev environment, with disabled-by-default logs, alarms, and dashboard.
- Added the Phase 25 controlled-validation evidence and approval runbooks.
- Added schema-compatible evidence generation to the guarded cloud-validation
  harness for single AWS or GCP live actions.
- Required successful validation, ordered lifecycle checkpoints, and confirmed
  cleanup before evidence is atomically written; failed or incomplete runs
  cannot publish passed evidence.
- Added focused cloud/evidence integration tests and the Phase 26 live
  validation operations runbook.
- Added reviewed scope-file support to cloud validation with read-only AWS
  account/region, GCP project/zone, and GitHub repository matching.
- Added fail-closed scope mismatch handling before any live mutation, with
  secret-safe output and malformed/duplicate/unsupported scope tests.
- Added approved-scope fixtures and the Phase 27 preflight operations runbook.
- Completed Phase 28 tenant-isolation tests for enabled, disabled, missing, and
  alternate tenant attributes, including advisory tenant preservation.
- Completed Phase 29 bounded JSONL archive retention, rotation, permissions,
  restart recovery, and retention-policy validation.
- Completed Phase 30 tenant/namespace-separated cache telemetry, hit-rate
  summaries, immutable-key checks, and S3 namespace validation.
- Completed Phase 31 optional CloudWatch subscription routing with explicit
  destination/filter inputs, encryption and retention gates, and no remediation.
- Completed Phase 32 Docker/Kubernetes security and rendered-manifest checks,
  with clear offline skips when Docker or kubectl is unavailable.
- Completed Phase 33 offline IAM policy simulation for scoped resources,
  constrained PassRole, wildcard rejection, and review-only application.
- Completed Phase 34 multi-replica fenced reservation, takeover, and capacity
  conservation tests.
- Completed Phase 35 read-only release validation aggregation with redacted,
  atomic reports and live-provisioning detection.
- Completed Phase 36 read-only cloud preflight identity/reporting improvements.
- Completed Phase 37 pinned AWS validation inputs, bounded timeouts, cleanup
  discovery, and fail-closed live-action tests.
- Completed Phase 38 fixed-workload and benchmark provenance/comparability
  validation for synthetic and real evidence.
- Completed Phase 39 guarded AMI image contract and Packer validation.
- Completed Phase 40 network reachability matrices and guarded NAT/endpoint
  measurement checks.
- Completed Phase 41 event sink/archive contracts for redaction, retention,
  replay, idempotency, permissions, and drops.
- Completed Phase 42 deterministic telemetry-backed intelligence evaluation
  with AI policy-boundary enforcement.
- Completed Phase 43 offline GitHub JIT, runner registration, bootstrap,
  cancellation, cleanup, and untrusted-workload validation.
- Completed Phase 44 offline Terraform provider/module/IAM dependency checks
  with explicit unavailable-provider status.
- Completed Phase 45 read-only final readiness aggregation and report output.
- Completed Phase 46 production secret/configuration contracts and Kubernetes
  secret-reference validation.
- Completed Phase 47 guarded GCP project/zone/template/label lifecycle checks.
- Completed Phase 48 GitHub organization/repository, webhook, JIT, runner-group,
  registration, cleanup, and fork-isolation validation.
- Completed Phase 49 AMI cold-start checkpoint benchmarks with percentile,
  timeout, regression, provenance, and digest evidence.
- Completed Phase 50 cache/network/startup/cost measurement evidence contracts.
- Completed Phase 51 deterministic AWS/GCP performance and cost comparison with
  provenance and missing-provider warnings.
- Completed Phase 52 built-image and rendered-manifest security scanning.
- Completed Phase 53 disaster-recovery, replay, fencing, reaping, and takeover
  evidence validation.
- Completed Phase 54 final architecture and production-readiness audit pack.
- Completed Phase 55 versioned activation requests, owner/approver/scope/deadline
  validation, and secret/wildcard rejection.
- Added a read-only production-preflight handoff wrapper that requires the
  reviewed activation request and scope file, blocks live flags, and emits a
  redacted PASS/BLOCKED handoff report.
- Added the Phase 55 production activation runbook.
- Reconciled stale TODO entries against completed implementation phases and
  separated offline contracts from genuinely environment-gated work.
- Added controller integration coverage for cancellation cleanup and expired
  runner reaping, including durable lease/runner assertions.
- Added independent AWS provider contract validation, mocked lifecycle
  regressions, pinned launch inputs, readiness/timeout checks, and a guarded
  live-EC2 procedure.
- Added independent GCP provider contract validation, mocked lifecycle
  regressions, project/zone/template checks, readiness/timeout checks, and a
  guarded live-GCP procedure.
- Added fixed-commit workload intake contracts, offline manifest derivation,
  secret/floating-ref/command-boundary rejection, and controlled-validation
  handoff documentation.
- Added controlled workload execution contracts and a bounded detached-worktree
  runner with command allowlists, timeout/output limits, network refusal,
  redaction, and atomic owner-only evidence.
- Incorporated the high-effort implementation review remediation for durable
  webhook processing, target-specific live scope, GCP JIT fail-closed startup
  metadata, provider-specific cleanup, fork/label admission, numeric DynamoDB
  TTL, stable provider identities, and host-workload refusal.

### Next

1. Run the approved read-only AWS/GCP/GitHub preflight with the reviewed scope file.
2. Configure production retention, notification routes, and tenant rollout.
3. Perform one approved live cloud/workload validation and publish validated evidence.

### Guardrails

- AI is optional and disabled by default.
- No real cloud provisioning in Phase 1.
- Kubernetes remains an optional deployment target, not a controller dependency.
- No Kubernetes provider, EC2 Fleet, warm pools, or vector database until
  measurements justify them.
- Runner termination and security decisions remain deterministic.
