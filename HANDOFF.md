# Handoff

## Snapshot

The repository is a greenfield Go project. Phase 0 documentation and the Phase 1 core foundations are present. Go 1.27.1 was used from a temporary `/tmp` toolchain; the full test and race suites pass.

## Important decisions

- Go is the platform language.
- GitHub `workflow_job` is the lifecycle trigger.
- GitHub JIT configuration is preferred over long-lived runner registration credentials.
- Providers expose a cloud-neutral runner contract.
- Phase 1 uses a fake provider and a replaceable local persistence adapter.
- AWS Phase 2 uses Launch Template plus `RunInstances`, with launch-time ownership tags and IMDSv2 required.
- Reconciliation and maximum runner lifetime are mandatory.
- AI never controls provisioning, security, cancellation, or cleanup.
- The local executable exposes `/healthz` and signed `/webhooks/github` intake using a fake provider.
- Phase 2 AWS provider exists under `controller/internal/providers/aws/` and is covered by mocked EC2 tests.
- Phase 3 adds `controller/internal/github/jit.go`, `controller/internal/runners/`, `controller/internal/controller/jit_adapter.go`, and `bootstrap/runner/`.
- The controller can use an injected assignment service to pass transient JIT data into provider bootstrap metadata without persisting it.
- The requested high-reasoning review found several blockers; they were fixed and the full verification suite passes afterward.
- Phase 4 adds `state.FileRepository`, GitHub transient retries, `lifecycle.Reconciler`, `STATE_PATH`, and periodic `RECONCILE_INTERVAL` processing.
- Phase 5 adds `ami/runner.pkr.hcl`, base-image scripts, `ami/profiles/base-linux-x64.yaml`, and the standalone `tools/benchmark` module.
- Phase 6 adds `workloads/workload-manifest.v1.yaml`, `tools/workload-preflight`, `tools/workload-validation`, and `docs/phase-6-plan.md`.
- Phase 7 adds `benchmarks/`, `tools/benchmark-compare`, `tools/cost-estimator`, and `docs/phase-7-plan.md`.
- Phase 8 adds `controller/internal/providers/gcp/`, opt-in GCP executable wiring, `docs/gcp-provider.md`, `docs/phase-8-plan.md`, and `terraform/modules/runner-gcp/` documentation.
- Phase 9 adds `controller/internal/capacity/`, registry-backed scheduler reservations, and managed/hybrid capacity documentation.
- Phase 10 adds `controller/internal/intelligence/stats/`, `controller/internal/intelligence/ai/`, `intelligence/`, and `docs/phase-10-plan.md`.
- Phase 11 adds `controller/internal/config/`, `Dockerfile`, `.dockerignore`, `.github/workflows/ci.yml`, `tools/smoke/`, and `docs/production-readiness.md`.
- Phase 12 adds `deploy/kubernetes/controller.yaml` and its deployment guide.
  It deploys one non-root, read-only-root-filesystem controller with no
  Kubernetes RBAC, a cluster-internal Service, health probes, a state PVC, and
  only Secret references. The base configuration uses the fake provider and
  keeps GitHub JIT and AI disabled.
- The CI Docker build now uses the repository root rather than the global
  `controller` working directory.
- Phase 13 adds `controller/internal/state/dynamodb/`, `controller/internal/coordination/`, `DYNAMODB_TABLE_NAME` runtime selection, and shared-state/IAM documentation.
- Phase 14 adds Terraform modules under `terraform/modules/controller-state`, `controller-iam`, and `runner-runtime`, plus Autopilot policy-generation guidance.
- Phase 15 adds `terraform/environments/dev/`, `tools/cloud-validation/`, `docs/phase-15-plan.md`, and `docs/cloud-validation.md`.
- Phase 16 adds `controller/internal/observability/`, Prometheus `/metrics` wiring, CloudWatch EMF formatting, and `docs/phase-16-plan.md`.
- Phase 17 adds `controller/internal/cache/`, the optional S3 backend, `cache/`, and `docs/phase-17-plan.md`.
- Phase 18 adds `controller/internal/network/`, `terraform/modules/runner-network/`, `network/`, and `docs/phase-18-plan.md`.
- Phase 19 adds `controller/internal/security/`, `tools/security-validation/`, `security/`, and `docs/phase-19-plan.md`.
- Phase 20 adds `controller/internal/events/`, the optional JSONL archive, `events/`, and `docs/phase-20-plan.md`.
- Phase 21 adds `controller/internal/extensions/`, advisory policy handling, `extensions/`, and `docs/phase-21-plan.md`.
- Phase 22 adds the CloudWatch observability Terraform module, versioned alert
  fixtures and validator, and the operator runbook in
  `docs/observability-operations.md`.
- Phase 23 adds read-only analytics/cache/cost/security consumers, runtime
  extension metrics, extension alert fixtures, and `docs/extensions-operations.md`.
- Phase 24 adds typed extension runtime configuration, explicit tenant-scoped
  registration, runtime policy fixtures/validation, and `docs/extensions-deployment.md`.
- Phase 25 adds controlled-validation evidence schema/fixtures/validator,
  optional observability Terraform composition, and the evidence runbooks.
- Phase 26 adds guarded cloud-validation evidence generation for successful
  single-provider AWS/GCP runs, integration tests, and live-validation operations documentation.
- Phase 27 adds reviewed scope-file checks for cloud preflight, approved-scope
  fixtures/validation, and `docs/preflight-scope-operations.md`.
- Phases 28-35 add tenant isolation, archive retention, cache telemetry,
  CloudWatch routing, Docker/Kubernetes checks, IAM simulation, multi-replica
  failover tests, and the read-only release-validation gate.
- Phases 36-45 add cloud preflight/live-gate hardening, workload/image/network
  evidence, event-sink checks, intelligence evaluation, GitHub validation,
  Terraform checks, and the final readiness gate.
- Phases 46-54 add production config/secrets, GCP/GitHub scope validation,
  AMI and measurement evidence, multi-cloud comparison, artifact security,
  recovery evidence, and the final audit pack.

## Continue here

Read `docs/phase-1-plan.md`, then extend the current implementation in this order:

1. Run the read-only AWS/GCP/GitHub preflight and review target ownership.
2. Configure production retention and extension notification routes.
3. Run an approved live validation and validate the redacted evidence package
   with `tools/evidence-validation/test.sh`; use `--evidence-report` only with
   a single approved AWS or GCP live action. Supply a reviewed `--scope-file`
   and stop on any identity or scope mismatch.

Do not add AWS calls until the fake-provider end-to-end test passes.

## Known evidence gaps

- No real repository workflows are present in this workspace.
- No AWS or GCP account configuration is present.
- GitHub App ownership, runner-group policy, target organization, and webhook deployment are not yet supplied.
- Cold-start, job-duration, cache, and cost measurements do not exist yet.
- Real GitHub and cloud lifecycle validation remains outstanding.
- Packer itself is not installed in this environment; only offline AMI checks have run.
- No real repository/workflow was available, so Phase 6 reports are synthetic or local-command evidence only.
- Phase 7 outputs are synthetic fixtures until a real AWS/GitHub workload is collected.
- No GCP credentials or project configuration are present; GCP tests are mocked.
- Managed/hybrid pool configuration is currently process configuration; shared production registry and multi-replica leases remain future work.
- Intelligence fixtures are synthetic; no model endpoint is contacted and AI is disabled by default.
- Docker was unavailable locally, so the image build remains unverified here.
- `kubectl`, Kind, k3d, and Minikube are also unavailable locally, so Kubernetes
  client and cluster validation remain unverified. The local host is macOS
  arm64; Docker Desktop is required before image or local-cluster testing.
- CloudWatch resources have not been applied in this environment. Terraform
  provider validation remains environment-dependent; module formatting and
  offline alert validation pass. Configure a reviewed KMS key and notification
  destination before enabling production alarms.
