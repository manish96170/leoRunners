# TODO

## Customer onboarding track (AWS + GitHub, self-hosted)

Goal: a customer with a fresh AWS account and a GitHub org can install, run a
first job, operate, and remove the runner from the docs alone. Docs are written
first as the target experience, then the gaps they expose are built.

### 0. Decisions

- [x] License: PolyForm Noncommercial 1.0.0 (`LICENSE`).
- [x] Name: keep `leoRunners` for now (revisit before public release).
- [x] v1 scope: AWS + GitHub only; GCP and multi-cloud stay as later work.
- [x] Controller runtime on AWS: ECS Fargate behind an ALB (no Kubernetes needed).

### 1. Docs

- [x] 1.1 Root `README.md`.
- [ ] 1.2 `docs/aws-quickstart.md` (GitHub App, AWS prerequisites, deploy, webhook, first job).
- [ ] 1.3 `docs/configuration.md` (environment variable reference).
- [ ] 1.4 `docs/architecture-overview.md` (customer-facing architecture and security model).
- [ ] 1.5 `docs/operations.md` (monitoring, upgrades, troubleshooting, cost).
- [ ] 1.6 `docs/uninstall.md`.
- [ ] 1.7 `docs/maintainers/README.md` index for phase plans and validation runbooks.

### 2. Build the gaps the docs expose

- [x] 2.2a Generic AWS modules: `aws-ecs-controller`, `aws-shared-gateway`, `aws-ephemeral-runner` (contract only), example composition, mocked-provider tests (`terraform/validate-aws-modules.sh`).
- [x] 2.2b Module docs: `docs/aws-modules.md`, image publication, IAM policy workflow, state and secrets, rollback and cleanup, GitHub runtime contract.
- [x] 2.2g Generic AWS foundation environment (`terraform/environments/aws-foundation`) and shared gateway design note (`docs/aws-shared-gateway-design.md`).
- [x] 2.1 GitHub App auth in the controller: App ID + installation ID + private key, short-lived installation tokens through the `BearerTokenSource` seam, with Terraform, Kubernetes and docs support (`docs/github-runtime-contract.md`). Tested against a fake GitHub server only.
- [x] 2.1a GitHub App auth exercised against github.com: installation tokens minted from the stored key, JIT runners created and registered, jobs run on them (local rehearsal against a throwaway repository, repository scope).
- [x] 2.1b Organization-level JIT (`GITHUB_RUNNER_SCOPE=organization`, Terraform `github.runner_scope`): the App needs the narrower organization Self-hosted runners permission instead of repository Administration. Fake-server tests only; not yet run against github.com (the first live run used repository scope, see 2.1a). Kept checked because the implementation is done; the live check is still open.
- [ ] 2.1c Serve several App installations from one controller (installation ID per event, allowlisted).
- [x] 2.1d `GITHUB_JIT_REQUEST_TIMEOUT`, `GITHUB_REGISTRATION_TIMEOUT` and `GITHUB_REGISTRATION_POLL_INTERVAL` are applied by the executable. Found on the first real run, where a loaded local machine needed more than the fixed 10 minute registration wait.
- [x] 2.2h Modules validate on AWS provider v5 and v6 (region read by key; `validate-aws-modules.sh --provider-version`). Found by a customer composition on v5.
- [x] 2.2c Runner layer: `aws-runner-layer` (launch template, runner role and instance profile, ingress-free security group) behind the `aws-ephemeral-runner` contract; controller `RUNNER_MAX_LIFETIME`, `AWS_RUNNER_TAGS`, user data and runner cap wired. Offline only; real EC2 launch still to confirm (2.5d).
- [ ] 2.2d Publish an official controller image (digest list per release).
- [ ] 2.2e Wire multi-replica coordination into the executable, then relax `desired_count`.
- [x] 2.2f Acknowledge webhooks quickly and orchestrate asynchronously (`controller/internal/dispatch`; measured 16s on a cold launch against an emulator before the change). A durable queue is a possible follow-up.
- [ ] 2.3 GitHub App manifest for one-click App creation.
- [ ] 2.4 Job-level sizing labels in `runs-on:` (confirm whether label parsing exists).
- [x] 2.5a Local rehearsal harness (`tools/local-rehearsal`): floci + controller + Cloudflare quick tunnel; stage 1 (webhook intake, launch, terminate, restart safety) verified. The Docker VM used by the rehearsal needs about 15 GB free; leftover anonymous Docker volumes from unrelated containers can fill it.
- [x] 2.5b Generic runner user-data template (`bootstrap/runner/user-data.sh`) with a pinned, checksum-verified runner download.
- [x] 2.5e Reaper verified against the emulator; found and fixed two ways a runner could be orphaned (table TTL on the reaping deadline; no provider-side discovery). See `docs/aws-provider.md`.
- [x] 2.5f `tools/launch-check`: read-only evidence check for a launched runner (public IP, IMDS, tags on instance and volumes, encryption, shutdown behaviour); offline-tested with a stub.
- [x] 2.5g `examples/validation-workload`: a small front-end project and PR-only workflow that exercises what real repositories ask of a runner (exact Node, Yarn cache, lint shards, hidden-file artifacts, legacy amd64-only Terraform with teardown, Cypress, optional Docker). Ran in the local rehearsal (see 2.5c): lint, both unit shards and the build passed; the integration job did not run (see 2.5i).
- [x] 2.5c A real workflow ran on LeoRunners runners from the local rehearsal: lint (exact Node 26.8.2, Yarn cache, actionlint, Biome, custom lint), both unit shards, and a build with a hidden-file artifact all passed on controller-launched runners. Found and fixed six defects on the way (PRs #15-#20). The integration job (legacy Terraform, Cypress, teardown) did not run: it needs real x64 hardware (2.5i).
- [x] 2.5h Minted installation tokens are restricted to the allowlisted repositories (`GITHUB_APP_TOKEN_REPOSITORIES`, derived from the repository allowlist by default), so an App installed on other repositories cannot be used on them through the controller.
- [ ] 2.5i Run the integration job (legacy amd64-only Terraform, Cypress on Chrome, Docker, temporary environment and teardown) on a real x64 EC2 runner. The emulator's arm64 containers cannot run it and boot too slowly to be a fair test. Planned on the front-end image (2.6b to 2.6d), with the sample repository only; keep `tools/launch-check` output as evidence.
- [ ] 2.5j Release idle runners sooner: a runner created for a job cancelled while queued now waits for another job or for its lifetime (`RUNNER_MAX_LIFETIME`), because runners are single-use and interchangeable and the controller no longer terminates the runner "belonging" to a finished job.
- [ ] 2.5d End-to-end smoke test against a disposable AWS account and test repo. The first live-EC2 test uses the sample repository (`examples/validation-workload`) only, never a customer production repository. Nothing on real EC2 or real DynamoDB is proven yet.

### 2.6 Runner images and routing

Decisions: the AMI recipe, extension hook and profile format are generic and live here. Builds, extension environment values (tool versions, URLs, checksums), AMI IDs, retention and approvals live in the customer infrastructure repository. Prefer a few profile images grouped by kind of workload over one image per repository. Two images are planned: the generic base image, and a front-end image with Chrome and Docker.

- [ ] 2.6a Build the generic base AMI (`base-linux-x64`) in a customer account, with `extra_tags` for request ID and cleanup deadline (customer side).
- [ ] 2.6b Finish and merge the second AMI profile `frontend-browser-linux-x64` (parent `base-linux-x64`): extension `ami/extensions/frontend-browser.sh` (pinned Chrome by URL and SHA-256, libraries Cypress needs, Docker, optional pinned Terraform), profile file, derived-profile checks in `tools/ami-validation/validate.sh`, and bootstrap adding the runner user to the `docker` group only when the image already has that group. In progress on a branch, not merged.
- [ ] 2.6c Build the front-end AMI (customer side), with Chrome pinned by a checksum-verified RPM kept in the customer's own storage.
- [ ] 2.6d Run the sample repository on the front-end image on real x64 EC2 and keep `tools/launch-check` output as evidence. This is the same test as 2.5i.
- [ ] 2.6e Label-to-image routing: design and build. The controller supports one launch template per deployment, so routing several labels to several images is not built.
- [ ] 2.6f Decision, not code: after about three more front-end repositories pass on the front-end image, retire the base image unless some repository still needs it.
- [ ] 2.6g A durable webhook queue. The queue is in memory today (see 2.2f), so an accepted event can be lost if the process dies.

Notes on other open items: the published controller image with a digest per release is 2.2d; multi-replica coordination is 2.2e; releasing idle runners sooner is 2.5j.

### 3. Validate and correct

- [ ] 3.1 Run the quickstart from scratch on a clean AWS account and test org.
- [ ] 3.2 Fix docs where they disagree with reality; remove "needs build" markers.

## Phase 0 follow-up

- [x] Choose the initial persistence implementation: DynamoDB-backed adapter.
- [x] Confirm GitHub App installation scope and runner-group policy contract.
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
- [x] Add controller integration tests for cancellation, completion, and reaper paths.

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
- [x] Real telemetry-backed intelligence evaluation harness and fixtures.
- [x] Shared production capacity registry and multi-replica reservation leases.
- [ ] Real GCP VM lifecycle validation with ADC.
- [ ] Real JIT bootstrap and ephemeral runner validation.
- [ ] Derive a manifest from an approved real repository and fixed commit.
- [ ] Run candidate versus baseline workload on AWS/GitHub.
- [ ] Real Packer validation and AMI bake.
- [ ] Cold-start measurement on the exact published AMI.
- [ ] AMI pipeline and startup measurements.
- [ ] Real workload validation and benchmark report.
- [x] GCP provider.
- [x] Managed and hybrid capacity policies.
- [x] Optional AI failure analysis boundary, disabled by default.
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
- [x] Offline Terraform provider/module validation; target-environment validation remains open.
- [x] Offline IAM policy refinement and simulation; real ARN simulation remains open.
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
- [x] Versioned CI event envelope with deterministic IDs and redaction.
- [x] Bounded event bus with deduplication and backpressure metrics.
- [x] Durable redacted JSONL archive with replay/filtering.
- [x] Production-shaped event sink, archive retention, and extension consumers.
- [x] Versioned extension metadata/advisory fixtures and validator.
- [x] Bounded extension registry/dispatcher with failure isolation.
- [x] Deterministic advisory action policy and enablement checks.
- [x] Production-shaped extension consumers and failure alerting.
- [x] CloudWatch log group, retention, optional KMS encryption, notification-only alarms, and dashboard module.
- [x] Versioned low-cardinality observability alert fixtures and offline validator.
- [x] Observability operations runbook with triage, missing-data, p99, escalation, rollback, and cleanup procedures.
- [x] Configure production scraping, CloudWatch routing, alarms, and retention contracts.
- [x] Read-only analytics, cache, cost, and security extension consumers.
- [x] Runtime extension failure, timeout, panic, queue-drop, and report-drop metrics.
- [x] Extension failure alert fixtures and notification-only validation.
- [x] Extension operations runbook and rollback procedures.
- [x] Typed extension runtime configuration and disabled-by-default startup behavior.
- [x] Explicit tenant-scoped registration and advisory policy wiring.
- [x] Runtime policy fixtures and offline validation for bounds and wildcards.
- [x] Extension deployment guide and rollout/rollback procedures.
- [x] Register consumers behind explicit tenant policy and validate tenant isolation offline.
- [x] Versioned controlled-validation evidence schema and redacted validator.
- [x] Positive, secret, raw-payload, and missing-file evidence fixtures.
- [x] Explicitly opt-in observability Terraform composition in dev environment.
- [x] Controlled-validation evidence approval, cleanup, and rollback runbook.
- [x] Guarded cloud-validation generation of schema-compatible evidence reports.
- [x] Cloud/evidence integration tests rejecting failed or incomplete cleanup.
- [x] Reviewed provider scope-file checks before live cloud mutations.
- [x] Approved-scope schema, positive/unsafe fixtures, and offline validation.
- [x] Preflight scope operations and mismatch/abort runbook.
- [x] Tenant isolation validation across extension runtime paths.
- [x] Event archive retention, rotation, and owner-only permission validation.
- [x] Cache tenant/namespace telemetry and S3 namespace validation.
- [x] Optional CloudWatch subscription routing and offline safety checks.
- [x] Docker/Kubernetes security and rendered-manifest validation.
- [x] IAM policy refinement and offline simulation checks.
- [x] Multi-replica capacity fencing, takeover, and conservation tests.
- [x] Read-only final release-validation gate and redacted report.
- [x] Read-only preflight identity and evidence reporting.
- [x] Pinned AWS validation template, bounded cleanup, and mutation guards.
- [x] Fixed workload/benchmark provenance and comparability checks.
- [x] Guarded AMI/Packer image contract validation.
- [x] Network reachability and NAT/endpoint measurement validation.
- [x] Event sink/archive retention, replay, idempotency, and redaction checks.
- [x] Telemetry-backed intelligence evaluation with policy boundaries.
- [x] GitHub JIT/bootstrap/registration and untrusted-workload validation.
- [x] Terraform provider/module/IAM offline validation.
- [x] Final production-readiness gate and report.
- [x] Production configuration/secret contract and Kubernetes reference validation.
- [x] Guarded GCP lifecycle validation.
- [x] GitHub production scope, webhook, JIT, and fork isolation validation.
- [x] AMI cold-start and startup measurement evidence.
- [x] Cache/network/startup/cost measurement evidence.
- [x] Multi-cloud performance and cost comparison.
- [x] Built-image and rendered-manifest security scanning.
- [x] Disaster-recovery, replay, fencing, and failover evidence.
- [x] Final architecture and production-readiness audit pack.
- [x] Versioned production activation request and approval validation.
- [x] Read-only production-preflight handoff and mutation guards.
- [x] Production activation runbook and abort/retention procedures.
- [x] Controller cancellation/completion/reaper integration coverage.
- [x] Independent AWS provider mocked lifecycle and guarded validation harness.
- [x] Independent GCP provider mocked lifecycle and guarded validation harness.
- [x] Approved fixed-commit workload intake and redacted manifest derivation.
- [x] Controlled fixed-commit workload execution with bounded commands and evidence.
- [ ] Run real read-only AWS/GCP/GitHub preflight.
- [ ] Run one approved live validation and confirm cleanup.
