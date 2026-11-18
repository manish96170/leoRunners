# Phase 18: Network Reachability and Isolation

## Purpose

Phase 18 defines the network contract for ephemeral CI runners across AWS,
GCP, and customer-managed capacity. A runner must be able to reach GitHub and
the package registries required by its approved workload, while inbound access
and cross-tenant paths remain closed. Network availability is a prerequisite
for runner registration and job execution, but network optimization must not
change scheduling ownership or bypass workload policy.

## Deliverables

1. Publish the versioned network policy in
   [`network/config.v1.yaml`](../network/config.v1.yaml).
2. Document subnet, egress, DNS, security-group, provider, and fork-isolation
   decisions in [`docs/networking.md`](networking.md).
3. Validate reachability with evidence from the exact image, provider, region,
   subnet, route table, and security profile used by the workload.
4. Keep live network changes outside the controller. Infrastructure owners
   must review and apply Terraform or cloud-console changes separately.

## Stages

### 18.0 Inventory and offline validation

- Identify every GitHub API, webhook, artifact, package registry, container
  registry, and operating-system mirror required by approved workloads.
- Record whether each destination uses HTTPS, DNS, a proxy, or a private
  endpoint.
- Validate the policy fixture, provider mapping, forbidden ingress rules, and
  secret-free evidence schema without contacting a cloud service.

### 18.1 Read-only preflight

- Confirm account or project, VPC, subnet, route table, NAT or endpoint path,
  DNS resolver, security group or firewall, and image identity.
- Run DNS resolution and HTTPS checks from the candidate runner network, using
  a harmless endpoint and bounded timeouts.
- Capture route, endpoint, and security metadata without storing credentials,
  tokens, full user data, or customer payloads.

### 18.2 One approved workload

- Use one immutable repository revision, one provider, one region or zone, one
  capacity slot, and one short-lived runner.
- Confirm GitHub registration, package installation, and cleanup from the
  actual ephemeral instance.
- Reconcile and repeat cleanup after the job, then review cost and reachability
  evidence before expanding the policy.

### 18.3 Controlled expansion

- Add providers, regions, registries, and trust boundaries one at a time.
- Prefer private subnets and private endpoints where they reduce exposure and
  have an acceptable operational and cost profile.
- Revert a profile to preflight-only or deny a destination when evidence shows
  unexpected public ingress, DNS drift, excessive NAT cost, or cross-boundary
  access.

## Acceptance gates

- Every required destination has an owner, protocol, port, DNS name, provider
  mapping, and evidence reference.
- No inbound internet access is required for runner registration or job work.
- Outbound access is limited to approved destinations or an approved egress
  proxy; security groups are stateful and have no unrestricted inbound rule.
- DNS resolution uses the intended VPC or cloud resolver and does not leak
  private names to an unapproved resolver.
- Untrusted forks cannot reach trusted cache, metadata, control-plane, or
  customer-network destinations beyond the explicitly approved workload path.
- NAT, endpoint, egress, and cross-zone costs are estimated and attributed
  before live rollout.
- A failed reachability check fails preflight with a useful reason; it does
  not trigger automatic security-group, route, IAM, or cross-account changes.
- Evidence proves the exact provider, region, image, subnet, routes, DNS path,
  destination result, timestamps, cleanup, and policy version without secrets.

## Non-goals

- No automatic VPC, subnet, route, NAT, endpoint, DNS, firewall, or security
  group mutation.
- No public exposure of the controller, webhook endpoint, runner ports, or
  instance metadata service.
- No assumption that one cloud's subnet, endpoint, or firewall model maps
  directly to another provider.
- No broad allowlist such as `0.0.0.0/0` as a substitute for destination
  ownership and evidence.

## Exit evidence

Store a redacted report containing policy version, workload and commit
identity, provider, account or project fingerprint, region or zone, image,
subnet class, route and egress mode, DNS mode, security profile, destination
checks, latency and status, cost classification, and cleanup result. Keep raw
URLs, resolver logs, instance metadata, credentials, signed requests, and
customer payloads out of general telemetry.
