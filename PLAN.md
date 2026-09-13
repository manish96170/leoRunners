# Plan

## Current milestone: Phase 19 security hardening

Status: deployment assets complete; local container and cluster validation blocked

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

### Next

1. Run security validation against the exact built image and deployment manifests.
2. Review IAM/OIDC and untrusted-fork policy in a controlled environment.
3. Perform one approved live cloud/workload validation with security evidence.

### Guardrails

- AI is optional and disabled by default.
- No real cloud provisioning in Phase 1.
- Kubernetes remains an optional deployment target, not a controller dependency.
- No Kubernetes provider, EC2 Fleet, warm pools, or vector database until
  measurements justify them.
- Runner termination and security decisions remain deterministic.
