# Network Policy Fixtures

`config.v1.yaml` is a versioned, provider-neutral network policy fixture for
ephemeral CI runners. It is intended for offline validation and read-only
preflight. It does not create subnets, routes, NAT gateways, endpoints, DNS
records, security groups, firewall rules, or cross-account resources.

## Contract

- `preflight-only` is the default rollout mode.
- Private subnets are preferred; public subnets require explicit approval.
- GitHub and workload registry reachability must be listed and evidenced.
- NAT, endpoint, proxy, and cross-zone cost assumptions must be recorded.
- Inbound access is denied by default; outbound access is destination scoped.
- DNS mode and provider mapping are explicit policy fields.
- Forks and other untrusted workloads use an isolated trust boundary.
- Failed checks block rollout and never cause automatic network mutations.
- Validation evidence is redacted and excludes secrets, tokens, user data, and
  complete request payloads.

## Safe update process

1. Copy the policy to a new version and preserve the prior fixture.
2. Add an owner, destination purpose, protocol, port, provider mapping, and
   evidence reference for every new destination.
3. Review subnet, egress, DNS, firewall, endpoint, and fork-isolation changes.
4. Run offline validation and read-only checks from the exact runner image and
   placement.
5. Test one approved immutable workload, including cleanup and cost evidence.
6. Expand one provider, region, registry, or trust boundary at a time.

## Evidence fields

Record policy version, workload revision, provider, region or zone, account or
project fingerprint, image, subnet class, egress mode, DNS mode, security
profile, destination ID, check result, latency, timestamp, cost class, and
cleanup result. Use opaque fingerprints for sensitive identifiers. Never put
credentials, signed URLs, authorization headers, full user data, or customer
payloads in logs or metric labels.
