# UAT Shared Infrastructure Gateway

## Decision

The existing GitNexus MCP deployment remains independently owned and must not
be reused, modified, repaired, migrated, or made a dependency of LeoRunners.
It has a separate CloudFront/ALB/ECS/EFS lifecycle and current health issues.

New UAT services use a clean, dedicated shared gateway under:

```text
https://infrastructure.getanchor.io
```

The initial gateway service is LeoRunners' GitHub webhook controller. A clean,
separately owned GNGraph/GitNexus-compatible MCP service may be added later;
the existing GitNexus MCP endpoint is not moved by this work.

## Target architecture

```text
Internet
  |
  | HTTPS :443
  v
infrastructure.getanchor.io
  |
  v
Dedicated UAT Application Load Balancer
  |
  +-- POST /webhooks/github
  |     -> dedicated LeoRunners target group
  |     -> dedicated ECS Fargate controller service
  |
  +-- POST /gitnexus/mcp                    (future, separate service)
  |     -> dedicated GNGraph target group/service
  |
  +-- /agents/<agent-name>/mcp              (future, separate service)
        -> dedicated target group/service per agent
```

Use one dedicated UAT ECS cluster only as a shared scheduling boundary. Every
service remains separately owned and deployable through its own ECS service,
task-definition family, task/execution roles, ECR repository, security group,
target group, secrets, logs, alarms, persistent storage, and ownership tags.

## Explicitly excluded from reuse

Do not reuse from the existing GitNexus deployment:

- CloudFront distribution, ALB, listener, target group, or security groups;
- ECS service, task definition, ECR repository, EFS, or access point;
- execution/task/scheduler roles, bearer token, GitHub token, or GitLab token;
- log group, deployment workflow, Terraform state, or lifecycle ownership.

Do not use Commerce GitLab runner manager/worker infrastructure, its Auto
Scaling Groups, launch templates, instance profiles, network controls, tags,
or Terraform state.

## DNS and TLS

CloudFront and an Elastic IP are not part of the initial design.

The DNS provider should eventually create:

```text
infrastructure.getanchor.io CNAME <dedicated UAT ALB DNS name>
```

The dedicated ALB uses an ACM public certificate in `us-west-2` for
`infrastructure.getanchor.io`. ACM DNS validation must complete before the
HTTPS listener is enabled. If restrictive CAA records exist, they must allow an
Amazon ACM certificate authority. No Let’s Encrypt/Caddy configuration is
required for this design.

The ALB must redirect HTTP to HTTPS or omit the HTTP listener. Public routes
must not use plaintext HTTP.

### TLS boundary

The UAT baseline requires TLS from the public client to the ALB. The ALB may
use HTTP to a Fargate task only across the VPC-private ALB-to-task path, with a
dedicated task security group that permits ingress only from the shared ALB
security group. This is not cryptographic end-to-end TLS and must not be
described as such. If a future compliance boundary requires encryption between
the ALB and container, the service must support HTTPS itself or run a dedicated
TLS sidecar and use an HTTPS target group.

## Route-specific authentication

| Route | Authentication |
| --- | --- |
| `/webhooks/github` | GitHub `X-Hub-Signature-256` HMAC plus replay/idempotency controls, using a dedicated LeoRunners secret |
| `/gitnexus/mcp` | Dedicated MCP bearer token initially; do not inherit LeoRunners or existing GitNexus credentials |
| `/agents/*/mcp` | Dedicated per-service token/scopes; do not inherit another MCP service's access |
| health endpoints | ALB target checks or separately protected operations access; do not expose by default |

## UAT service rollout

1. Apply the reviewed LeoRunners foundation only: ECR, ECS cluster, DynamoDB,
   roles, logs, and ingress-free security groups.
2. Request ACM certificate and DNS validation for `infrastructure.getanchor.io`.
3. Add the dedicated ALB, HTTPS listener, LeoRunners target group, ECS service,
   and `/webhooks/github` route after separate plan review.
4. Validate GitHub webhook HMAC, state, cleanup, and no-runner behavior before
   authorizing EC2 runner lifecycle permissions.
5. Add a clean GNGraph service and `/gitnexus/mcp` only as a separate,
   independently reviewed deployment. Verify its base-path contract or apply a
   reviewed ALB URL rewrite to preserve its backend endpoint.

No Dashboard Actions, GitHub runner group, runner AMI, EC2 runner, MCP client
cutover, tag, release, stage, or production deployment is part of this gateway
decision.

## Future GNGraph service requirements

The clean UAT GNGraph service starts with no import of the existing GitNexus
EFS data. It requires its own encrypted EFS file system and access point,
because Fargate task replacement without persistent clone/index storage causes
full repository re-analysis and high startup cost.

The public route is additive:

```text
POST /gitnexus/mcp
```

The current GNGraph backend contract is expected to use `/mcp`. Before a client
cutover, a dedicated UAT GNGraph task must prove Streamable HTTP initialization,
session ID handling, follow-up `tools/list`, long-running queries, reconnect
behavior, and any required SSE behavior. Only then may a reviewed ALB rewrite
map `/gitnexus/mcp` to `/mcp`. The existing GitNexus URL remains unchanged
until this additive compatibility test succeeds.

The GNGraph target health check must represent MCP readiness rather than process
startup. Initial indexing/re-analysis must finish, or explicitly report not
ready, before the ALB serves MCP traffic; a process that is running but still
rebuilding indexes must not produce a persistent public 502.

GNGraph and LeoRunners retain separate EFS/EFS access points, tokens, IAM
roles, ECR repositories, security groups, target groups, logs, alarms, storage,
and deployment roles.
