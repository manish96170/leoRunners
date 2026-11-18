# Network Validation

This directory validates the runner VPC reachability contract offline and
provides an opt-in, read-only measurement harness. It does not create, change,
or delete VPCs, routes, NAT gateways, endpoints, security groups, DNS records,
or instances.

## Offline validation

```bash
tools/network-validation/validate.sh
tools/network-validation/test.sh
```

The matrix checks private-subnet placement, approved DNS resolution, TLS/443
destinations, selected egress-mode compatibility, inbound isolation, IMDSv2,
and cost labels for NAT or VPC endpoints. The report never includes credentials,
headers, query strings, or raw resolver output.

## Guarded measurement

Measurement is intentionally blocked unless both flags are supplied:

```bash
tools/network-validation/measure.sh \
  --allow-live \
  --confirm I_UNDERSTAND_NETWORK_MEASUREMENT \
  --output /tmp/network-measurement.json
```

The harness performs DNS resolution and bounded HTTPS requests to the matrix
destinations, recording only pass/fail, the matrix hostname, egress-mode label,
and elapsed milliseconds. It is read-only from the infrastructure perspective;
it does not verify or mutate route tables. Run it from the actual private runner
subnet to measure NAT versus endpoint behavior, then reconcile the labels and
bytes with provider billing data. A failed required check must block rollout.

Do not put private hostnames, tokens, signed URLs, request payloads, or resolver
answers in the matrix or report. Use opaque destination IDs when a hostname is
sensitive.
