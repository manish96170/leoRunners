# AWS Deployment Modules

LeoRunners ships generic Terraform modules for running the controller in any
customer AWS account. They are variable-driven building blocks. A customer (or
an integrator) writes a small composition that supplies its own account,
region, VPC, subnets, load balancer, certificate, tags, repository scope, and
secret references.

**Customer tenant compositions are out of scope for this repository.** Real
ARNs, account IDs, hostnames, DNS, approval workflow, and cost-center tags
belong in a separate infrastructure repository that consumes these modules by
immutable Git source. Nothing in LeoRunners is specific to one company, DNS
provider, CI system, or add-on service.

## Modules

| Module | Path | Purpose |
|---|---|---|
| `aws-ecs-controller` | `terraform/modules/aws-ecs-controller` | ECR, logs, DynamoDB state, secret containers, roles, security group, target group, listener rule, Fargate service, in two gated phases |
| `aws-shared-gateway` | `terraform/modules/aws-shared-gateway` | adopts an existing ALB and certificate; optional HTTPS listener with a fixed default response, optional HTTP redirect, ALB security-group ingress |
| `aws-ephemeral-runner` | `terraform/modules/aws-ephemeral-runner` | contract only (no resources): launch template, IMDSv2, encrypted gp3, ownership tags, lifetime, PassRole scope, request-scoped cleanup |

`aws-ecs-controller` reuses `terraform/modules/controller-state` for the
DynamoDB table so the state schema has one definition. Consume the modules
through a Git source that includes the whole repository package (the
`//terraform/modules/...` form below) so the relative sibling reference
resolves.

A worked composition with placeholder values is in
`terraform/examples/aws-customer`.

## Phases

```text
Phase 0  inert (default)       no AWS resources (a state-only guard object
                               carries the plan-time checks)
Phase 1  create_foundation     ECR, logs, state table, empty secret containers,
                               roles, security group, target group; no service,
                               no listener rule
Phase 2  enable_listener_rule  route + Fargate service; refused until the
         enable_service        image digest, populated secrets, reviewed
                               runtime policy, and ALB inputs all exist
```

Each phase is one reviewed change. A plan that tries to skip ahead fails with
a precondition message before any AWS call.

## Variable contracts

Each module's `variables.tf` is the authoritative contract and each README
lists required inputs, defaults that matter, and outputs.

- Required explicit choices with no default: `egress_https_cidr_blocks`
  (controller), `https_ingress_cidr_blocks` (gateway), `listener_path_patterns`
  and `listener_priority` (route). Nothing is hardcoded, including the webhook
  path.
- Secrets are Secrets Manager ARN references only. A credential-looking key in
  `extra_environment` is rejected.
- Non-secret example inputs: `variables.example.tfvars` in each module and
  `terraform/examples/aws-customer/terraform.tfvars.example`.

Route only the webhook path to the controller. The same process also serves
`/healthz` and `/metrics` on its port; keep `listener_path_patterns` exact so
neither is reachable through the public listener.

## What the modules do not assume

A DNS provider, CloudFront, Elastic IPs, NAT gateways, VPC or subnet names, a
particular ALB, a particular infrastructure repository, GitHub-only use, or any
other service sharing the load balancer. Priorities for other services sharing
the ALB are a convention you publish through `listener_rule_priority_ranges`.

## Pinning and release tags

Reference modules by an immutable tag, ideally with the commit it points to
recorded in your change request. Tags for these modules use the form
`tf-aws-vMAJOR.MINOR.PATCH`.

```hcl
module "controller" {
  source = "git::https://github.com/manish96170/leoRunners.git//terraform/modules/aws-ecs-controller?ref=tf-aws-v0.1.0"
  # ...
}
```

A tag can be moved by whoever controls the repository. For a stronger pin,
use the full commit SHA in `ref=`.

## Offline verification

```sh
terraform/validate-aws-modules.sh          # fmt, validate, and mocked-provider tests
terraform/validate-aws-modules.sh --skip-init   # static checks only
```

The tests use a mocked AWS provider: no credentials, no AWS API calls.
`terraform init` downloads the provider plugin from the public registry.

## Replacing local module copies

1. Change each `source` to the Git form above with a pinned `ref`.
2. `terraform init -upgrade`, `terraform validate`, then `terraform plan` with your reviewed tfvars.
3. If the old local module used different resource addresses or names, add `moved` blocks in the composition so Terraform renames state instead of replacing resources. The resource addresses inside `aws-ecs-controller` are `aws_ecr_repository.controller`, `aws_cloudwatch_log_group.controller`, `module.state[0].aws_dynamodb_table.controller_state`, `aws_secretsmanager_secret.webhook`, `aws_secretsmanager_secret.github_token`, `aws_iam_role.execution`, `aws_iam_role.task`, `aws_iam_role_policy.task_runtime`, `aws_security_group.task`, `aws_vpc_security_group_ingress_rule.from_alb`, `aws_vpc_security_group_egress_rule.https_cidr["<cidr>"]`, `aws_lb_target_group.controller`, `aws_lb_listener_rule.controller`, `aws_ecs_task_definition.controller`, and `aws_ecs_service.controller`.
4. Name and description arguments that force replacement can be pinned to the existing values with `execution_role_name`, `task_role_name`, `task_security_group_description`, `state_table_name`, and `default_response_message`.
5. Do not apply until the plan shows no replacement or destroy of the load balancer, certificate, or DNS association. These modules never manage them.

## Related documents

- [Image publication contract](aws-image-publication.md)
- [Generated IAM policy workflow](aws-iam-policy-workflow.md)
- [State and secret handling](aws-state-and-secrets.md)
- [Rollback and request-scoped cleanup](aws-rollback-and-cleanup.md)
- [GitHub runtime contract](github-runtime-contract.md)
