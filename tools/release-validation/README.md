# Release Validation

This directory contains the final offline release gate for Leo Runners. The
aggregator runs the existing controller, extension, observability, evidence,
cloud-fixture, Docker, Kubernetes, IAM, and Terraform static test suites.

It is read-only: it never calls live cloud provisioning, GitHub runner
registration, Terraform apply, or Kubernetes apply. Missing local tooling is a
`WARN`; an actual validator failure is a `FAIL`.

```bash
tools/release-validation/validate.sh --report /tmp/leo-release.json
tools/release-validation/test.sh
```

Exit codes are `0` for all pass, `2` for warnings only, and `1` for failures.
Reports are atomically written with owner-only permissions and contain bounded,
redacted messages. The report contract is in
`validation/release-report.schema.v1.json`.
