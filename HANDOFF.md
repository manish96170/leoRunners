# Handoff

## Read this first (state at 2026-11-26)

LeoRunners is a generic, self-hosted, ephemeral GitHub Actions runner platform:
a Go controller, an AWS EC2 provider, Terraform modules, a runner bootstrap, an
AMI recipe and a local rehearsal harness. The repository is public and
personally owned. It is a product, not one customer's deployment: think of it as
an alternative to hosted runner services that anyone can deploy into their own
cloud account.

**Goal now:** make it customer-ready on AWS, proven with a real pull-request
workload, first in a throwaway repository, then (much later, only on the owner's
word) in a real front-end repository.

**Where we are:** the controller has run a real workload on runners it launched
itself against github.com, using a real GitHub App, with a local emulator
standing in for AWS. Four jobs passed. Nothing has run on real EC2 yet. The next
step is a real x64 EC2 run of the same throwaway repository, using two AMIs
(see "Images"). Use `git log` for the exact commit; the last tag
(`tf-aws-v0.1.0`) is old, so customer repositories pin a commit SHA.

### Ground rules (do not break these)

- **Nothing customer- or company-specific goes in this repository.** No names,
  domains, account IDs, App or installation IDs, subnets, labels, user home paths
  or customer defaults. Every environment value is a variable. Customer values
  live in the customer's own infrastructure repository, which consumes these
  modules from a pinned commit. Scan before every commit:
  `git diff | grep '^+' | grep -iE '<your marker list>'`.
- **Never edit another repository.** Findings for the infra side go to the owner
  as plain text (a prompt file kept outside this repo); fixes for generic code
  come here.
- **Git identity** in this repo is the personal noreply identity set in its local
  config. The machine's global git email is a company address and must never
  appear in this repo. Check `git config user.email` before committing.
- **Push, PR, merge** use only the personal GitHub login, per command, without
  changing global git or `gh` configuration (`GH_TOKEN` from
  `gh auth token --user <personal>` plus a one-shot credential-helper override
  for `git push`). Another login on the machine belongs to the company; use it
  only for the throwaway test repository, per command, and only where the owner
  said so.
- **Workflow:** branch, commit, push, PR, wait for the four CI checks (Unit
  tests, Race tests, Vet, Build and container checks), squash-merge with
  `--delete-branch`, sync `main`.
- **No real AWS without explicit instruction.** The machine has company AWS
  profiles; never use them. Local work runs against the floci emulator.
- **Secrets** (App private key, tokens, webhook secret) never go in the repo,
  logs, chat or tool output. The key goes to the emulator's secret store through
  `tools/local-rehearsal/store-app-key.sh`; the webhook secret lives in
  `~/.leo-runners-rehearsal/webhook-secret` and is read from there.
- **Do not change settings of the owner's GitHub App or organization on your own
  initiative.** When the owner approves a change, make exactly that change and
  say what it was. GitHub asks for "Confirm access" (re-authentication) on some
  settings pages; that step is the owner's.
- License: PolyForm Noncommercial 1.0.0.

### Who owns what

| LeoRunners (this repo, generic) | The customer's infrastructure repository |
|---|---|
| Controller code, Terraform modules, runner bootstrap, AMI recipe and extension hook, profile format, validation tools, docs, tests | Account, region, network, subnets, security groups, hostnames, certificates, runner labels, approvals, request IDs, cleanup deadlines, tool versions and the extension scripts that bake them, AMI IDs, retention |
| Defaults safe for any customer; every environment value is an input | Passes those inputs to pinned copies of the modules; applies only after a reviewed plan and an explicit approval |
| Never holds a secret | Secrets are placed by a human in the customer's own secret store |

A missing input, a rigid default, or anything the other side had to hard-code
is a generic defect: fix it here, test it on both AWS provider v5 and v6
(`terraform/validate-aws-modules.sh --provider-version <x.y.z>`), merge, and
hand over a new commit SHA.

### How the controller works (the parts that bit us)

1. GitHub sends `workflow_job` webhooks. The handler checks the signature,
   answers immediately (`202`, `200` for `ping`) and queues the event.
2. For a `queued` job the controller creates one runner: a one-time JIT
   configuration from GitHub, then an instance from the launch template with that
   configuration in its user data, then it waits for the runner to register.
3. A runner runs exactly one job and powers the instance off.
4. **Runners are interchangeable.** GitHub gives a queued job to any idle runner
   whose labels match, not to the runner created for it. So the runner created for
   job A may run job B. Consequences, all implemented: a job's end retires only
   the runner the webhook names (`runner_name`, which carries the job number it
   was created for); a job that ended without running retires none; the
   reconciler never terminates a runner because its job ended; leftovers go at
   their lifetime (`RUNNER_MAX_LIFETIME`).
5. The registration wait can take minutes and the reconciler runs throughout, so
   records are re-read after the wait (`markRunnerReady`, `settleAssigned`).
6. Two reapers: state-driven (records) and a provider-side backstop that lists
   instances by the `leo-runners:managed-by` tag and removes any 10 minutes past
   `leo-runners:expires-at`, with or without a record. The state table's TTL is
   `ttl_at` (seven days after `expires_at`), never the reaping deadline.
7. Registration scope: `repository` (default; needs repository Administration
   write; runners can join only the Default runner group, ID 1) or
   `organization` (needs the organization "Self-hosted runners" permission;
   an organization runner group applies). The organization path is tested only
   against a fake server.
8. Minted installation tokens are limited to the allowlisted repositories
   (`GITHUB_APP_TOKEN_REPOSITORIES`, derived from `GITHUB_ALLOWED_REPOSITORIES`
   unless an organization is allowed or owners differ), because an App's grant
   applies to every repository it is installed on.
9. The launch template owns the network interface (no public IP); the controller
   must not also be given a subnet or security groups. `max_runners` defaults to
   1: a workflow that queues several jobs at once needs a higher value or its
   extra jobs never get a runner.

### What exists and is verified

- Everything above, with unit tests (`go test -race ./...`, 32 packages), Terraform
  mocked-provider tests on AWS provider 5.100.0 and 6.73.0, and shell suites.
- `terraform/modules/aws-ecs-controller`, `aws-shared-gateway`,
  `aws-runner-layer` (launch template, runner role and instance profile, an
  ingress-free security group, a ready-made `runner` object), the
  `aws-ephemeral-runner` contract, `controller-state`, an example composition
  with a no-cycle test.
- `tools/launch-check/check.sh`: a read-only check of a launched instance (no
  public IP, IMDSv2, tags on instance and volumes, encryption, shutdown
  behaviour). Tested against a stub only.
- `examples/validation-workload`: a small front-end project and a PR-only,
  read-only, secret-free workflow that exercises what real repositories ask of a
  runner (exact Node, Yarn cache, lint, test shards, a hidden-file artifact
  round trip, legacy amd64-only Terraform with teardown, Cypress on Chrome,
  optional Docker). It passed fully on GitHub-hosted runners.
- `tools/local-rehearsal`: floci + the controller + a Cloudflare quick tunnel,
  `check-reaper.sh`, `app-installations.sh`, `app-deliveries.sh`.
- `ami/`: the base image recipe, an extension hook, tag support, and a derived
  front-end profile (see "Images").

### The first live runs (what they proved and found)

A throwaway repository in a test organization ran the sample workload on
runners launched by the rehearsal. Lint, both unit shards and the build with a
hidden-file artifact passed on controller-launched runners. The integration job
did not run (it needs real x64 hardware). Defects found and fixed, each with a
test that fails against the old code:

| Defect | PR | Symptom |
|---|---|---|
| Registration wait raced the reconciler (revision conflict) | #16 | healthy runners killed mid-job |
| Runners treated as bound to jobs | #20 | a finished job killed the runner running another job |
| Timeouts validated but never applied | #19 | slow start cut off at ten minutes |
| `ping` answered 400 | #15 | failed deliveries |
| `libatomic` missing from the base image | #17 | Node exit 127 |
| Repository-level runner given an organization group ID | #18 | 404 from GitHub |
| Token reached every repository the App is installed on | #22 | exposure of an unrelated repository |

**The lesson:** an emulator that starts instantly hides races and assumptions
that real timing exposes. Expect the first real EC2 run to find more; fix them
generically, add the failing test first.

### Images (AMIs)

- **Decision:** the recipe, extension hook and profile format are generic;
  builds, tool versions, URLs, checksums, AMI IDs and retention belong to the
  customer. Prefer a few profile images by kind of workload over one per
  repository.
- **Two images planned:** `base-linux-x64` (generic: git, jq, unzip, `libatomic`,
  curl, openssl; the runner is downloaded and checksum-verified at launch) and
  `frontend-browser-linux-x64` (base plus a pinned Chrome, the libraries Cypress
  needs, Docker, optionally a pinned Terraform), built with
  `ami/extensions/frontend-browser.sh`. After about three more front-end
  repositories pass on the front-end image, the base image can be retired unless
  something still needs it.
- A build uses a temporary builder instance that Packer removes; it still creates
  real AWS resources and needs the customer's approvals. `extra_tags` tags the
  builder, volume, AMI and snapshots for tag-based cleanup.
- Cost shape: an AMI is cheap (snapshot storage, cents to a few tens of cents a
  month); running instances are the cost, and a baked image shortens billed boot
  time. Confirm prices in the cloud provider's calculator.
- **Not built:** routing several labels to several images. One controller
  deployment has one launch template.

### Resuming the local live run (runbook, generic)

Needs: Colima/Docker with about 15 GB free in the VM (if low, `docker volume
prune -f` removes only unreferenced volumes; unrelated containers leak them),
`aws`, `terraform`, `go`, `cloudflared`, `python3`.

1. `tools/local-rehearsal/floci.sh up` and `setup.sh` (floci forgets everything
   when it restarts; the key must be stored again after a restart).
2. `store-app-key.sh <downloaded .pem>` (then delete the file) and
   `app-installations.sh <app-id>` to confirm the key and find the installation.
3. `~/.leo-runners-rehearsal/github.env`: App ID, installation ID, runner group
   ID (`1` for repository scope), scope, allowed repository, labels. None is a
   secret.
4. `tunnel.sh` prints the webhook URL (it changes at every restart), then
   `run-controller.sh --jit`. The owner pastes the URL into the App's webhook
   settings (SSL verification stays enabled; the secret comes from
   `~/.leo-runners-rehearsal/webhook-secret`).
5. Set the sample repository's `LEO_RUNS_ON` variable to the runner labels and
   push a commit to its pull request. Watch with `app-deliveries.sh`, the
   controller log and the run's job list. Four runners boot slowly under the
   emulator (many minutes); `GITHUB_REGISTRATION_TIMEOUT` is 30 minutes there.
6. Clean up: cancel the run, `stop-controller.sh`, `tunnel.sh stop`, delete
   leftover runner registrations, terminate instances, check no runner
   containers remain, ask the owner to disable the webhook and remove the repo
   variable.

The emulator runs arm64 containers; the sample's legacy Terraform job fails
there by design (no arm64 build), which is the point of the real x64 step.

### Next steps, in order

1. Done: the front-end profile work (extension, profile, derived-profile checks,
   bootstrap docker group) is merged; only its Chrome launch check is unproven
   (it needs a real builder). Prompt files for the infra session are kept outside
   this repository: regenerate them from the current `main`, do not trust copies
   left in a temp folder.
2. Customer side builds the base and front-end AMIs (after their own approvals),
   from a pinned commit of this repo, with `extra_tags` and a checksum-verified
   Chrome kept in their own storage. Prompt files for that live outside the repo.
3. Customer side applies the runner layer and controller service; real x64 run of
   the sample repository on the front-end image; `tools/launch-check` output kept
   as evidence; clean up by tag.
4. Fix what that run finds (generic, test first), publish a new commit SHA, then
   cut a release tag and a controller image with a recorded digest.
5. Only then: a controlled window on a real front-end repository, following the
   owner's rules (separate PR-only workflow, explicit label, hosted-runner
   fallback intact).
6. Backlog (see `TODO.md`): label-to-image routing, organization scope against
   github.com, releasing idle runners sooner, a durable webhook queue,
   multi-replica coordination, a GitHub App manifest, customer docs.

### Known limits

- Never run on real EC2 or real DynamoDB. Floci is a rehearsal, not a performance
  or security test (and boots four runners slowly).
- One controller replica; one App installation per controller; the webhook queue
  is in memory.
- Repository scope needs the broad Administration permission and the Default
  group; organization scope is untested against github.com.
- A runner created for a job cancelled while queued waits for another job or its
  lifetime.
- The integration job (legacy Terraform, Cypress, Docker, teardown) has not run
  on a LeoRunners runner.
- An existing state table with TTL on `expires_at` must be switched to `ttl_at`
  (`terraform/modules/controller-state/README.md`).

### Gotchas for whoever continues

- Google's repository keeps only the current Chrome, so a versioned URL stops
  working within weeks: pin Chrome by an RPM you keep in your own storage and
  verify by SHA-256.
- Node 25+ does not ship corepack; the sample installs Yarn with npm when absent.
- Biome 2.6 also lints Markdown; the sample excludes it.
- A mocked Terraform provider gives computed values only on `apply`, not `plan`.
- `sed -i` on macOS needs `-i ''`; a shell tool may reject dynamic quoting, so
  put logic in a script file; macOS Colima mounts only `$HOME`.
- Do not run two `terraform init` provider downloads at once on a nearly full
  disk.
- Chrome cannot start under CPU emulation (QEMU); test its launch only on real
  x86 hardware (`CHROME_LAUNCH_CHECK=false` skips it for emulated builds).
- The browser tool connection to the owner's Chrome can time out; ask the owner
  to re-allow it, and never type credentials or confirm access for them.

### How to run the checks

```sh
cd controller && go vet ./... && go test -race -count=1 ./...
for t in tools/local-rehearsal/test.sh tools/config-validation/test.sh deploy/kubernetes/test.sh \
         tools/smoke/test.sh tools/terraform-validation/test.sh bootstrap/runner/test.sh \
         tools/launch-check/test.sh examples/validation-workload/test.sh ami/test.sh \
         tools/ami-validation/test.sh; do bash $t; done
for v in 5.100.0 6.73.0; do terraform/validate-aws-modules.sh --provider-version $v; done
```

Run the two provider versions one after the other. For concurrency changes also
run `go test -race -count=60 ./internal/dispatch`.

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
