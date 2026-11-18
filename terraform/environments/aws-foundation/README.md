# aws-foundation

An example environment composition that creates the isolated AWS foundation for
one LeoRunners deployment: an immutable ECR repository, ECS cluster,
CloudWatch log group, DynamoDB state table, dedicated execution, controller and
runner identities, and ingress-free controller and runner security groups.

It deliberately does **not** create a VPC, CloudFront distribution, ECS
service or task, Secrets Manager secret, GitHub configuration, runner AMI,
launch template, or EC2 runner. It can adopt, but never create or replace, an
existing Application Load Balancer and certificate.

Copy it into your own infrastructure repository, or use it as a reference. For
a gated controller service with listener rules, see
`../../modules/aws-ecs-controller` and `docs/aws-modules.md`. This composition
covers the foundation layer around them: shared identities, network rules, and
the HTTPS gateway.

## Required inputs

| Input | Purpose |
|---|---|
| `environment`, `owner`, `repository`, `runner_group` | tags only; nothing is created in GitHub |
| `controller_vpc_id`, `controller_vpc_cidr` | existing VPC for the controller; the CIDR is used for DNS egress |
| `runner_vpc_id`, `runner_vpc_cidr`, `runner_subnet_ids` | existing network for future ephemeral runners |

Everything else has a default. `terraform.tfvars.example` holds non-secret
placeholders.

## Optional HTTPS gateway

Set `enable_https_gateway = true` with `gateway_alb_arn`,
`gateway_alb_security_group_id`, `gateway_certificate_arn` and
`gateway_https_ingress_cidr_blocks` to add, through `../../modules/aws-shared-gateway`:

- an HTTPS listener whose default response is a fixed 404;
- ingress rules on the ALB security group for the CIDRs you list.

The ALB and certificate are never created, modified, or replaced. No application
route is reachable until a service adds its own target group and listener rule.
See `docs/aws-shared-gateway-design.md`.

## Review-only plan

```sh
cd terraform/environments/aws-foundation
terraform init -backend=false
terraform fmt -check -recursive
terraform validate
terraform test
terraform plan -refresh=false -var-file=/secure/path/example.tfvars
```

`terraform test` uses a mocked AWS provider: no credentials, no AWS calls.

## Before an apply

Generate the controller runtime policy from the actual Go source. The role has no
runtime permissions until a reviewed policy is supplied:

```sh
uvx iam-policy-autopilot@latest generate-policies \
  /absolute/path/controller/internal/providers/aws/provider.go \
  /absolute/path/controller/internal/state/dynamodb/repository.go \
  --tf-dir /absolute/path/terraform/environments/aws-foundation \
  --region <region> --account <account-id> \
  --service-hints ec2 dynamodb iam --pretty
```

Refine the output as described in `docs/aws-iam-policy-workflow.md`. Do not
upload the generated policy or run `terraform apply` without separate review
and explicit approval. Use a real request ID and bounded cleanup deadline
instead of the review-only defaults, so cleanup can select resources by exact
tags (see `docs/aws-rollback-and-cleanup.md`).

## Defaults worth reviewing

- Controller HTTPS egress defaults to `0.0.0.0/0` on TCP/443, because GitHub's address ranges are broad. Narrow `controller_https_egress_cidr_blocks` if you route through a NAT gateway or VPC endpoints.
- The state table keeps point-in-time recovery and deletion protection on. Disable deletion protection only for disposable environments.
- The runner security group is created in `nat` egress mode with no inbound access.
