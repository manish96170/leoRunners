# AWS Provider

Phase 2 maps a cloud-neutral `RunnerSpec` to one ephemeral EC2 instance. The provider uses `RunInstances` with a configured Launch Template, not EC2 Fleet. The Launch Template owns the AMI, instance profile, network defaults, storage, and bootstrap. The provider supplies per-job tags and may override only explicitly allowed fields.

## Required behavior

- Validate region, Launch Template ID/version, and required network configuration before launch.
- Launch exactly one instance per runner lease.
- Apply `Platform=ci-runner`, `ManagedBy=leo-runners`, repository, workflow run, job, runner, `CreatedAt`, and `ExpiresAt` tags at launch.
- Require IMDSv2 using `MetadataOptions.HttpTokens=required`.
- Poll `DescribeInstances` with a context deadline until the instance is running and status checks are healthy.
- Treat missing, terminated, or failed instances as provider errors.
- Make termination repeatable and safe after partial provisioning.

The controller role launches and describes instances using a restricted Launch Template. The runner instance role is separate and has only workload-approved permissions. No GitHub PAT, JIT token, or long-lived secret is stored in the AMI.

## Operations

The reaper finds instances by the ownership tags and expiry timestamp. It terminates expired or orphaned instances even after controller restart. Cloud API retries must respect context deadlines and avoid creating a second instance for the same persisted provisioning attempt.

References: [EC2 `run-instances`](https://docs.aws.amazon.com/cli/latest/reference/ec2/run-instances.html), [launch templates](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/launch-instances-from-launch-template.html), and [AWS EC2 IAM examples](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ExamplePolicies_EC2.html).
