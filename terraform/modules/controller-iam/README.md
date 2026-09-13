# Controller IAM Module

This module creates the IAM role assumed by the ephemeral CI controller. It
provides trust-policy scaffolding and attachment points for a reviewed runtime
policy. It intentionally does **not** derive, guess, or broaden AWS
permissions.

## Trust boundary

Supply at least one explicit runtime principal through
`trusted_service_principals`, `trusted_principal_arns`, or the OIDC inputs.
Examples include an EC2 controller instance profile, an EKS web-identity
service account, or a separately managed workload identity. The module has no
default trust principal and cannot create an open trust policy.

For OIDC, provide the provider ARN, issuer condition prefix, and exact
`system:serviceaccount:<namespace>:<service-account>` subject values. The
module does not accept a wildcard subject. Keep trust and permissions policies
separate.

## Runtime permissions

`generated_policy_json` and `generated_policy_arns` are deliberately empty by
default. Before supplying either, run the approved `iam-policy-autopilot`
workflow against the actual Go controller runtime source, review its output,
and record the resulting policy as a generated artifact owned by the security
review process. Do not hand-write a replacement policy in this module.

The generated policy must scope `iam:PassRole` to the exact runner instance
role ARN and constrain it with `iam:PassedToService` (and any applicable
resource conditions). It must scope `ec2:RunInstances` to the approved Launch
Template, image, subnet, security group, and ownership/tag contract, and keep
`ec2:DescribeInstances`/`ec2:TerminateInstances` limited to the managed runner
resource boundary where AWS supports those resource conditions. Never pair
`iam:PassRole` or `ec2:RunInstances` with an unrestricted resource as a
shortcut.

The controller-state policy is a separate concern. If the controller uses
DynamoDB, generate and review the data-plane permissions against the actual
adapter and table/index ARNs; do not merge administrative table, IAM, or
runner permissions into a broad managed policy.

## Policy generation reference

From the repository root, a security-controlled operator can generate a
candidate policy with the current Autopilot tool. Use absolute paths and
include every Go file that makes runtime AWS SDK calls; do not upload or apply
the output automatically:

```text
uvx iam-policy-autopilot@latest generate-policies \
  /absolute/path/to/controller/internal/providers/aws/provider.go \
  /absolute/path/to/controller/internal/state/dynamodb/repository.go \
  --tf-dir /absolute/path/to/terraform \
  --service-hints ec2 dynamodb iam \
  --pretty
```

The command is a reference only. Confirm Autopilot availability and supported
flags in the target environment, inspect the generated policy, and separately
review resource ARNs and conditions before passing it to this module.

## Example

```hcl
module "controller_iam" {
  source = "./terraform/modules/controller-iam"

  role_name                 = "leo-controller-prod"
  trusted_service_principals = ["ec2.amazonaws.com"]
  generated_policy_arns     = [aws_iam_policy.controller_runtime.arn]

  tags = {
    managed-by  = "terraform"
    component   = "controller"
    environment = "prod"
  }
}
```

The example assumes `aws_iam_policy.controller_runtime` is generated and
reviewed elsewhere. This module does not create that policy.

Before attaching a generated policy, run the offline contract checks from the
repository root:

```text
./tools/iam-validation/validate.sh /path/to/generated-policy.json /path/to/reviewed-contract.json
```

The checker is review-only. A passing result does not upload, simulate through
AWS, apply Terraform, or replace human IAM review. Keep the policy and its
contract together as review evidence, and reject the change if either file is
modified after validation.

## Validation

Run `./validate.sh` from this directory. If Terraform is installed, the
script also runs `terraform fmt -check`; initialization and apply are left to
an operator after policy review. Use a plan review, IAM Access Analyzer, and
policy simulation before production use. Never commit credentials, Terraform
plans, generated policy secrets, or provider state.
