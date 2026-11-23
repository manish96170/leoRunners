# aws-runner-layer

Creates what the controller needs to launch ephemeral EC2 runners, and nothing
else:

| Resource | Notes |
|---|---|
| Launch template | IMDSv2 required (hop limit 1, no instance metadata tags), encrypted gp3 root volume deleted with the instance, `terminate` on shutdown, **no public IP** (the template owns the network interface), ownership tags on the instance, volume and network interface. It never holds user data. |
| IAM role and instance profile | EC2 may assume the role. It starts with **no permissions**. |
| Security group (optional) | **No inbound rules.** Egress is exactly what you list. |

It does not create the AMI, subnet, VPC, NAT, endpoints, the controller, or any
GitHub setting. Every value is an input; no name, topology, account or label is
assumed. The security settings come from the `aws-ephemeral-runner` contract
module, which this module calls, so the contract and the implementation cannot
drift.

**Status:** validated offline (mocked provider, AWS provider v5 and v6) and by
composition with `aws-ecs-controller`. **Not yet exercised on real EC2.**

## How it fits together

```text
aws-runner-layer ──controller_runner──> aws-ecs-controller (runner = ...)
        │                                       │
        │ role_arn                              │ task role
        └──────── iam:PassRole, exactly this ───┘   (in the reviewed task policy)
```

```hcl
module "runner_layer" {
  source = "git::https://github.com/<owner>/<repo>.git//terraform/modules/aws-runner-layer?ref=<commit-sha>"

  name_prefix              = "example"
  ami_id                   = var.runner_ami_id
  instance_type            = "c7i.xlarge"
  vpc_id                   = var.vpc_id
  subnet_id                = var.runner_subnet_id
  egress_https_cidr_blocks = var.runner_egress_cidrs
  request_id               = var.request_id
  tags                     = var.tags
  controller_role_arn      = module.controller.task_role_arn   # optional check
}

module "controller" {
  source = "git::https://github.com/<owner>/<repo>.git//terraform/modules/aws-ecs-controller?ref=<same-commit-sha>"
  # ...
  runner = merge(module.runner_layer.controller_runner, { labels = ["self-hosted", "linux"] })

  # The reviewed policy must include iam:PassRole on exactly
  # module.runner_layer.role_arn (see "Controller policy" below).
  task_role_policy_json = local.reviewed_policy_json
}
```

Pin the modules and the controller image to **the same commit**.
`terraform/examples/aws-customer` composes all of it, with a test that proves
there is no dependency cycle between the layer and the controller.

## Inputs

| Input | Required | Default | Notes |
|---|---|---|---|
| `name_prefix` | yes | | 3-32 chars, lowercase letters, digits, hyphens |
| `ami_id` | yes | | immutable runner AMI; the module never picks one |
| `instance_type` | yes | | size it for the workload |
| `subnet_id` | yes | | the template defines the network interface here |
| `vpc_id` | with `create_security_group` | `null` | VPC of `subnet_id` |
| `create_security_group` | | `true` | ingress-free group |
| `security_group_ids` | | `[]` | existing groups, in addition to or instead of the created one; they should allow no inbound traffic |
| `egress_https_cidr_blocks` | with `create_security_group` (or a prefix list) | `[]` | no default: runners that cannot reach GitHub cannot register |
| `egress_https_prefix_list_ids` | | `[]` | for example S3 gateway endpoints |
| `egress_dns_cidr_blocks` | | `[]` | only for custom resolvers |
| `role_name`, `iam_path`, `permissions_boundary_arn` | | `<prefix>-runner`, `/`, `null` | |
| `managed_policy_arns` | | `[]` | attached to the runner role |
| `role_policy_json` | | `null` | inline policy for what workflows legitimately need; rejects `Action "*"`, `Allow`+`NotAction`, wildcard `iam:PassRole`, wildcard `iam:` actions |
| `enable_ssm` | | `false` | Session Manager access |
| `controller_role_arn` | | `null` | when set: must differ from the runner role and share its account |
| `root_device_name`, `root_volume_size_gb`, `root_volume_iops`, `root_volume_throughput_mibps`, `root_volume_kms_key_arn` | | `/dev/xvda`, 100, 3000, 125, `null` | encryption is never optional |
| `user_data` | | bundled `bootstrap/runner/user-data.sh` | must contain `{{GITHUB_JIT_CONFIG_B64}}` exactly once |
| `max_lifetime_minutes` | | `120` | 5-1440; the hard lifetime the reaper enforces |
| `request_id`, `cleanup_deadline` | | `null` | stamped on every runner; `request_id` enables the cleanup selector |
| `tags` | | `{}` | no comma in keys or values, no `=` in keys, no `leo-runners:` or `aws:` prefix |

## Outputs

| Output | Use |
|---|---|
| `controller_runner` | ready-made `runner` object for `aws-ecs-controller`; add `labels` (and `pool_id` if needed) |
| `launch_template_id`, `launch_template_arn`, `launch_template_version` | the template and its exact version |
| `role_arn`, `role_name` | the runner role: `iam:PassRole` must name exactly `role_arn` |
| `instance_profile_arn`, `instance_profile_name` | |
| `security_group_id`, `security_group_ids`, `subnet_id` | |
| `ownership_tags` | tags on every runner |
| `cleanup_selector` | tag intersection for exactly this request's runners; `null` without `request_id` |
| `controller_policy_constraints` | review checklist for the controller policy |
| `user_data` | the bootstrap the controller adds at launch |

`controller_runner` sets `subnet_id = null` and `security_group_ids = []` on
purpose: EC2 rejects a launch that names a subnet or security groups **and**
a template network interface. It also sets `max_runners = 1`; raise it
deliberately.

## What the controller does with it

`aws-ecs-controller` turns `runner` into environment: `AWS_LAUNCH_TEMPLATE_ID`,
`AWS_LAUNCH_TEMPLATE_VERSION`, `AWS_RUNNER_INSTANCE_PROFILE_ARN`,
`CAPACITY_LABELS`, `CAPACITY_MAX_RUNNERS`, `RUNNER_MAX_LIFETIME`,
`RUNNER_USER_DATA` and `AWS_RUNNER_TAGS`. At launch the controller adds the
user data (with GitHub's one-time JIT configuration substituted for the
placeholder) and per-job tags under the `leo-runners:` prefix. The template
itself never contains a credential.

## Tags and cleanup

Every instance carries `leo-runners:managed-by=leo-runners`, the key the
controller's provider stamps, selects on, and its reaper backstop lists by.
`AWS_RUNNER_TAGS` makes the controller send `tags`, `RequestId` and
`CleanupDeadline` itself on every launch, so they are present whatever the
template says, and the backstop only considers instances carrying all of them.
Cleanup uses the full `cleanup_selector` intersection and nothing else; never
find runners by name, VPC, subnet or instance type.

## Controller policy

The controller role needs, at minimum (see `docs/aws-iam-policy-workflow.md`):

- `iam:PassRole` on exactly `role_arn`, condition `iam:PassedToService = ec2.amazonaws.com`. `controller_policy_constraints.pass_role` states it.
- `ec2:RunInstances` on the launch template (`launch_template_arn`), the AMI, the subnet, the security groups, and the instance, volume and network-interface resources, with the request tag `leo-runners:managed-by = leo-runners`.
- `ec2:CreateTags` only with `ec2:CreateAction = RunInstances`.
- `ec2:TerminateInstances` only on resources tagged `leo-runners:managed-by = leo-runners`.
- `ec2:DescribeInstances` (it cannot be resource-scoped).

## Bootstrap and the AMI

With `user_data` unset the bundled `bootstrap/runner/user-data.sh` downloads a
pinned GitHub runner at every launch and verifies its SHA-256. That needs egress
to GitHub's download hosts and costs launch time. Production images should
bake the runner in (`ami/`) and pass a shorter `user_data`. Tool versions a
workload needs (language runtimes, browsers, Terraform) belong in the image
profile, not in this module.

## Known limits

- Not yet run on real EC2. Things to confirm on the first real launch: the
  launch succeeds with the template's network interface and no subnet in the
  request; the instance and volume carry `leo-runners:managed-by`,
  `leo-runners:runner-id` and your `RequestId`; the instance has no public IP;
  IMDSv1 is refused.
- One subnet. A multi-subnet layout needs several launch templates or a
  controller that chooses a subnet; not built.
- The default egress is none. A runner without a route to GitHub never
  registers and is reaped at its lifetime.
- A runner role with permissions runs them for every pull request that can use
  the runner. Keep forks denied (the controller default) and scope the role.

Offline tests: `terraform init -backend=false && terraform test`.
