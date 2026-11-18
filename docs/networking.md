# Ephemeral Runner Networking

Networking must give a short-lived runner the smallest useful outbound path.
The runner needs GitHub and the package or artifact services required by its
approved workload. It does not need inbound SSH, a public listener, or access
to the controller's host filesystem.

## Public versus private subnets

Private subnets are the default for runners. They avoid public addresses and
make inbound exposure less likely. They require a deliberate egress design:
NAT, an egress proxy, or private service endpoints. Their additional route,
DNS, endpoint, and operational dependencies must be tested from the actual
subnet.

Public subnets can simplify early experiments because a public address may
provide direct outbound internet access. They increase exposure, complicate
address allowlisting, and make an accidental inbound rule more damaging. A
public subnet is allowed only for an owner-approved validation profile with no
inbound security-group rule, no sensitive workload, bounded lifetime, and
documented cleanup. Public IP assignment must never be the implicit fallback.

## Required reachability

Inventory destinations from workflow evidence and the runner image. Typical
classes are:

- GitHub API and Actions service endpoints for JIT configuration, registration,
  job coordination, and artifact operations.
- Package registries such as npm, PyPI, Maven, crates.io, Go module mirrors,
  NuGet, Terraform Registry, and private registries used by the workload.
- Container registries and image mirrors used during approved builds.
- Operating-system mirrors only when the immutable image cannot provide the
  required packages.

Use HTTPS and certificate validation. Do not hardcode an unreviewed IP address
for a hostname whose provider publishes changing addresses. The destination
allowlist is a policy input, not a reason to log access tokens or full request
URLs.

## NAT and VPC endpoints

NAT gateways provide a familiar path from private subnets to public services,
but they add hourly, processing, and sometimes cross-zone transfer costs.
Centralized NAT can reduce duplicated gateways while adding routing and
cross-zone dependencies. Place egress intentionally, measure bytes and job
duration, and include idle capacity in the cost estimate.

VPC endpoints can keep supported AWS service traffic on private paths and may
reduce NAT processing. Gateway endpoints and interface endpoints have different
service coverage, pricing, DNS, security-group, and availability behavior.
Use endpoints for services they actually cover; GitHub and many public package
registries still require an approved internet egress or proxy. Do not claim
that endpoints provide private connectivity to an arbitrary public hostname.

The equivalent GCP choice may involve Private Google Access, Private Service
Connect, Cloud NAT, or an egress proxy. These are not interchangeable with AWS
endpoints and must be mapped and tested per provider.

## Security groups and firewall rules

Runner security profiles should allow established return traffic and the
minimum outbound TCP 443 path required by the policy. Inbound rules should be
empty unless a workload-specific exception is reviewed. Do not open SSH,
RDP, runner listener ports, or all TCP from the internet for convenience.

The controller, runner, cache, state store, and customer network are separate
trust boundaries. Network reachability does not grant authorization. Keep
instance metadata access protected by IMDSv2 on AWS and the provider's
equivalent identity controls on GCP; do not expose metadata through a proxy.

## DNS

Use the cloud VPC resolver or an approved forwarding path. Capture the resolver
mode and outcome, not sensitive query payloads, in validation evidence. Verify
that private names resolve only inside the intended network and that split-horizon
records cannot redirect a trusted workload to an unapproved destination.

DNS failure, stale records, certificate mismatch, and proxy misconfiguration
must be distinct preflight outcomes. A fallback to a public resolver requires
an explicit policy decision because it can leak private names and make routing
less predictable.

## Untrusted forks and isolation

Treat fork and pull-request code as untrusted. It must not inherit trusted
repository cache namespaces, customer network routes, private package
credentials, controller credentials, or broad cloud identity permissions.
Use a separate security profile, capacity pool, subnet or egress policy when
the workload can execute arbitrary code. Prefer public package mirrors through
an allowlisted proxy or a narrowly scoped read-only credential.

A fork may be denied rather than placed on a trusted pool when isolation cannot
be proven. The scheduler must not bypass this decision because a pool is idle.
Cache policy, network policy, and runner identity must share the same trust
boundary identifier.

## Provider mapping

| Concern | AWS | GCP | Customer-managed |
| --- | --- | --- | --- |
| Private compute path | Private subnet | Subnet without external IP | Customer VLAN or private subnet |
| Public egress | NAT gateway or proxy | Cloud NAT or proxy | Approved egress proxy/firewall |
| Private cloud services | Gateway or interface VPC endpoints | Private Google Access or Private Service Connect | Customer-specific endpoint |
| Policy control | Security group, NACL, route table | VPC firewall, routes, tags/service account | Firewall, ACL, route policy |
| Identity boundary | Instance profile and IMDSv2 | VM service account and metadata controls | Customer-issued short-lived identity |
| DNS | VPC resolver/Route 53 Resolver | Cloud DNS resolver | Customer resolver/forwarder |

The table is a mapping aid, not proof of equivalent behavior. Record the
provider-specific resource IDs and evidence for each live profile.

## Evidence-driven rollout

Start with `network/config.v1.yaml` in `preflight-only` mode. Validate policy
syntax and the destination inventory offline, then run read-only checks from
the exact image and network placement. Keep account, project, route, and
endpoint identifiers in owner-only evidence storage.

For the first live workload, use one immutable commit and one runner. Check
DNS, TCP/TLS reachability, GitHub JIT/registration, package installation,
workflow completion, and cleanup. Record whether the path used NAT, an
endpoint, or a proxy and classify transfer and hourly charges as estimates
until matched to billing data.

Expand only after repeated evidence shows the required destinations succeed,
no unexpected inbound path exists, fork isolation holds, and cleanup is
idempotent. Roll back by selecting a previous policy version or disabling the
profile. Do not automatically edit routes, firewall rules, endpoints, IAM, or
cross-account resources in response to a failed check.
