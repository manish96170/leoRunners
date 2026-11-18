# Development Terraform Composition

This root composes only the existing controller-state, controller-iam, and
runner-runtime modules. It creates no VPC, subnet, security group, AMI, launch
template, GitHub integration, or remote backend. Those are deliberate inputs or
separate deployment concerns.

## Optional observability

CloudWatch logs, notification-only alarms, and a dashboard are composed from
`../../modules/observability` only when `enable_observability = true`. The
default is `false`, so the normal development composition creates no
observability resources. Review retention, the fixed low-cardinality dimensions,
and any external notification ARNs before enabling it. This root creates no SNS
topic, IAM permission, remediation action, or lifecycle automation.

When disabled, the observability outputs are `null` or empty. When enabled, the
module uses the existing AWS provider and the common dev tags; it does not
create networking or change the required reviewed controller policy input.

## Required policy review

`controller_generated_policy_json` is required and has no default. Supply the
exact JSON output of the approved `iam-policy-autopilot` workflow after a human
review has scoped the real DynamoDB, EC2, Launch Template, runner-role, and KMS
ARNs. This composition does not generate, infer, upload, or approve IAM policy
permissions. Do not use a broad wildcard policy to make validation pass.

The policy must scope `iam:PassRole` to the runner role and constrain it to EC2;
it must also scope EC2 launch and DynamoDB access to the resources created or
explicitly supplied by the deployment. Run IAM simulation and Access Analyzer
before applying.

## Backend-disabled validation

The included `backend-disabled.tf.example` is documentation, not Terraform
configuration. Terraform's local backend is the default when no backend block
exists; use the CLI flag below to prevent backend initialization:

```bash
terraform init -backend=false
terraform validate
terraform plan -refresh=false -var-file=dev.tfvars
```

Copy `terraform.tfvars.example` to a private `dev.tfvars`, replace the absolute
reviewed-policy path, and provide the required names and trust inputs. Keep
`dev.tfvars`, policy JSON, `.terraform/`, state, and plan files out of version
control.

## Apply boundary

Applying this root creates the DynamoDB table and IAM roles. It does not create
networking or a usable EC2 runner by itself. The default SSM attachment is off,
and deletion protection and point-in-time recovery are on. Review the plan,
trust policy, generated policy, and account before `terraform apply`.

```bash
terraform fmt -check -recursive
terraform init -backend=false
terraform validate
terraform plan -out=dev.tfplan -var-file=dev.tfvars
```

To inspect the opt-in surface without applying it, set
`enable_observability = true` in a private tfvars file and run the same
backend-disabled validation and plan commands. Confirm that the plan contains
only the expected log group, alarms, and dashboard in addition to the existing
composition.

No credentials belong in Terraform files, user data, AMIs, or policy artifacts.
