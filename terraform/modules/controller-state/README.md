# Controller State Module

This reusable Terraform module creates the shared durable state table used by
the Go controller. It creates one DynamoDB table and its data model controls;
it does not create application IAM policies, runner roles, or cross-account
infrastructure.

The table uses string `pk`/`sk` keys and two sparse GSIs matching
`controller/internal/state/dynamodb`:

| Index | Keys | Purpose |
| --- | --- | --- |
| `gsi1` | `gsi1pk` / `gsi1sk` | state and lifecycle-event queries |
| `gsi2` | `gsi2pk` / `gsi2sk` | expiry/reaper queries |

The controller writes index attributes only on records participating in the
corresponding access pattern, so the GSIs remain sparse. Both use `ALL`
projection because the repository reads complete records from index results.

## Intended inputs

- a stable table name;
- an optional customer-managed KMS key ARN;
- deletion protection and point-in-time recovery settings;
- tags identifying owner, data classification, environment, and managed-by.

The table uses on-demand billing and explicit GSI key attributes. Do not put
secrets in variables, tfvars, plans, state, tags, item attributes, or outputs.

## Usage

```hcl
module "controller_state" {
  source = "../../modules/controller-state"

  table_name = "leo-runners-controller-state"
  kms_key_arn = var.controller_state_kms_key_arn

  tags = {
    owner       = "platform-team"
    environment = "production"
  }
}
```

The controller must use the output `table_name`. The runtime IAM role is
managed separately and should be granted only the table and index ARNs from
`table_arn`, `state_index_arn`, and `expiry_index_arn`.

## Validation before apply

Run the following from this directory:

```text
terraform fmt -check -recursive
terraform init -backend=false
terraform validate
terraform plan -var-file=your-reviewed.tfvars
```

Review the plan for exactly one table, two GSIs, `PAY_PER_REQUEST`, TTL on
`expires_at`, point-in-time recovery, encryption, and deletion protection. Use
`terraform show -json` only in a controlled environment because plans can
contain sensitive values. A plan does not replace an IAM policy review,
Access Analyzer check, migration rehearsal, or backup restore exercise.

## IAM boundary

The runtime controller role should receive only the data-plane permissions it
needs on the named table and indexes:

- `dynamodb:GetItem`, `PutItem`, `UpdateItem`, `DeleteItem`;
- `dynamodb:Query` on the table and only the configured index ARNs;
- `dynamodb:TransactWriteItems` only if the adapter actually uses it;
- `dynamodb:DescribeTable` only if startup validation requires it;
- `kms:Decrypt` and `kms:GenerateDataKey` only for the configured table KMS
  key, when a customer-managed key is used.

Do not grant the runtime role `dynamodb:Scan`, table creation/deletion,
`UpdateTable`, backup/restore administration, KMS key administration,
`iam:PassRole`, or wildcard DynamoDB resources by default. A separate
deployment role owns Terraform and schema changes. A separate break-glass role
owns restore and recovery operations, with approval and audit logging.

Resource scoping must include the table ARN and each index ARN. If a policy
uses tag conditions or `dynamodb:LeadingKeys`, validate that the condition key
is present and cannot be bypassed by an empty or missing request value.

## State, retention, and backups

- Enable TTL on `expiresAt`, while treating TTL as eventual cleanup only.
- Keep controller reaping and provider termination independent of TTL.
- Enable point-in-time recovery and scheduled backups according to the
  environment retention policy.
- Encrypt the table and backups. Restrict KMS key administration to the
  security/infrastructure role; permit the controller only data-key use.
- Enable deletion protection in production and require an explicit recovery
  procedure before disabling it.
- Keep migration snapshots in encrypted, access-controlled storage and delete
  them only after the documented rollback window.

## Migration contract

The module does not migrate data. Before applying it:

1. Snapshot the file repository with owner-only permissions.
2. Validate and import records through a dedicated, auditable migration tool.
3. Use conditional writes so reruns cannot overwrite newer records.
4. Compare counts, IDs, revisions, expiry timestamps, event keys, and
   job/runner/lease relationships.
5. Run a read-only reconciliation and preserve the snapshot until sign-off.

The first rollout should be one Region and one active writer. Add standby
replicas only after fencing, conditional conflicts, and restart recovery have
been tested. Do not configure active-active global-table writes as a shortcut
for controller coordination.

## Validation before apply

This module intentionally does not author application IAM policies. The
deployment role owns the table, while a separately reviewed runtime role owns
data-plane access. Do not add `iam_role`, `aws_iam_policy`, or
`aws_iam_role_policy` resources here as a shortcut.

References: [DynamoDB best practices](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/best-practices.html),
[DynamoDB backups](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/BackupRestore.html),
and [IAM pass-role guidance](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_passrole.html).
