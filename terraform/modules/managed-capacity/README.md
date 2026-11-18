# Managed Capacity Module Contract

This directory documents the Terraform boundary for platform-owned ephemeral
runner capacity. It is a contract for a future deployable module, not an
authorization to create resources. The module must be applied only in an
operator-owned AWS account or GCP project and only after an approved plan.

## Inputs

The eventual module should accept explicit values for:

- cloud, account/project, region, and approved zones;
- runner image/profile and immutable template revision;
- private subnet, egress, firewall/security-group, and encrypted disk policy;
- controller and runner identities;
- capacity limits, allowed labels, tenant bindings, and expiry policy;
- ownership tag/label values and reaper discovery configuration.

Secrets and JIT configuration are not module inputs. They must be supplied at
runtime through the control-plane secret integration and one-time bootstrap
channel. Terraform state and plan output must contain no GitHub token, cloud
private key, or JIT value.

## Ownership And IAM

The operator owns the account/project and approves all module changes. The
controller identity is limited to the managed runner resources and required
read/terminate operations. The runner identity is separate and receives only
workload-approved permissions. Managed capacity must be tagged/labeled with
platform ownership, tenant, job, runner, creation, and expiry metadata.

The module must not create or alter customer accounts/projects, customer IAM,
customer networks, or cross-account trust. Any future integration with a
customer-owned resource must consume an already-created, reviewed reference.

## Lifecycle Contract

The provider launches one disposable runner per persisted provisioning attempt,
requires secure metadata/network settings, and supports bounded,
idempotent create, readiness, and termination operations. Reaping is scoped to
operator-owned resources and can recover from controller restart using durable
state plus ownership tags/labels.

Before implementation, validate the module in a disposable operator-owned
environment with `terraform fmt`, `terraform validate`, `terraform plan`, IAM
review, and a one-runner cleanup test. No `terraform apply` is implied by this
README.
