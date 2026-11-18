# Customer Capacity Module Contract

This directory documents the Terraform boundary for customer-owned ephemeral
runner capacity. The customer applies and owns this module in its own account
or project. It is documentation scaffolding only; it does not grant the
platform authority to make infrastructure changes.

## Inputs And Customer Responsibilities

The eventual module should accept explicit values for:

- customer account/project, region, approved zones, and provider;
- customer-owned image/profile and immutable template revision;
- subnet, controlled egress, firewall/security-group, and encrypted storage;
- customer controller integration identity and least-privilege runner
  identity;
- tenant/repository/workflow allowlists, labels, quotas, and expiry policy;
- ownership tags/labels and reaper discovery scope.

The customer reviews the plan, creates any IAM or trust relationship, grants
only the documented permissions, and controls network egress. GitHub App
authorization remains explicit. JIT configuration and cloud private keys must
never be stored in Terraform variables, state, images, labels, logs, or
startup metadata.

## Control-Plane Boundary

Customer capacity uses the same GitHub-facing control plane and provider
contract as managed capacity. The platform may use an already-approved,
scoped integration to create, inspect, and terminate only the ephemeral
runner resources described by the customer configuration. It must not create,
modify, or delete customer IAM, networks, projects/accounts, images, quotas,
or unrelated resources.

The customer can select `customer-only` to guarantee that jobs never use
managed capacity. In hybrid mode, managed fallback requires an explicit
tenant/workload policy and must be recorded as a policy transition. A
customer-owned pool cannot be selected for another tenant.

## Security And Lifecycle Contract

Use separate controller and runner identities. The controller identity must
be constrained by region, template, ownership tags/labels, and resource
actions. The runner identity must not administer provider resources or other
tenants. Prefer private networking, no inbound SSH, IMDSv2 where applicable,
and controlled outbound HTTPS.

The provider must support exactly one runner per provisioning attempt,
bounded/idempotent operations, durable ownership metadata, and restart-safe
reaping. The customer approves the module and performs deployment in a
disposable environment before production use. Cross-account setup and removal
are explicit customer actions; there is no automatic cross-account change.
