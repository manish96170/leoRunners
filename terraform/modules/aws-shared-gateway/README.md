# aws-shared-gateway

Adopts an existing Application Load Balancer and certificate by ARN and adds a
safe HTTPS front door: an optional HTTPS listener whose default action is a
fixed response (404 unless changed), an optional HTTP-to-HTTPS redirect, and
the matching ALB security-group ingress rules. Services (such as
`aws-ecs-controller`) attach their own listener rules to the listener it
publishes.

The module has no ALB, certificate, or DNS resource. It cannot create, modify,
or replace the load balancer, and it assumes no hostname, route, DNS provider,
CloudFront distribution, Elastic IP, or NAT gateway.

## Behavior

Disabled by default: with no flags the module creates no AWS resources (it keeps a state-only `terraform_data` guard that carries the plan-time checks).

| Input | Effect |
|---|---|
| `enable_https_listener = true` | creates the HTTPS listener with the fixed default response; requires `certificate_arn` |
| `existing_https_listener_arn` | adopt a listener that already exists instead of creating one (mutually exclusive with the above) |
| `enable_http_redirect = true` | permanent redirect listener to HTTPS |
| `manage_security_group_ingress` (default true) | manages ALB security-group ingress; set false when another stack owns that security group |
| `additional_certificate_arns` | extra SNI certificates on the created listener |
| `listener_rule_priority_ranges` | validated, non-overlapping priority convention |

## Required inputs

| Input | Notes |
|---|---|
| `alb_arn` | ARN of the existing Application Load Balancer |
| `certificate_arn` | required when `enable_https_listener = true` |
| `alb_security_group_id` | required when security-group rules are managed |
| `https_ingress_cidr_blocks` and/or `https_ingress_prefix_list_ids` | no default; choose who may reach the listener |

## Listener-rule conventions

Publish one priority range per service class and let consumers pick values
inside their range:

```hcl
listener_rule_priority_ranges = {
  controller = { start = 100, end = 199 }
  other      = { start = 200, end = 299 }
}
```

The module validates the ranges (1-50000, no overlap) and republishes them as
the `listener_rule_priority_ranges` output. It does not create rules.

## Outputs

| Output | Meaning |
|---|---|
| `https_listener_arn` | listener to attach rules to (created or adopted), or null |
| `http_redirect_listener_arn` | redirect listener, or null |
| `listener_rule_priority_ranges` | the validated convention |
| `alb_arn` | echo of the adopted ALB |

## Example

```hcl
module "gateway" {
  source = "git::https://github.com/manish96170/leoRunners.git//terraform/modules/aws-shared-gateway?ref=tf-aws-v0.1.0"

  alb_arn                   = var.alb_arn
  alb_security_group_id     = var.alb_security_group_id
  certificate_arn           = var.certificate_arn
  enable_https_listener     = true
  https_ingress_cidr_blocks = var.https_ingress_cidr_blocks
  tags                      = var.tags
}
```

Offline tests: `terraform init -backend=false && terraform test` (mocked AWS provider).

## Review notes

- An existing listener on the same port causes a `DuplicateListener` error on create. Adopt it with `existing_https_listener_arn` instead.
- Removing this module removes only the listener, certificate attachments, and ingress rules it created. Rules other stacks added to the listener must be removed first, otherwise the listener delete fails.
