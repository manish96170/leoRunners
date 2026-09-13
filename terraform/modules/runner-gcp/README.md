# Runner GCP Module

This directory is the Phase 8 contract for a future Terraform module that
will support one ephemeral Google Compute Engine VM per CI runner lease. It is
documentation scaffolding only. There is no deployable module here yet, and
`terraform apply` is not part of validation for this phase.

## Intended Inputs

The eventual module should accept:

- project, region, and an explicit approved zone list;
- a global or regional Compute Engine instance template reference;
- machine and image/template revision policy;
- a dedicated subnet and firewall/network tags;
- controller and runner service-account identities;
- startup/bootstrap reference without embedded secrets;
- allowlisted ownership labels and expiry policy;
- boot-disk encryption and deletion settings;
- optional Cloud NAT/private-access assumptions;
- lifecycle limits and reaper discovery settings.

The instance template should own stable VM properties such as machine type,
boot image, disk, network interface, service account, startup metadata, and
Shielded VM configuration. The controller should provide only per-runner
identity and ownership values at instance creation. Do not pass arbitrary
metadata into labels or startup metadata.

## Security Contract

The controller service account and the runner service account are separate.
The controller may manage only the intended Compute Engine resources and must
not be used as the VM identity. The runner identity receives only workload-
approved permissions and cannot administer Compute Engine, IAM, or other
runner identities. Prefer workload federation or service-account
impersonation; never add a service-account private key to this module,
variables files, an image, or startup data.

The default network profile has no external IP, no inbound SSH, and outbound
HTTPS through controlled egress such as Cloud NAT. Private Google Access is
enabled when Google APIs are needed from a private subnet. GitHub JIT
configuration is short-lived secret material and must travel through a
one-time protected bootstrap channel. It must not appear in labels, ordinary
instance metadata, Terraform state, logs, or plan output.

## Lifecycle Contract

The future provider must create exactly one VM for each persisted provisioning
attempt, wait for guest readiness and GitHub registration, and delete the VM
on completion, cancellation, expiry, failed provisioning, or failed
registration. Create and delete operations must be context-bounded and
idempotent. A lost create response must be reconciled by deterministic
identity and ownership labels before a retry can be attempted.

The reaper must scan approved projects/zones for the ownership labels, delete
expired or orphaned VMs, and record cleanup-pending state when Google APIs
remain unavailable. The VM's `expires-at` label is a recovery hint; durable
leases and lifecycle state remain authoritative. Stopped or already-missing
instances must not be mistaken for healthy capacity.

## Example Workflow

1. Copy the example values from `variables.example.tfvars` into a local,
   ignored file and replace every placeholder.
2. Review IAM, subnet, firewall, NAT, template revision, zone, and cost
   limits with the infrastructure owner.
3. Run `terraform fmt` and `terraform validate` only after a complete module
   exists.
4. Run `terraform plan` in a disposable project and inspect the exact VM,
   service-account, network, and label changes.
5. Obtain explicit approval before any real apply or VM launch.

This scaffolding intentionally stops before step 3. It creates no resources
and makes no Google Cloud API calls.

References: [Compute Engine instance
templates](https://cloud.google.com/compute/docs/instance-templates/create-instance-templates),
[create a VM from a template](https://cloud.google.com/compute/docs/instances/create-vm-from-instance-template),
and [Application Default
Credentials](https://cloud.google.com/docs/authentication/application-default-credentials).
