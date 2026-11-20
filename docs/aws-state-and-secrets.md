# State and Secret Handling

## Terraform state

Terraform state records every attribute of every managed resource, including
`task_role_policy_json`. Treat it as sensitive:

- use a remote backend with encryption, versioning, access control, and locking;
- keep `*.tfstate`, `*.tfplan`, and populated `*.tfvars` out of Git (this repository's `.gitignore` already excludes them);
- write plan files and `terraform show -json` output with `umask 077` and delete them after review.

Secret values never enter Terraform: the modules create empty Secrets Manager
containers, take ARN references, and have no variable that accepts a secret
value. `extra_environment` rejects credential-looking keys.

## Controller state (DynamoDB)

`aws-ecs-controller` creates the state table with the repository's
`controller-state` module: on-demand billing, TTL on `ttl_at` (seven days after a record's expiry), sparse
lifecycle indexes (`gsi1` state/event, `gsi2` expiry/reaper), encryption at
rest, point-in-time recovery, and deletion protection (each toggleable).

It holds orchestration metadata: job and runner records, leases, expiry
timestamps, and event idempotency keys. The one-time GitHub JIT configuration
is transient bootstrap input and is not persisted. Use an existing table with
`existing_state_table` and `create_state_table = false`; it must follow the
`controller-state` schema.

## Secret containers

| Secret | Name | When created |
|---|---|---|
| GitHub webhook secret | `<name_prefix>/github-webhook-secret` | always with the foundation |
| GitHub App private key (PEM) | `<name_prefix>/github-app-private-key` | when `github.app` is set (GitHub App mode) |
| GitHub installation token | `<name_prefix>/github-installation-token` | static-token mode: when `github.jit_enabled` without `github.app`, or `create_github_token_secret` |

Populate them outside Terraform. Avoid putting values in shell history or
process arguments; use a file with restrictive permissions:

```sh
umask 077
openssl rand -hex 32 > webhook-secret.txt
aws secretsmanager put-secret-value \
  --secret-id "$(terraform output -raw webhook_secret_arn)" \
  --secret-string file://webhook-secret.txt
# Set the same value as the GitHub webhook secret, then delete the file.
rm -f webhook-secret.txt
```

Then set `secrets_ready = true` (your assertion that every referenced secret
holds an approved value) in the change that enables the service.

## Secret rotation

ECS injects secret values when a task starts and never refreshes a running
task. After changing a secret, roll the service:

```sh
aws ecs update-service --cluster <cluster> --service <service> --force-new-deployment
```

Rotate the webhook secret on both sides (GitHub and Secrets Manager) in the
same window.

In GitHub App mode the controller mints and refreshes its own installation
tokens from the private key, so token expiry needs no action. Rotating the
private key itself follows the steps in
[GitHub runtime contract](github-runtime-contract.md#rotating-the-private-key).
In static-token mode the installation token expires after about an hour and
must be re-issued and the service redeployed, so that mode suits short
validation runs only.

## Encryption keys

`secrets_kms_key_arn` encrypts the secrets with your customer-managed key and
grants the execution role `kms:Decrypt` on it. `state_kms_key_arn`,
`log_kms_key_arn`, and `ecr_kms_key_arn` do the same for their resources; the
key policies must allow the relevant service.

## Logs

Controller logs go to a dedicated CloudWatch log group with configurable
retention (default 30 days). The controller is built not to log tokens, JIT
configuration, or raw webhook bodies; confirm that against the revision you
deploy before extending retention.
