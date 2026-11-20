# Handoff

## Read this first (current state, 2026-11-20)

LeoRunners is a generic, self-hosted, ephemeral GitHub Actions runner platform
(controller in Go, AWS EC2 provider, Terraform modules). The repository is public
and personally owned. The goal now is to make it **customer-ready on AWS** and to
prove it with a real pull-request job.

`main` is at the commit after PR #6. The only tag is `tf-aws-v0.1.0`, which is
older than PRs #4 to #6 (use a commit SHA until a new tag is cut after the first
live run).

### Ground rules

- **Nothing company- or customer-specific goes in this repo.** No names,
  domains, account IDs, user home paths or customer defaults. Every
  environment-specific value is a variable. Customer config lives in separate
  infrastructure repos, which consume the modules from a pinned ref. Do not touch
  any other repo.
- **Git identity** is `Manish Sharma <30901306+manish96170@users.noreply.github.com>`
  (set locally in this repo). The global git email is a company address and must
  never appear here. Check `git config user.email` before committing and
  `git log --format='%ae %ce'` before pushing.
- **Push, PR and merge** with only the `manish96170` GitHub login, per command,
  without changing global git or gh config:
  `export GH_TOKEN=$(gh auth token --user manish96170)` and a credential-helper
  override for `git push` (`-c credential.https://github.com/.helper=` then a
  helper that returns `username=manish96170` and `password=$GH_TOKEN`). The
  default `gh` account on the machine is a company one.
- **Workflow:** branch, commit, push, PR, wait for the 4 CI checks (Unit tests,
  Race tests, Vet, Build and container checks), squash-merge with
  `--delete-branch`, sync local `main`. Scan for company markers before every
  commit.
- **No real AWS without explicit instruction.** The machine has company AWS
  profiles; never use them. Everything local runs against the floci emulator.
- **Secrets** (GitHub App private key, tokens) never go in the repo, logs or the
  chat. The key goes to the emulator's Secrets Manager through
  `tools/local-rehearsal/store-app-key.sh`.
- License is PolyForm Noncommercial 1.0.0.

### What exists and is verified

- **GitHub App authentication** (App ID, installation ID, private key; short-lived
  installation tokens), Terraform, Kubernetes and docs. Mutually exclusive with a
  static installation token. One controller serves one installation. Tested only
  against a fake GitHub server so far.
- **Async webhook acknowledgement** (`controller/internal/dispatch`): the webhook
  returns `202` in about a millisecond; provisioning runs in the background
  (`WEBHOOK_WORKERS`, `WEBHOOK_QUEUE_SIZE`, `WEBHOOK_EVENT_TIMEOUT`, metric
  `webhook_events_total`). The queue is in memory.
- **Reaper fixes** (PR #6): the state table's TTL is `ttl_at` (seven days after
  `expires_at`, the logical expiry that drives reaping), and the reconciler has a
  provider-side backstop (`providers.ManagedLister`; AWS lists by the
  `leo-runners:managed-by` tag) that terminates owned instances more than 10
  minutes past their `leo-runners:expires-at` tag, with or without a record. An
  instance with no readable expiry tag or not tagged as ours is never removed.
  The GCP provider does not implement the backstop.
- **AWS Terraform:** `terraform/modules/aws-ecs-controller`,
  `aws-shared-gateway`, `aws-ephemeral-runner` (contract only), `controller-state`,
  `terraform/environments/aws-foundation`, example in `terraform/examples/aws-customer`.
  Validated by `terraform/validate-aws-modules.sh` and
  `tools/terraform-validation/validate.sh`.
- **Runner user-data template:** `bootstrap/runner/user-data.sh` (pinned,
  SHA-256-verified runner download; `pin-latest-runner.sh` refreshes the pin).
  Production should pre-bake the runner into the AMI instead.
- **Local rehearsal harness:** `tools/local-rehearsal/` (README has the full
  procedure). Stage 1 (no GitHub) is verified: bad signature `401`, queued `202`
  immediately even on a cold launch, duplicates and restarts safe, completed
  terminates, SIGTERM drains, a signed webhook through a real Cloudflare quick
  tunnel works, other paths return `404` via `webhook-gate.py`.
  `check-reaper.sh` verifies the reaper cases (about two minutes).
- **Migration check for an existing private infra repo:** the generic
  `aws-ecs-controller` / `aws-shared-gateway` modules were planned and applied on
  the emulator over copies of an older, company-specific composition using
  `moved` blocks. Result: no real resource deleted without a replacement (task
  definition and ECR lifecycle policy are replaced; table gets deletion
  protection and the new TTL attribute). Details were handed to that repo's own
  session; feedback from it is the likely input for this session.

### How to run the checks

```sh
cd controller && go vet ./... && go test -race -count=1 ./...        # 32 packages
tools/local-rehearsal/test.sh                                         # harness self-test
for t in bootstrap/runner/test.sh tools/config-validation/test.sh deploy/kubernetes/test.sh \
         tools/smoke/test.sh tools/terraform-validation/test.sh; do bash $t; done
terraform/validate-aws-modules.sh
tools/local-rehearsal/floci.sh up && tools/local-rehearsal/setup.sh && tools/local-rehearsal/check-reaper.sh
```

For concurrency changes also run `go test -race -count=60 ./internal/dispatch`;
it once caught a flaky hang in a test.

### Local environment facts (floci)

- Rehearsal state lives in `~/.leo-runners-rehearsal` (state, logs, built
  binary). The floci container is `leo-floci` on `127.0.0.1:4566`, account
  `000000000000`. `tools/local-rehearsal/down.sh` tears it down.
- Instances are Docker containers; user-data runs 30 to 60 seconds after launch.
  Use arm64 AMIs (`ami-amazonlinux2023-arm64`) on Apple Silicon. Amazon Linux 2023
  is not supported by the runner's `installdependencies.sh`, so the template
  installs dependencies with dnf. Colima mounts only `$HOME`; use `docker cp`
  instead of bind mounts from temp paths.
- Override the webhook secret with `LEO_WEBHOOK_SECRET` (not `WEBHOOK_SECRET`).
- The shell tool rejects some dynamic quoting; put complex logic in a script file.

### Next steps, in order

1. **Apply feedback** from the infra-repo session if there is any (a module gap,
   a missing input, an unexpected replacement). Fix in this repo generically,
   test, PR.
2. **Stage 2: a real PR job** (needs the owner): create the GitHub App in a
   browser (settings in `tools/local-rehearsal/README.md` "Stage 2": Repository
   Administration read/write, Actions read-only, Workflow job event, installed on
   the test repository only), choose a test repo (ideally in a test org), then
   `store-app-key.sh`, `app-installations.sh <app-id>`, fill
   `~/.leo-runners-rehearsal/github.env`, `run-controller.sh --jit`, `tunnel.sh`,
   paste the webhook URL into the App, open a PR with the sample workflow.
   Also resolves TODO 2.1a and 2.5c. Expect to fix bugs; this is the first real
   GitHub contact.
3. **Real-AWS layer** (needs a **non-company** AWS account from the owner): launch
   template, runner IAM role and instance profile, runner network, baked AMI
   behind the `aws-ephemeral-runner` contract (TODO 2.2c), then the disposable
   account smoke test (2.5d).
4. Then: org-level JIT endpoint so the App needs a narrower permission (2.1b),
   apply the unused GitHub timeout settings (2.1d), publish a controller image
   with a digest list (2.2d), GitHub App manifest (2.3), customer docs (TODO
   section 1), cut a new release tag after the live run, optional durable queue.

### Known limits (also in `TODO.md` and the docs)

- GitHub App auth has never talked to github.com; only a fake server.
- One App installation per controller; one controller replica (`desired_count`
  0 or 1) until multi-replica coordination is wired into the executable.
- The job queue is in memory; a crash loses queued events (GitHub redelivery and
  the reconciler cover part of this).
- `GITHUB_JIT_REQUEST_TIMEOUT`, `GITHUB_REGISTRATION_TIMEOUT` and
  `GITHUB_REGISTRATION_POLL_INTERVAL` are validated but not applied.
- Repo-level JIT needs Administration: write on the App.
- Behaviour on real EC2 and real DynamoDB is untested; floci is a rehearsal, not
  a performance or security test.
- An existing state table with TTL on `expires_at` must be switched to `ttl_at`
  (see `terraform/modules/controller-state/README.md`).

---

# Original phase log (historical)

## Original snapshot

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
- Phase 12 adds the Kubernetes manifest set in `deploy/kubernetes/` and its deployment guide.
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
- Phase 55 adds the reviewed activation-request contract, read-only preflight
  handoff wrapper, and `docs/production-activation.md`.
- Phase 56 reconciles stale TODO entries so remaining work reflects actual
  environment-gated validation and deployment tasks.
- Phase 57 adds controller lifecycle integration coverage for cancellation and
  expired-runner reaping paths.
- Phase 58 adds independent AWS provider validation, mocked regressions, and a
  guarded live-EC2 procedure.
- Phase 59 adds independent GCP provider validation, mocked regressions, and a
  guarded live-GCP procedure.
- Phase 60 adds approved fixed-commit workload intake, offline manifest
  derivation, and `docs/workload-intake.md`.
- Phase 61 adds bounded controlled workload execution, evidence validation, and
  `docs/controlled-workload-validation.md`.
- The latest implementation review remediation is present in the worktree;
  remaining review gaps are limited to larger architecture/environment work
  such as secure JIT secret transport, real provider integration, and sandbox
  implementation.

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
