# GCP Provider

Phase 8 defines the GCP infrastructure contract for one ephemeral Compute
Engine VM per runner lease. This is scaffolding and operational guidance only;
it does not provision a project, network, service account, instance template,
or VM by default.

## Provider Boundary

The cloud-neutral controller supplies a runner specification, an ownership
scope, a provisioning-attempt key, and an expiry time. The GCP adapter is
responsible for translating that request into one Compute Engine instance,
observing its state, and deleting it. Scheduling, GitHub workflow semantics,
leases, persistence, and retry policy remain outside the provider.

The intended sequence is:

1. Select an eligible GCP capacity pool and a zone.
2. Create exactly one VM from the configured instance template, applying the
   per-runner labels and a deterministic name/idempotency key.
3. Wait for the VM and guest bootstrap to become ready within a context
   deadline.
4. Observe GitHub runner registration through the controller.
5. Delete the VM on completion, cancellation, expiry, failure, or reaping.

An instance template should contain the stable machine definition: machine
type, boot image or image family, boot disk, network interface, service
account, metadata/startup-script contract, and shielded-VM settings. A
template can be global or regional. Use a regional template when locality or
data-residency boundaries matter; otherwise a global template can be reused
across eligible zones. The VM creation request must still name the target
zone. See the [Compute Engine instance template
documentation](https://cloud.google.com/compute/docs/instance-templates/create-instance-templates).

## Zones And Capacity

Zones are capacity-pool data, not scheduler branches. Record the project,
region, zone, machine type, image/template revision, labels, security profile,
and availability in the pool configuration. Prefer an explicit ordered list
of approved zones so a capacity error can be retried in another zone without
changing the GitHub-facing job contract.

Do not use a managed instance group, autoscaling, a warm pool, or persistent
runner capacity for this phase. A lease reserves one VM and the provider must
not silently fan out to multiple instances. A future multi-zone policy must
preserve that one-lease/one-runner invariant and make any retry a new recorded
attempt with a stable idempotency key.

## Service Accounts And IAM

Separate the controller identity from the VM identity:

- The **controller service account** may create, describe, and delete only the
  Compute Engine resources used by this provider. Grant it the smallest
  project or folder role possible, and use IAM Conditions or a dedicated
  project when resource-level restrictions are required. It must not be the
  VM's service account.
- The **runner service account** is attached to the instance template and has
  only workload-approved APIs. It must not have permission to create or
  delete Compute Engine instances, impersonate the controller, administer IAM,
  or read unrelated secrets.
- Prefer service-account impersonation or Workload Identity Federation for
  operators and CI. Do not put a service-account key in the AMI, template,
  Terraform variables, startup script, or repository.

Local Go development uses [Application Default
Credentials](https://cloud.google.com/docs/authentication/application-default-credentials).
ADC checks `GOOGLE_APPLICATION_CREDENTIALS`, then the local file created by
`gcloud auth application-default login`, then the attached service account
when running on Google Cloud. Choose the credential source deliberately and
verify the active principal before any live test. Production should use the
attached controller identity or federation rather than a checked-in JSON key.

## Network And Egress

Use a dedicated subnet and a firewall policy with no inbound SSH or open
management ports. The VM needs outbound HTTPS to GitHub and any approved
package, artifact, container, and cache endpoints required by the workload.

The default profile should have no external IP. Provide egress through Cloud
NAT or an equivalent controlled path, and enable Private Google Access when
the VM needs Google APIs without a public address. Restrict egress with the
organization's approved firewall, proxy, DNS, and logging controls. If a
public IP is temporarily required for an isolated test, make that an explicit
profile choice with a short expiry and a documented rollback.

The startup contract carries the short-lived GitHub JIT configuration only at
boot. It must not be placed in instance-template labels, ordinary persistent
metadata, disk images, logs, or Terraform state. Use a one-time protected
file/descriptor or an equivalent ephemeral channel, remove it after the
runner starts, and ensure process arguments and diagnostics do not expose it.

## Labels And Ownership

Apply stable, queryable labels to every VM at creation. The exact label values
must be normalized to Compute Engine label rules and must not contain secrets.
The recommended ownership set is:

```text
platform=ci-runner
managed-by=leo-runners
provider=gcp
repository=<normalized-repository>
workflow-run=<run-id>
job=<job-id>
runner=<runner-id>
attempt=<provider-attempt-key>
created-at=<unix-seconds>
expires-at=<unix-seconds>
```

Keep the durable job, runner, lease, and attempt records as the source of
truth. Labels are the recovery index used by the reaper after a controller
restart or partial failure. Do not copy arbitrary `RunnerSpec.Metadata` into
labels; only an allowlisted, length-checked set may cross this boundary.

## Deletion And Reaper

Deletion is the normal termination operation, not merely a stop. The provider
must make it idempotent: a successful delete, an already-missing VM, and a
known terminal state should converge to the same durable result. Use a bounded
context for each API call and record `cleanup-pending` when deletion fails.

The reaper should:

- scan the configured project and approved zones for `platform=ci-runner` and
  `managed-by=leo-runners`;
- delete VMs whose `expires-at` label is in the past;
- delete orphan VMs whose attempt or lease is absent from durable state;
- reconcile VMs that are already deleting or missing;
- retry transient Google API failures with a deadline and bounded backoff;
- record every cleanup attempt without treating a duplicate event as a new
  runner.

A failed create response must not be assumed to mean that no VM exists. The
adapter should use its deterministic name/attempt identity and labels to
discover and reconcile an ambiguous result before retrying. Never launch a
second VM solely because a create response was lost.

## Readiness And Bootstrap

`RUNNING` is not sufficient proof that a runner is usable. Poll the Compute
Engine instance and guest-health signals until the VM is running, the startup
contract has completed, and the exact GitHub runner ID/name is online. Bound
each wait by the lifecycle context. Treat failed, terminated, missing, or
timed-out VMs as provider errors and invoke cleanup.

The instance template should use a versioned, immutable base image where
possible. Templates may carry a small startup script, but the image and
startup path must contain no GitHub PAT, JIT token, cloud key, or customer
secret. Compute Engine startup scripts require the guest environment; custom
images must include and validate that guest agent. See the [startup script
documentation](https://cloud.google.com/compute/docs/instances/startup-scripts).

## Explicitly No Deployment By Default

This repository currently supplies documentation and example Terraform
variables only. It does not include a live GCP provider implementation or a
complete Terraform module in this phase. Do not run `terraform apply`, create
service-account keys, or launch a VM as part of reading or testing these
files. A future implementation must add plan-only validation, a fake provider
test suite, IAM review, cost limits, and an explicitly approved disposable
project before any real deployment.

References: [create a VM from an instance
template](https://cloud.google.com/compute/docs/instances/create-vm-from-instance-template),
[delete a Compute Engine
instance](https://cloud.google.com/compute/docs/instances/deleting-instance),
and [Application Default
Credentials](https://cloud.google.com/docs/authentication/application-default-credentials).
