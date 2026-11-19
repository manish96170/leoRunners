# aws-ecs-controller

Runs the LeoRunners controller on ECS Fargate behind an existing Application
Load Balancer. It is generic: every account, region, VPC, subnet, ALB,
certificate, tag, repository-scope, and secret reference is a variable. The
module never receives a secret value and never assumes a DNS provider,
CloudFront, Elastic IP, NAT gateway, or any particular ALB.

Customer-specific compositions (real ARNs, IDs, hostnames, tag policy, change
approval) live in a separate infrastructure repository that calls this module.
Nothing tenant-specific belongs here.

## Two safe phases

| Phase | Flags | Creates | Never creates |
|---|---|---|---|
| 0 inert | none (default) | no AWS resources (only a state-only `terraform_data` guard that holds the plan-time checks) | everything |
| 1 foundation | `create_foundation = true` | ECR repository, log group, DynamoDB state table, empty secret containers, execution and task roles, task security group, target group, optional ECS cluster | task definition, ECS service, listener rule |
| 2 service | `enable_listener_rule = true`, `enable_service = true` | listener rule, task definition, Fargate service | anything outside the controller |

Phase 2 is refused at plan time until all of these exist:

- `image_digest` is an immutable `sha256:` digest (mutable tags are rejected by input validation);
- `secrets_ready = true`, the operator's assertion that every referenced secret holds an approved value;
- a reviewed runtime policy (`task_role_policy_json` and/or `task_role_policy_arns`);
- `listener_arn`, `listener_path_patterns`, and `listener_priority`, with the route enabled;
- subnets and an ECS cluster (`create_cluster` or `cluster_arn`).

The listener rule can be enabled before the service (a reviewed route with no
target) and the service can start with `desired_count = 0` for a dry run.

## Required inputs

| Input | Type | Notes |
|---|---|---|
| `name_prefix` | string | 3-20 chars; prefixes every name |
| `egress_https_cidr_blocks` | list(string) | no default; choose who the task may reach on 443 |
| `vpc_id`, `alb_security_group_id` | string | required once `create_foundation = true` |

Everything else has a default or is required only by the phase that uses it.
`variables.tf` is the authoritative contract; `variables.example.tfvars` is a
non-secret starting point.

### Phase 2 inputs

| Input | Purpose |
|---|---|
| `image_digest`, plus `create_ecr_repository = true` or `image_repository` | immutable image reference |
| `secrets_ready` | operator assertion that secrets are populated |
| `task_role_policy_json` / `task_role_policy_arns` | reviewed runtime permissions |
| `listener_arn`, `listener_path_patterns`, `listener_priority` (`listener_host_headers` optional) | ALB integration; no path is hardcoded |
| `subnet_ids`, `assign_public_ip` | where tasks run |
| `create_cluster` or `cluster_arn` | cluster |
| `cpu`, `memory`, `cpu_architecture` | sizing; architecture must match the image |

### GitHub and runner settings

`github` controls the runtime contract:

- `jit_enabled = false` (default): the controller verifies webhooks but provisions nothing real (fake provider).
- `jit_enabled = true`: requires `runner_group_id`, an organization or repository allowlist, a label allowlist, and one way to authenticate to GitHub. These mirror the controller's own startup validation, so a misconfiguration fails at plan time and not as a crash loop.

GitHub authentication is one of two mutually exclusive modes:

| Mode | Set | Secret container created | Notes |
|---|---|---|---|
| **GitHub App (recommended)** | `github.app = { app_id, installation_id, token_permissions }` | `<name_prefix>/github-app-private-key` | the controller signs a short-lived JWT with the key and mints its own installation tokens, refreshing before they expire; no redeploy for token expiry |
| Static installation token | leave `github.app` null | `<name_prefix>/github-installation-token` | the token expires after about an hour and must be rotated externally; suited to short validation runs only |

In App mode the private key is only ever a Secrets Manager reference. Create the
GitHub App yourself, store its PEM key in the created container outside
Terraform, and see `docs/github-runtime-contract.md` for the permissions.
`app.token_permissions` optionally narrows each minted token below what the App
was granted, for example `{ administration = "write" }`.

`runner` stays disabled. Enabling it requires `github.jit_enabled`, the
launch template, the dedicated runner role and instance profile ARNs, and a
`task_role_policy_json` whose `iam:PassRole` statement names exactly
`runner.role_arn`. See `../aws-ephemeral-runner`.

## What the module refuses

- `iam:PassRole` allowed on any resource containing `*`
- `Allow` with `NotAction`, `Action = "*"`, or wildcard `iam:` actions
- credential-looking `extra_environment` keys
- `desired_count` above 1 (the controller does not wire multi-replica coordination yet)
- secret values: only Secrets Manager ARNs are accepted

The policy checks cannot prove a policy is least privilege. They reject shapes
that are never acceptable here; a human still reviews the generated policy
(see `docs/aws-iam-policy-workflow.md`).

## Outputs

| Output | Meaning |
|---|---|
| `phase` | `disabled`, `foundation`, or `service` |
| `ecr_repository_url`, `ecr_repository_arn` | where to publish the image |
| `state_table_name`, `state_table_arn`, `state_index_arns` | state table and indexes (feed the policy review) |
| `webhook_secret_arn`, `github_token_secret_arn`, `github_app_private_key_secret_arn` | secret containers to populate outside Terraform |
| `execution_role_arn`, `task_role_arn`, `task_role_name` | the two separate roles |
| `task_security_group_id`, `target_group_arn`, `cluster_arn` | network and routing |
| `listener_rule_arn`, `service_name`, `task_definition_arn`, `image_uri` | phase 2 results (null before) |
| `container_environment` | the non-secret environment the container receives, for review |
| `container_secret_names` | names (never values) of the variables injected from Secrets Manager |
| `policy_review_scope` | account, region, table and index ARNs, runner role: inputs for Autopilot review |
| `log_group_name`, `log_group_arn` | controller logs |

## Example

```hcl
module "leo_runners_controller" {
  # Pin to an immutable release; see docs/aws-modules.md.
  source = "git::https://github.com/manish96170/leoRunners.git//terraform/modules/aws-ecs-controller?ref=tf-aws-v0.1.0"

  name_prefix              = "leo-runners"
  create_foundation        = true
  vpc_id                   = var.vpc_id
  alb_security_group_id    = var.alb_security_group_id
  egress_https_cidr_blocks = var.egress_https_cidr_blocks
  create_cluster           = true
  tags                     = var.tags
}
```

Offline tests: `terraform init -backend=false && terraform test` in this directory
(uses a mocked AWS provider; no credentials, no AWS calls).

## Known limits

- One controller replica. Rolling updates stop the old task before starting the new one.
- Traffic from the ALB to the task is HTTP inside the VPC. This is not end-to-end TLS.
- A static `GITHUB_APP_INSTALLATION_TOKEN` is read once at process start and expires after about an hour. Use `github.app` for anything longer than a short validation run; see `docs/github-runtime-contract.md`.
