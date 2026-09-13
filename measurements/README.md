# Phase 50 Measurement Evidence

This directory defines an offline, versioned evidence contract for cache,
network, startup, and cost measurements. The sample is synthetic and makes no
claim about AWS, GCP, or invoice pricing.

schema.v1.json is the normative contract and evidence.v1.json is a safe
fixture. Namespaces are represented by bounded ref: values; raw S3 bucket
names, object prefixes, cache keys, account IDs, URLs, tokens, and payloads do
not belong in evidence.

All traffic is recorded in bytes, time in integer milliseconds, and money in
USD. Every record carries collector provenance, execution class, a comparison
identity, cache operation counters, network path attribution, startup phases,
and explicit pricing assumptions.

Validate with:

    ./tools/measurement-validation/test.sh

Synthetic and real records must not be compared. A real record requires a
controlled collector and current pricing source; the fixture remains synthetic
even when its values resemble a live run.
