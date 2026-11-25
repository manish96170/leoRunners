# Runner validation guide

## Purpose

This is a reusable, guide-only record for making LeoRunners ready for a
controlled customer validation. It does not authorize a deployment, create a
cloud resource, configure a GitHub App, or change a customer workflow.

The first live validation must prove that one disposable runner can accept one
pull-request job, run it, and clean itself up. It is an experiment, not a
production rollout.

## Repository boundaries

- This public repository contains generic platform code, modules, examples,
  tests, and documentation only.
- Customer names, domains, account identifiers, repository identifiers,
  network identifiers, labels, credentials, and defaults do not belong here.
- Customer-specific composition, infrastructure values, and workflow changes
  belong in a separate private infrastructure repository.
- Never store a GitHub App key, access token, webhook secret, or cloud
  credential in this repository, logs, evidence, or chat.

## Safety baseline

- Use the repository-local personal Git identity. Verify it before committing.
- Use only the intended personal GitHub account for each push, pull-request,
  and merge command. Do not alter global authentication configuration.
- Do not use a real cloud account unless its owner gives explicit approval for
  that exact validation.
- Until then, run the local rehearsal only. It uses floci, not an AWS account.
- A real validation uses a disposable, non-production cloud account and a
  dedicated test repository or tightly scoped pull request.
- Keep the incumbent runner path unchanged until the replacement passes every
  required gate and its rollback is exercised.

## Current verified baseline

The following has been verified locally or with mocked services:

- GitHub App authentication contracts, short-lived installation-token flow,
  webhook signature checking, and JIT configuration client behavior for both
  repository-level and organization-level registration.
- Asynchronous webhook acknowledgement, idempotent event handling, lifecycle
  state transitions, cancellation, reconciliation, and reaping.
- AWS provider contracts for a launch template and disposable instances,
  including launch-time ownership tags, IMDSv2, readiness polling, and
  idempotent termination.
- Terraform modules for controller state, controller runtime, shared ingress,
  IAM scaffolding, and the ephemeral-runner contract.
- Runner bootstrap with a pinned and checksum-verified runner download.
- Local rehearsal of invalid signatures, queue acknowledgement, duplicate
  delivery, restart safety, completion cleanup, shutdown draining, scoped
  webhook routing, and reaper cases.

These checks do **not** prove real EC2, DynamoDB, network, cache, browser,
Docker, or full workload behavior. A local rehearsal against github.com has
since passed lint, test shards and a hidden-file artifact build on
controller-launched runners (see `HANDOFF.md`).

## Local verification commands

Run from the repository root. These commands are local-only unless a command
is explicitly configured otherwise.

```sh
cd controller && go vet ./... && go test -race -count=1 ./...

tools/local-rehearsal/test.sh

for t in bootstrap/runner/test.sh tools/config-validation/test.sh \
  deploy/kubernetes/test.sh tools/smoke/test.sh \
  tools/terraform-validation/test.sh; do
  bash "$t"
done

terraform/validate-aws-modules.sh --provider-version 5.100.0
terraform/validate-aws-modules.sh --provider-version 6.73.0

tools/local-rehearsal/floci.sh up
tools/local-rehearsal/setup.sh
tools/local-rehearsal/check-reaper.sh
```

For changes to concurrent webhook dispatch, also run:

```sh
cd controller
go test -race -count=60 ./internal/dispatch
```

## Local rehearsal facts

- The emulator endpoint is local and uses a synthetic account identifier.
- Its state, logs, and compiled binary are stored outside the repository in a
  local rehearsal directory.
- Emulator instances are containers. Their user-data starts after a short
  delay, so local timing is not an EC2 performance measurement.
- On Apple Silicon, use the documented arm64 emulator image. The local
  bootstrap handles the dependency-installation differences documented in the
  rehearsal harness.
- `down.sh` removes the rehearsal environment and its migration-test resources.
  Confirm its scope before using it.

## Ordered path to a first live job

### 1. Incorporate infrastructure feedback

Review the separate infrastructure-session findings for module gaps, missing
inputs, invalid assumptions, or unexpected replacements. Make only generic
fixes here. Run the local verification suite, open a pull request, and merge
only after review.

### 2. Perform a real GitHub pull-request rehearsal

The environment owner creates a GitHub App in a browser and installs it only
on the chosen test repository. Prefer `GITHUB_RUNNER_SCOPE=organization`: the
App then needs only Actions read (repository) and Self-hosted runners read and
write (organization), and the organization runner group adds its own
repository-access limit. The default repository scope needs the broad
repository Administration write permission instead; see
`docs/github-runtime-contract.md`. Keep the private key in the designated
local secret store, never in a file committed to the repository.

Run the controller in JIT mode through the documented local rehearsal tunnel.
Use a sample pull-request workflow that has no deployment permissions. Record
only redacted evidence of:

1. signed webhook accepted;
2. JIT runner created with the exact approved label;
3. checkout and workflow commands completed;
4. job completion observed;
5. runner deregistration and instance termination completed; and
6. no residual registration or owned emulator resource remains.

Expect integration defects in this first contact with github.com. Fix them in
generic code with regression tests before attempting a real-cloud validation.

Also confirm in this step, with a test organization, whether the plan allows
the runner group you intend to use, and that a runner from the group is refused
for a repository the group does not include.

### 2b. Prove a representative workload in a sample repository

Before any customer workflow is routed to LeoRunners, reproduce its
requirements in a small sample repository that you own (start from
`examples/validation-workload`), using public stand-ins
for the real tools: the same runtime and package-manager versions, the same
infrastructure-tool version with a local backend, a headless browser test,
Docker if needed, cache and artifact upload and download (including hidden
files), and a temporary integration resource that the workflow creates and
destroys. Run it on a real runner from step 3. A failure here costs minutes in
a repository nobody else uses; the same failure in a customer repository costs
a review window.

### 3. Add the real-cloud runner layer

Use a cloud account explicitly supplied for this purpose and separate from any
production or shared environment. The private infrastructure composition must
provide, at minimum:

- a published, immutable runner image;
- runner launch template, instance role, profile, and private network path;
- independent controller and runner permissions;
- bounded ingress and egress rules;
- encrypted state and secret handling;
- an immutable profile version and explicit runner label allowlist;
- a capacity limit of one runner for the first validation;
- complete ownership tags created atomically; and
- a request-specific cleanup deadline and rollback procedure.

Before applying, review the infrastructure plan for dedicated resources only.
After applying, run a single disposable smoke test, check the launched
instance with `tools/launch-check/check.sh`, and prove scoped cleanup by exact
ownership-tag intersection, never by name or network location.

### 4. Complete follow-up work and release

After one successful live job and cleanup proof:

1. publish a controller image with a recorded digest list, built from the same
   commit as the Terraform modules that deploy it;
2. add the App manifest and complete operator documentation;
3. capture runner image startup, workload, cache, artifact, network, and cost
   measurements; and
4. cut a new release tag only after the live evidence is reviewed.

## Target-workload admission checklist

Before routing any customer workflow to LeoRunners, create an external,
versioned profile that proves the exact workload requirements. Validate all of
the following under a controlled pull request:

- CPU architecture, operating system, disk, and memory;
- required runtime and package-manager versions;
- required infrastructure-tool versions;
- Docker capability, if the workload requires it;
- browser binary and headless test support, if required;
- GitHub checkout, cache, artifact upload/download, including hidden files;
- temporary integration apply, health check, test, and teardown where needed;
- one-runner JIT registration, completion, deregistration, and termination;
- zero leaked runner registrations, instances, volumes, network attachments,
  or capacity after cleanup; and
- an unchanged, tested incumbent-runner fallback.

Do not change a normal workflow's runner selector globally for this experiment.
Prefer a dedicated pull-request-only UAT job and use a newly approved explicit
label rather than a guessed or broad label.

## Current known limits

- GitHub App authentication has run against github.com only from the local
  rehearsal, in repository scope. Organization scope and real EC2 have not.
- The controller currently serves one App installation and one replica.
- The webhook queue is in memory, so a crash can lose queued events.
- A runner created for a job cancelled while queued waits for another job or
  until its lifetime (`RUNNER_MAX_LIFETIME`) ends.
- Organization-level JIT is implemented and tested against a fake GitHub only.
  Repository-level JIT needs broader GitHub App permissions and remains the
  default for compatibility.
- Real EC2, DynamoDB, runner-image, and real-workload behavior have not been
  proven. The local emulator is a functional rehearsal, not a security or
  performance result.

## Evidence and release rule

The first live validation is complete only when the owner has reviewed
redacted evidence for the full lifecycle and scoped cleanup. A successful
workflow alone is insufficient. Keep the project pre-release and do not tag a
new release until that evidence is available and follow-up defects are either
fixed or explicitly accepted with documented limits.

## Related documents

- `tools/local-rehearsal/README.md`: the full local procedure, including the
  GitHub App settings.
- `docs/github-runtime-contract.md`: controller GitHub settings, scopes, and
  authentication modes.
- `docs/aws-modules.md` and `docs/aws-iam-policy-workflow.md`: module inputs,
  outputs, and the reviewed-policy workflow.
- `docs/aws-provider.md`: instance tags, the reaper, and its backstop.
- `docs/runner-images.md`: runner image profiles and rollback.
