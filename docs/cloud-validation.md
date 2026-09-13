# Cloud Validation Runbook

This runbook is for one controlled, disposable validation job. It assumes the
operator has already reviewed [Phase 15](phase-15-plan.md), selected one cloud
provider, and obtained explicit approval for the named account/project and
resource prefix.

## Non-destructive defaults

Begin with offline tests, fake provider mode, Terraform `plan`, and read-only
identity queries. Do not run `apply`, send a real GitHub event, launch a VM, or
delete resources until the operator has reviewed the preceding output.

Use a dedicated account/project, region, workspace, test repository, and
short-lived credentials. Set capacity to one, a two-hour expiry, and a hard
budget alert. Prefer private subnets and deny inbound SSH. Attach no customer
secrets to the test job.

Use an explicit resource prefix such as `leo-validation-<ticket>-<date>`. Every
resource, tag, label, state record, and cleanup query must carry that prefix.
Never use a provider-wide delete command.

## 1. Preflight

From the repository root, run:

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
```

Run `./tools/docker-validation/validate.sh` when Docker is installed. Run
`terraform fmt -check -recursive` and `terraform validate` in each selected
Terraform module after provider initialization. A missing local tool is a
preflight failure for live validation, even when offline checks pass.

Record versions and exit codes. Store output in an owner-only evidence folder;
redact account details, tokens, paths containing secrets, and environment
dumps.

## 2. Identity and policy gate

Confirm identities with read-only commands:

```text
aws sts get-caller-identity --profile <validation-profile>
gcloud auth list
gcloud config get-value project
gh auth status
```

The controller role must be separate from the runner instance profile or GCP
service account. Review the generated policy rather than applying it directly.
Replace wildcard resource entries with the exact DynamoDB table/index, KMS,
Launch Template or instance-template, and runner-role resources. Review
`iam:PassRole`, secret-manager access, network mutations, and deletion rights.
Run IAM simulation and Access Analyzer where supported. GCP reviewers should
check service-account impersonation, project scope, and absence of broad owner
or editor grants.

Stop on any unexpected principal, wildcard resource, public ingress, broad role
passing, customer-secret access, or cross-account mutation.

## 3. Terraform plan gate

Use a fresh validation workspace and a locked dependency set:

```text
terraform init -lockfile=readonly
terraform fmt -check -recursive
terraform validate
terraform plan -out=validation.tfplan
terraform show -no-color validation.tfplan
```

Review before approval:

- exact account/project, region/zone, workspace, and resource prefix;
- one state table, encryption/PITR/TTL, deletion protection, and billing mode;
- one controller identity and one runner identity;
- exact image/template version and private network path;
- no broad ingress, unexpected public address, or unrelated destroy;
- bounded capacity, expiry, logs, and temporary resources only.

Keep the plan and state owner-only and outside source control. Do not paste
their contents into tickets or chat. Apply only the reviewed plan:

```text
terraform apply validation.tfplan
```

If the plan changes between review and apply, stop and create a new plan.

## 4. AWS one-job sequence

Before applying, verify the AWS region, Launch Template version, subnet,
security group, instance profile, AMI, and controller role. The security group
should allow no inbound SSH, and the instance should require IMDSv2.

After the apply gate and explicit approval:

1. Start the controller with the selected AWS configuration, one capacity
   slot, bounded operation timeouts, and the validation state backend.
2. Verify `/healthz` and that the controller identity is the reviewed role.
3. Deliver one signed `workflow_job` queued event for the approved test SHA.
4. Confirm the GitHub JIT request, exact labels/name/group, and one provider
   attempt. Keep the JIT response out of logs and state.
5. Query the single created instance by the validation prefix and confirm the
   expected Launch Template, tags, private network, IMDSv2, and running state.
6. Confirm exact runner ID/name and online status through GitHub.
7. Record workflow result, job conclusion, lease/fencing state, lifecycle
   events, provider request IDs, and phase timings.
8. Stop intake, terminate the instance, and run reconciliation twice to prove
   cleanup idempotency.

Use read-only queries for inspection, for example:

```text
aws ec2 describe-instances --filters Name=tag:ValidationPrefix,Values=<prefix>
aws ec2 describe-launch-template-versions --launch-template-id <id>
aws dynamodb describe-table --table-name <validation-table>
```

Do not copy user data, JIT values, credentials, or full tags containing
sensitive metadata into evidence.

## 5. GCP one-job sequence

Run only after the first provider has been cleaned up. Verify project, region/
zone, instance-template version, subnet, firewall rules, service account, and
private egress. The controller identity and VM service account must remain
separate.

1. Start the controller with the selected GCP configuration, one capacity
   slot, bounded timeouts, and the validation state backend.
2. Verify `/healthz` and the active project/service account.
3. Deliver one signed queued event for the approved repository and commit.
4. Confirm one JIT request, one provider attempt, required labels, and no JIT
   value in durable state or logs.
5. Query the VM by the validation label and confirm template version, zone,
   network, service account, operation completion, and running state.
6. Confirm exact GitHub runner registration and online status.
7. Record job result, lifecycle/lease events, request IDs, timings, and cleanup.
8. Stop intake, delete the VM, and run reconciliation twice.

Use read-only inspection commands such as:

```text
gcloud compute instances describe <name> --zone=<zone>
gcloud compute instance-templates describe <template>
gcloud projects get-iam-policy <project>
```

Do not run project-wide deletion or modify shared firewall, IAM, or network
resources during this validation.

## 6. GitHub and workload checks

The test workflow must be safe to run once, bounded by a timeout, and tied to a
known commit. Confirm the webhook signature, delivery ID, normalized owner/
repository/run/job identity, labels, runner group, and deduplication result.

Confirm the runner bootstrap receives the encoded JIT configuration through
the one-time delivery contract and removes it after registration. Search logs,
state records, telemetry, evidence, and backups for credential patterns before
sign-off. A match is a failed gate and requires containment and rotation.

Record workload status, exit code, duration, cache observations, and tool
versions. Label fake, estimated, and real evidence separately. A single job is
not a performance baseline and must not support comparative speed or cost
claims.

## 7. Cost, cleanup, and rollback safeguards

Before apply, set a budget alert and record expected maximum runtime. After the
job, inspect compute, disk, public IP, NAT/egress, logging, and state costs.
Terminate the runner on every path, including timeout and operator abort. Use a
bounded cleanup context and then verify provider state independently.

The cleanup order is:

1. stop webhook intake and drain the worker;
2. release or fence the lease;
3. terminate/delete the ephemeral runner;
4. rerun reconciliation and provider discovery;
5. remove only temporary validation resources;
6. verify zero unexpected resources and close the budget alert.

If cleanup fails, retain the controller long enough for bounded retry, alert the
operator, and use the exact resource IDs and prefix for manual remediation. Do
not broaden the deletion query.

Rollback means stopping the new controller, fencing its leases, restoring the
last validated image/configuration, and switching to the preserved file state
or validated DynamoDB snapshot. Do not destroy shared or production resources.

## 8. Evidence bundle and sign-off

Capture a redacted, owner-only bundle with:

- approval/ticket, timestamps, identities, provider, region/zone, and prefixes;
- commit/image/template/profile IDs and Terraform plan digest;
- static test and preflight results;
- IAM review/simulation/Analyzer results;
- webhook and JIT request outcome without secret payloads;
- runner ID/name, provider IDs, lifecycle events, lease state, and timings;
- workload result, cost estimate, actual billing observations, and evidence
  labels;
- cleanup queries, reconciliation reports, final resource inventory, and any
  rollback record.

Hash the bundle and define retention. Exclude Terraform state, plan contents
with secrets, cloud credentials, GitHub tokens, JIT configuration, user data,
full environment output, and customer data. Sign off only after the final
resource inventory is empty except for explicitly retained shared resources.
