# Phase 15: Controlled Cloud Validation

## Purpose

Phase 15 validates one complete ephemeral-runner job in a disposable,
non-production environment. The goal is to prove the controller, shared state,
provider, GitHub JIT registration, workload execution, telemetry, and cleanup
contracts with real services while keeping spend, permissions, and blast radius
bounded.

This phase is a validation exercise, not a production rollout. Every default
must be read-only, dry-run, fake-mode, or otherwise non-destructive. A real
cloud mutation requires an explicit operator approval after the preceding gate
has passed.

## Scope and exit criteria

The minimum successful run is one GitHub `workflow_job` from an approved test
repository, one ephemeral runner in one selected provider and region, one
successful or intentionally failed job, durable lifecycle evidence, and
verified termination. The same procedure may then be repeated for the other
provider, but AWS, GCP, and GitHub are not enabled simultaneously by default.

Phase 15 exits only when:

- local preflight and all static validation gates pass;
- Terraform plan has been reviewed and contains only intended resources;
- apply is approved in a disposable account/project and succeeds;
- the runner receives a short-lived JIT configuration without persisting it;
- exactly one runner is assigned to the test job;
- the job outcome, runner state, lease, and cleanup events are captured;
- the runner and temporary cloud resources are confirmed deleted;
- measured cost and residual resources are reconciled;
- evidence is stored without credentials, tokens, JIT configuration, or secrets.

## Gates

### Gate 0: Freeze scope and identifiers

Record the validation owner, approval ticket, cloud account/project, region,
provider, GitHub organization/repository, test commit SHA, workflow/job name,
Terraform workspace, image ID, Launch Template or instance-template version,
state table name, and an expiry timestamp. Use a unique validation prefix on
every resource. Set a short maximum lifetime, such as two hours, and a hard
spend limit before provisioning.

Do not use a production repository, production credentials, a personal access
token, or a shared long-lived runner registration token. The test repository
must contain a harmless workflow and no sensitive fixtures.

### Gate 1: Local preflight

Run the repository's offline checks before contacting a cloud service:

```text
go test ./...
go test -race ./...
go vet ./...
go build ./...
./bootstrap/runner/test.sh
./tools/smoke/test.sh
./workloads/validate.sh
./ami/test.sh
./deploy/kubernetes/validate.sh
./tools/docker-validation/validate.sh
```

The Docker validator may report an environment-dependent skip when Docker is
not installed. Treat missing Docker, `kubectl`, Packer, Terraform, or the AWS/
GCP CLI as a failed live-validation preflight, not as proof of readiness.

Confirm the selected AMI or image profile, bootstrap version, controller image,
Go module checksums, and workload manifest are the reviewed artifacts. Capture
tool versions and validation output, but redact environment variables and
credential-bearing command output.

### Gate 2: Identity and IAM review

Verify the active identity before any plan or apply:

```text
aws sts get-caller-identity --profile <validation-profile>
gcloud auth list
gcloud config get-value project
gh auth status
```

Use separate controller and runner identities. The controller may use only the
reviewed table/index, required KMS data-key operations, provider launch/status/
terminate operations, and the exact runner instance role. The runner role has
no controller, state-table, IAM-administration, or customer-secret access.

Run policy generation in review mode, replace wildcard resources with real
ARNs, and inspect `iam:PassRole`, launch-template/instance-template access,
state-table/index access, KMS conditions, and observability destinations.
Run IAM simulation and IAM Access Analyzer where available. Stop if a policy
permits cross-account mutation, unrestricted role passing, broad resource
wildcards, secret reads, or deletion outside the validation prefix.

### Gate 3: Terraform plan

Use a fresh backend/workspace dedicated to validation. Initialize with locked,
reviewed provider versions and validate every module:

```text
terraform fmt -check -recursive
terraform init -lockfile=readonly
terraform validate
terraform plan -out=validation.tfplan
terraform show -no-color validation.tfplan
```

Review the plan for exact resource names, region/project, image/template,
subnet/network, security rules, encryption, deletion protection, TTL, billing
mode, IAM attachments, and outputs. A plan containing replacement of shared or
production resources, broad network ingress, public IPs without approval,
unbounded capacity, or unrelated destroys fails the gate.

Keep the plan file in an owner-only temporary location. Do not commit it or
print its contents into logs that may contain sensitive values. Plan output is
evidence of intent only; it is not approval to apply.

### Gate 4: Controlled apply

After written approval, apply exactly the reviewed plan:

```text
terraform apply validation.tfplan
```

Apply only the disposable state table, least-privilege IAM scaffolding, one
provider template/profile, and explicitly required networking. Do not apply
both cloud providers in the same run unless the test case requires it.

Immediately record resource IDs, tags/labels, creation time, and expiry time.
Install a cleanup watchdog before starting the GitHub workflow. If apply
differs from the reviewed plan, stop and destroy only the approved validation
resources after collecting failure evidence.

### Gate 5: One-job lifecycle validation

Run the following sequence for one provider at a time:

1. Start the controller in fake mode or read-only mode for wiring checks.
2. Verify `/healthz`, configuration validation, state backend, and provider
   identity.
3. Enable the selected real provider with a bounded context, capacity of one,
   and the validation prefix.
4. Send one signed `workflow_job` queued event from the approved GitHub test
   repository.
5. Confirm delivery idempotency and that one job produces at most one provider
   attempt and one runner.
6. Confirm the JIT request contains the required runner name, runner group,
   and labels, and that the encoded JIT value appears only in the bootstrap
   delivery path.
7. Confirm the instance/VM uses the reviewed image, private networking where
   required, IMDSv2 or equivalent metadata protection, and the expected
   tags/labels.
8. Confirm registration by exact runner ID/name and online status.
9. Let the test workflow finish, or cancel it intentionally as a separate
   case, and verify the corresponding success, failure, or cancellation state.
10. Confirm lease renewal, assignment metadata, lifecycle events, timings, and
    provider request IDs are present without secret values.
11. Confirm termination is requested once, completes, and remains idempotent
    when reconciliation runs again.

Repeat the same evidence set for GCP only after the AWS or first-provider run
has been fully cleaned up. For GCP, verify project/zone, instance-template
version, service account separation, network egress, labels, and operation
completion. For AWS, verify account/region, Launch Template version, instance
profile separation, IMDSv2, tags, and EC2 state transitions.

### Gate 6: Cleanup and reconciliation

Cleanup starts on success, failure, timeout, cancellation, operator abort, or
watchdog expiry. Stop webhook intake, drain the worker, release the fencing
lease, terminate the runner, and delete only resources bearing the unique
validation prefix. Run the reaper and provider discovery checks after the
controller has restarted.

Verify no runner, VM, volume, public IP, firewall rule, temporary IAM binding,
state item, lease, or test registration remains unexpectedly. Treat DynamoDB
TTL as asynchronous storage cleanup; immediate reconciliation and reaping must
enforce runner expiry.

### Gate 7: Evidence and sign-off

Produce a redacted evidence bundle containing:

- commit SHA, image/template/profile identity, tool versions, and timestamps;
- approved Terraform plan digest and resource inventory;
- IAM review, simulation, and Access Analyzer results;
- webhook status/result and delivery-id outcome;
- normalized job identity and lifecycle event summary;
- provider request IDs, runner ID/name, readiness and termination timings;
- workload result and benchmark/cost record with evidence labels;
- reconciliation report, cleanup inventory, and final cloud queries;
- failures, retries, operator approvals, and rollback actions.

Hash the bundle and record its retention period. Never include GitHub tokens,
encoded JIT configuration, cloud credentials, Terraform state, secret values,
unredacted user data, or complete environment dumps.

## Rollback

For a failed plan, do not apply. For a failed apply, stop intake and destroy
only the explicitly approved disposable resources. For a failed runner
workflow, terminate the runner and preserve redacted lifecycle evidence. For a
controller regression, stop the new controller, release or fence its leases,
restore the last validated image/configuration, and use the preserved file or
DynamoDB snapshot according to the Phase 13 rollback procedure.

Rollback must be operator-confirmed and bounded by the validation prefix. Never
delete a shared state table, production resource, customer-owned resource, or
unrelated network object as part of this phase.

## Explicit non-goals

- No production apply or customer workload execution.
- No automatic cross-account role, project, network, or IAM changes.
- No unrestricted wildcard IAM policies accepted as final configuration.
- No public ingress, SSH access, or long-lived runner credentials by default.
- No autoscaling, parallel jobs, warm pools, spot capacity, or multi-region
  failover claims from a one-job test.
- No deletion based only on an unreviewed name, timestamp, or provider-wide
  scan.
- No claim of performance, savings, or reliability without captured real
  evidence and comparable baseline data.
