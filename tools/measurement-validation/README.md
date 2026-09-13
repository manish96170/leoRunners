# Measurement validation

validate.sh validates the Phase 50 measurement contract offline. It makes no
cloud, network, S3, pricing API, Terraform, or credential calls.

It checks:

- synthetic/real provenance and evidence-level consistency;
- stable workload and comparison identity;
- cache hit/miss arithmetic and redacted S3 namespace references;
- byte units and network egress, VPC endpoint, and NAT attribution;
- startup phase units and total timing;
- USD cost arithmetic, bounded assumptions, and pricing provenance;
- mandatory redaction and secret-shaped content exclusion.

Run the focused suite:

    ./tools/measurement-validation/test.sh

Pass a second evidence file to enforce pair comparability:

    ./tools/measurement-validation/validate.sh baseline.json candidate.json

Exit codes are 0 for valid evidence, 1 for invalid or incomparable evidence,
and 2 for missing tools/files or malformed JSON. A fixture is not a live
measurement.
