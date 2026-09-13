# Security

Security is deterministic and independent of AI. The controller uses a GitHub App or equivalent short-lived GitHub credentials. JIT configuration is generated just before bootstrap; tokens and credentials are never baked into AMIs or persisted in logs.

Runner instances receive the minimum permissions needed by the selected workload. Arbitrary fork pull requests must not receive privileged cloud credentials. OIDC is preferred for workload-specific cloud access. Runner IAM, controller IAM, security groups, network egress, and secret access are separate policies.

AWS instances require IMDSv2, restrictive security groups, encrypted storage, ownership tags, and a hard expiry. Docker-in-Docker and privileged containers require an explicit security profile. Webhooks require HMAC verification, replay protection, and idempotency.

Open decisions: fork policy, GitHub App scope, secret manager, public versus private subnet, NAT/VPC endpoints, and whether untrusted jobs use isolated accounts or only isolated instances.
