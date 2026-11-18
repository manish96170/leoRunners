# Generated IAM Policy Workflow

Controller runtime permissions are generated from the controller source with
[IAM Policy Autopilot](https://github.com/awslabs/iam-policy-autopilot), then
reviewed and refined by a human. They are never hand-written broadly and never
attached automatically. `aws-ecs-controller` accepts the result as
`task_role_policy_json` (inline) or `task_role_policy_arns` (managed policies).

## Roles stay separate

| Role | Created by | Holds |
|---|---|---|
| ECS task execution role | `aws-ecs-controller` | image pull, log writes, `GetSecretValue` on exactly the referenced secrets |
| Controller task role | `aws-ecs-controller` | only the reviewed runtime policy you supply |
| Runner instance role | the customer / future runner layer | what a CI job on a runner may do |
| Deployment role | the customer | what Terraform needs to apply (generate it separately) |
| Customer workload roles | the customer | never shared with the roles above |

## 1. Generate

Run from a checkout at the revision you are deploying. Include every Go file
that makes runtime AWS SDK calls; at this revision those are the AWS provider
and the DynamoDB repository.

```sh
uvx iam-policy-autopilot@latest generate-policies \
  controller/internal/providers/aws/provider.go \
  controller/internal/state/dynamodb/repository.go \
  --region <region> --account <account-id> \
  --service-hints ec2 dynamodb iam \
  --pretty > raw-policy.json
```

Add `--tf-dir <your composition>` to let Autopilot see the Terraform resources.
Confirm flags in your environment; the tool changes between versions. Do not
upload or attach the output.

Raw output at the current revision (illustrative) looks like this, and is
**not acceptable as is**:

- DynamoDB actions on `table/*`, including `ReadDataForReplication` and `WriteDataForReplication` that the controller does not need
- `ec2:CreateTags`, `ec2:DescribeInstances`, `ec2:RunInstances` on `*`
- `ec2:TerminateInstances` on `instance/*`
- `iam:PassRole` on `*` (limited only by `iam:PassedToService`)
- `kms:Decrypt` on `key/*` limited by `kms:ViaService`
- two `WildcardResource` warnings

## 2. Refine

Use the module outputs `policy_review_scope` (account, region, table and index
ARNs, runner role) and, once the runner layer is planned,
`aws-ephemeral-runner`'s `controller_policy_constraints`.

- **DynamoDB:** scope to the table ARN and the two index ARNs only. Remove replication actions.
- **`iam:PassRole`:** Resource is exactly the dedicated runner role ARN, with `iam:PassedToService = ec2.amazonaws.com`. No wildcard or role prefix.
- **`ec2:RunInstances`:** list the resource types it acts on (instance, volume, network interface, launch template, image, subnet, security group), restrict to the approved launch template, AMI, subnets and security groups, and require the ownership request tags (`Platform=ci-runner`, `ManagedBy=leo-runners`).
- **`ec2:CreateTags`:** only `ec2:CreateAction = RunInstances`.
- **`ec2:TerminateInstances`:** only resources carrying the ownership tags (and the request tag when you use one).
- **`ec2:DescribeInstances`:** does not support resource-level scoping; `*` is acceptable for this read-only action.
- **`kms:Decrypt`:** your customer-managed key ARN, if you use one, instead of `key/*`.

Runner lifecycle is off by default. Until you enable it, the policy needs only
the DynamoDB statement.

## 3. Verify offline

The module rejects the unacceptable shapes at plan time: wildcard `iam:PassRole`
resources, `Allow` with `NotAction`, `Action "*"`, and wildcard `iam:` actions.
When `runner.enabled = true` it also requires an `iam:PassRole` statement whose
Resource includes exactly `runner.role_arn`. These checks are a floor, not a
review.

Also run:

```sh
tools/iam-validation/validate.sh <policy.json> <reviewed-contract.json>
aws accessanalyzer validate-policy --policy-type IDENTITY_POLICY \
  --policy-document file://policy.json
```

and an IAM policy simulation against the real ARNs before the first apply.

## 4. Supply

```hcl
task_role_policy_json = file("/secure/path/reviewed-controller-policy.json")
# or
task_role_policy_arns = ["arn:aws:iam::<account-id>:policy/<reviewed-controller-policy>"]
```

Keep the reviewed artifact in the customer's repository with its review
record, not in LeoRunners. `task_role_policy_arns` cannot be inspected by
Terraform, so runner lifecycle requires the inline form.

## Deployment role

Generate the Terraform deployment role's policy separately from a plan, and
keep plan artifacts owner-only and out of Git:

```sh
umask 077
terraform plan -out=plan.tfplan -var-file=/secure/path/vars.tfvars
terraform show -json plan.tfplan > plan.json
uvx iam-policy-autopilot@latest generate-policies plan.json --region <region> --account <account-id> --pretty
rm -f plan.tfplan plan.json
```
