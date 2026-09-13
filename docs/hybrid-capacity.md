# Hybrid Capacity

Hybrid mode uses the same control plane for customer-owned and platform-
managed pools. A tenant may register one or more customer pools and may be
allowed to use selected managed pools. The scheduler sees capacity metadata,
not separate product implementations.

## Request Flow

```text
job + tenant policy
        |
        v
filter tenant/security/workload eligibility
        |
        v
apply customer-first | managed-first | customer-only |
    managed-only | fallback
        |
        v
provision one runner, record pool ownership, reconcile and reap
```

The selected pool is persisted with the job and lease. A retry must reuse the
same ownership and tenant constraints unless a recorded fallback decision
explicitly permits a different pool class.

## Policy Rules

- `customer-first` prefers eligible customer pools and permits managed
  capacity only when the customer attempt is unavailable or transiently
  exhausted.
- `managed-first` reverses that order.
- `customer-only` and `managed-only` fail closed when their class cannot serve
  the job.
- `fallback` requires an explicit primary and secondary ownership class,
  workload allowlist, and transition telemetry.

No policy may cross a tenant boundary. A provider outage, missing permission,
or security-policy rejection is not silently converted into fallback. The
decision record must include the rejected reason and selected ownership.

## Tenant Isolation

Pool registration is scoped to a tenant and includes repository/workflow
allowlists, runner labels, OS/architecture, region, security profile, and
maximum concurrency. These constraints are applied before provider calls.
Provider resources receive stable tenant, repository, workflow, job, runner,
creation, and expiry ownership tags/labels, subject to provider limits.

Managed and customer pools use separate credential references and separate
reaper scopes. Telemetry is tenant-scoped and must not expose JIT material,
provider credentials, or another tenant's repository/workflow data. A
customer-owned runner identity must not be reused by managed capacity, and a
managed controller credential must not be installed in a customer account.

## Explicit Cross-Account Boundary

Hybrid onboarding is a declarative contract plus a customer-approved
installation step. The customer creates and reviews its account/project IAM,
network, image/template, and trust configuration. The platform may validate
the supplied references and use an already-authorized integration, but it
must not automatically create, alter, or delete resources in the customer
account/project. Removal likewise requires an explicit customer action.
