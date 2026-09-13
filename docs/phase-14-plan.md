# Phase 14 Plan

Phase 14 makes the shared-state design deployable through reusable Terraform modules. The controller-state module creates the DynamoDB table and sparse `gsi1`/`gsi2` indexes used by the Go adapter, enables `expires_at` TTL, optional customer-managed encryption, point-in-time recovery, deletion protection, and on-demand billing.

The controller IAM module creates only the runtime role/trust boundary and accepts a separately generated and reviewed policy. Run `uvx iam-policy-autopilot@latest generate-policies` against the runtime Go sources with `--tf-dir` before supplying a policy. Review all wildcard resources, replace them with the real table/index/Launch Template/runner-role/KMS ARNs, then run IAM simulation and Access Analyzer.

The runner-runtime module creates an EC2 instance profile with no application permissions by default. SSM access is opt-in. Runner and controller roles remain separate, and untrusted fork jobs must not receive privileged credentials.

## Deployment gates

1. Run Terraform formatting and validation with a compatible AWS provider plugin.
2. Review the plan without printing or committing plan/state contents.
3. Apply only in a non-production account first.
4. Confirm DynamoDB writes, conditional conflicts, expiry queries, and event idempotency.
5. Run two controller replicas with fencing-token takeover and forced restart.
6. Preserve a file-state snapshot and rehearse rollback before production use.
