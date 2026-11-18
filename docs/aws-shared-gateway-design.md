# Shared Gateway Design

A single HTTPS Application Load Balancer can front several independently owned
services. This note describes the pattern the `aws-shared-gateway` module
supports. It makes no assumption about the DNS provider, the hostname, or what
other services share the load balancer.

## Target shape

```text
Internet
  |  HTTPS :443
  v
<your hostname>
  |
  v
Application Load Balancer (created and owned outside LeoRunners)
  |
  +-- <webhook path>      -> controller target group -> ECS Fargate controller service
  +-- <other service path> -> that service's own target group and service
```

The load balancer, its DNS record, and its certificate are bootstrapped by
whoever owns them and are adopted by ARN. The modules add only a listener,
listener rules, and the security-group rules they declare. They never create,
modify, or replace the load balancer, certificate, or DNS association.

## Isolation model

The shared ALB and an optional ECS cluster are scheduling and routing
infrastructure only. Each service keeps its own task definition, target group,
task and execution roles, security group, image repository, log group, secrets,
alarms, deployment role, and ownership tags. LeoRunners must not reuse another
service's CloudFront distribution, ALB, ECS service, image repository, storage,
roles, secrets, logs, or Terraform state.

Publish a listener-rule priority range per service class through
`aws-shared-gateway`'s `listener_rule_priority_ranges` so services pick
non-overlapping priorities.

## DNS and TLS

- Point your DNS name at the ALB with a record your DNS provider supports.
- Use an issued public certificate in the ALB's region and complete certificate validation before enabling the HTTPS listener. If restrictive CAA records exist, they must allow your certificate authority.
- Public routes must use HTTPS. Redirect HTTP to HTTPS or omit the HTTP listener (`enable_http_redirect`).
- CloudFront and Elastic IPs are not required.

### TLS boundary

TLS is required from the public client to the ALB. The ALB may use HTTP to a
Fargate task across the VPC-private ALB-to-task path, with a dedicated task
security group that permits ingress only from the ALB security group. This is
not end-to-end TLS and must not be described as such. If a compliance boundary
requires encryption between the ALB and the container, the service must speak
HTTPS itself or run a TLS sidecar behind an HTTPS target group.

## Route-specific authentication

| Route | Authentication |
|---|---|
| GitHub webhook path | GitHub `X-Hub-Signature-256` HMAC plus replay and idempotency controls, using a dedicated secret |
| Any other service | its own credentials and scopes; never inherit another service's access |
| Health endpoints | ALB target checks only; do not expose them through public listener rules |

Route only the exact paths a service needs. The controller process also serves
`/healthz` and `/metrics`, so use exact `listener_path_patterns`.

## Rollout order

1. Apply the foundation: image repository, cluster, state, roles, logs, and ingress-free security groups.
2. Add the HTTPS listener (fixed response) through `aws-shared-gateway`.
3. Add the controller's target group, listener rule, and service through `aws-ecs-controller`, after separate plan review.
4. Validate webhook signature verification, state, cleanup, and no-runner behavior before authorizing EC2 runner lifecycle permissions.

No GitHub Actions enablement, runner group, runner AMI, EC2 runner, tag, release,
or production deployment is part of this design.
