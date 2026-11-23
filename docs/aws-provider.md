# AWS Provider

Phase 2 maps a cloud-neutral `RunnerSpec` to one ephemeral EC2 instance. The provider uses `RunInstances` with a configured Launch Template, not EC2 Fleet. The Launch Template owns the AMI, instance profile, network defaults, storage, and bootstrap. The provider supplies per-job tags and may override only explicitly allowed fields.

## Required behavior

- Validate region, Launch Template ID/version, and required network configuration before launch.
- Launch exactly one instance per runner lease.
- Apply the `leo-runners:managed-by=leo-runners` ownership tag, per-job tags (`leo-runners:runner-id`, `job-id`, `repository`, `run-id`, `workflow`, `created-at`, `expires-at`) and any configured extra tags (`AWS_RUNNER_TAGS`) at launch.
- Require IMDSv2 using `MetadataOptions.HttpTokens=required`.
- Poll `DescribeInstances` with a context deadline until the instance is running and status checks are healthy.
- Treat missing, terminated, or failed instances as provider errors.
- Make termination repeatable and safe after partial provisioning.

The controller role launches and describes instances using a restricted Launch Template. The runner instance role is separate and has only workload-approved permissions. No GitHub PAT, JIT token, or long-lived secret is stored in the AMI.

## Operations

Two independent mechanisms remove expired runners, so neither a lost record nor a lost instance can leave one running:

1. **State-driven.** The reconciler reads job, runner and lease records and terminates runners whose expiry has passed, and runners whose job record is missing.
2. **Provider-side backstop.** Each pass also lists the instances this deployment owns by the `leo-runners:managed-by` tag (plus any configured custom tags), without consulting state, and terminates any whose `leo-runners:expires-at` tag is more than ten minutes in the past. This catches an instance whose record was never written (a crash between launch and persist), was lost, or was deleted. An instance with no readable expiry tag is never removed on that basis.

Records outlive their expiry by seven days (the table's TTL is `ttl_at`, not `expires_at`), but the backstop does not depend on them. Cloud API retries must respect context deadlines and avoid creating a second instance for the same persisted provisioning attempt.

References: [EC2 `run-instances`](https://docs.aws.amazon.com/cli/latest/reference/ec2/run-instances.html), [launch templates](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/launch-instances-from-launch-template.html), and [AWS EC2 IAM examples](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ExamplePolicies_EC2.html).
