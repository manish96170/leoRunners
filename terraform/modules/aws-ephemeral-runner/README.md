# aws-ephemeral-runner (contract only)

This module **creates no resources and deploys no runners.** It defines and
validates the contract that the future ephemeral EC2 runner layer must honor,
so that the controller module, the policy review, and cleanup tooling all
agree before anything launches. Every value is supplied as a variable.

## What it fixes (not configurable)

- IMDSv2 required (`http_tokens = required`, hop limit 1, instance metadata tags off)
- encrypted gp3 root volume, deleted on termination
- no public IP, `instance_initiated_shutdown_behavior = terminate`
- ownership tags `Platform=ci-runner` and `ManagedBy=leo-runners` that callers cannot override (they match the AWS provider in `controller/internal/providers/aws`)

## Inputs

| Input | Notes |
|---|---|
| `name_prefix`, `ami_id`, `instance_types`, `subnet_ids`, `security_group_ids` | supplied by the customer; no names or topology assumed |
| `runner_role_arn`, `runner_instance_profile_arn` | the dedicated runner identity; exact ARNs, no wildcards |
| `controller_role_arn` | the controller task role allowed to pass the runner role |
| `root_volume_size_gb`, `root_volume_iops`, `root_volume_throughput_mibps`, `root_volume_kms_key_arn` | root volume sizing; encryption is always on |
| `max_lifetime_minutes` | hard lifetime (5-1440, default 120) |
| `request_id`, `cleanup_deadline` | optional request scoping for cleanup |
| `tags` | extra ownership tags |

Validation rejects: wildcard role ARNs, a runner role identical to the
controller role, roles and profile in different accounts, malformed AMI,
subnet, and security-group IDs, and any attempt to override `Platform` or
`ManagedBy`.

## Outputs (the contract)

| Output | Use |
|---|---|
| `launch_template_spec` | the arguments the future launch template resource must use |
| `ownership_tags` | tags for runner instances and volumes |
| `max_lifetime_minutes` | lifetime enforced through `ExpiresAt` and the reaper |
| `controller_policy_constraints` | the review checklist for the controller policy: `iam:PassRole` limited to exactly the runner role and `ec2.amazonaws.com`; `ec2:RunInstances` requires the ownership request tags and is limited to the listed AMI, subnets, and security groups; terminate and describe require the ownership resource tags |
| `cleanup_selector` | the tag intersection (`Platform`, `ManagedBy`, `RequestId`) that identifies exactly one request's runners; null when `request_id` is unset |

## Request-scoped cleanup

Cleanup must select resources by the full tag intersection in
`cleanup_selector` and by nothing else. Never discover by name, VPC, subnet,
or instance type. If `request_id` is unset there is no selector and no
request-scoped cleanup is available. See `docs/aws-rollback-and-cleanup.md`.

## Enabling runner lifecycle later

`aws-ecs-controller` accepts a `runner` object that stays disabled by default.
It will not enable the lifecycle unless the dedicated role and instance profile
ARNs, a launch template ID, GitHub JIT, and a controller policy whose
`iam:PassRole` names exactly the runner role are all present. A future module
version will add the launch template and runner IAM resources behind these same
inputs.

Offline tests: `terraform init -backend=false && terraform test` (no provider needed).
