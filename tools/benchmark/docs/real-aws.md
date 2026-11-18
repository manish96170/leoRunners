# Opt-in AWS benchmark procedure

The default benchmark is offline. Do not add cloud calls to the fake path.

For a controlled run, first verify the identity and region with the AWS CLI,
then use a disposable GitHub repository, a short runner expiry, a strict
budget alarm, and a cleanup trap. The controller must emit the seven observed
milestones into a `Collector`; record the resulting samples with the same
output writers used by the fake replay.

The guarded smoke command is:

```sh
AWS_PROFILE=ci-platform AWS_REGION=us-east-1 \
  go run . --mode real-aws --allow-real-aws --format json --output real-aws.json
```

This repository intentionally does not make that command provision anything:
`real-aws` exits with an integration-hook message until a separately reviewed
controller adapter supplies observed lifecycle callbacks. This prevents a
benchmark invocation from becoming an accidental billable deployment.

Before enabling such an adapter, validate `aws sts get-caller-identity`, IAM
scope, Launch Template version, subnet egress, IMDSv2, GitHub token scope, and
that every created instance is terminated even when registration or readiness
fails.
