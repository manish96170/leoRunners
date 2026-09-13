# Phase 9 Plan: Managed And Hybrid Capacity

## Goal

Phase 9 defines one deployment model for customer-owned capacity, platform-
managed capacity, and a controlled combination of both. The GitHub webhook,
scheduler, runner lifecycle, provider contract, persistence, reconciliation,
and reaper remain common control-plane behavior. Capacity ownership is data on
a pool registration, not a separate scheduler implementation.

## Scope

- Represent capacity pools with an explicit tenant, owner, provider, region,
  security profile, labels, availability, and billing boundary.
- Resolve `customer-first`, `managed-first`, `customer-only`,
  `managed-only`, and `fallback` policies deterministically.
- Preserve tenant and repository isolation across scheduling, provisioning,
  registration, telemetry, and cleanup.
- Document separate credentials and IAM boundaries for the control plane,
  customer provider, managed provider, and runner workload.
- Provide Terraform contracts for managed capacity and customer capacity.

This phase does not add a second control plane, automatic cross-account
bootstrap, capacity autoscaling, warm pools, or cost optimization.

## Capacity And Ownership Model

Every eligible pool has an immutable ownership class:

| Ownership | Infrastructure owner | Provider credentials | Billing owner |
| --- | --- | --- | --- |
| `customer` | Customer | Customer-supplied integration | Customer |
| `managed` | Platform operator | Platform-managed integration | Platform |

The scheduler receives a normalized job and a policy. It filters pools by
tenant, repository/workflow authorization, labels, OS/architecture,
security profile, region, and current availability before applying the policy
ordering. A pool cannot change ownership while it contains active leases.

## Policy Contract

| Policy | Eligible order | Failure behavior |
| --- | --- | --- |
| `customer-first` | Customer pools, then managed pools | Use managed capacity only when an eligible customer attempt cannot be started |
| `managed-first` | Managed pools, then customer pools | Use customer capacity only when an eligible managed attempt cannot be started |
| `customer-only` | Customer pools | Fail closed; never select managed capacity |
| `managed-only` | Managed pools | Fail closed; never select customer capacity |
| `fallback` | Configured primary class, then explicitly allowed secondary class | Emit a policy transition event and use the secondary class only when allowed |

Fallback is an allowlisted policy, not permission to try every pool. A
capacity error must be classified before fallback: authorization, tenant,
security, and workload-policy failures are terminal and must not be bypassed;
transient capacity or provider availability failures may be eligible. Every
decision records the policy, candidates considered, selected pool, and reason.

## Acceptance Gates

1. The same lifecycle path can select a customer or managed pool without
   changing GitHub-facing semantics.
2. The five policies above have deterministic unit-testable behavior,
   including empty pools, ineligible pools, and terminal failures.
3. Cross-tenant pools and credentials are rejected before provisioning.
4. Runner and controller identities are separate, and JIT material is never
   written to Terraform state, ordinary pool metadata, or telemetry.
5. Reconciliation can identify ownership and tenant from durable records and
   provider-owned tags/labels without guessing.
6. No implementation or deployment action changes another account/project
   without an explicit, reviewed customer-side installation step.

## Out Of Scope And Follow-Up

Terraform modules remain contracts until provider-specific deployment review
and an approved environment exist. Real multi-tenant load, quota management,
managed billing, autoscaling, and cross-region optimization are follow-up
work. Phase 10 may consume the policy and telemetry events, but intelligence
must remain outside the critical path.
