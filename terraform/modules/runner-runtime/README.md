# Runner Runtime IAM Module

This module creates the minimal identity an ephemeral EC2 runner needs to start:

- one IAM role trusted only by the EC2 service principal;
- one instance profile for the runner launch template;
- no inline application policy and no default permissions; and
- an optional, explicitly gated attachment of `AmazonSSMManagedInstanceCore`.

The default role cannot read repositories, access customer services, assume other
roles, modify infrastructure, or call arbitrary AWS APIs. Add workload access
through a separately reviewed identity design. This module intentionally does not
generate broad application policies.

## Usage

```hcl
module "runner_runtime" {
  source = "./terraform/modules/runner-runtime"

  name = "leo-runner-runtime"

  # Leave disabled unless operational SSM access has been approved.
  enable_ssm = false

  tags = {
    environment = "staging"
  }
}
```

Use the `instance_profile_name` output in the EC2 Launch Template. Keep the
controller role that calls `ec2:RunInstances` separate from this runner role, and
scope `iam:PassRole` in the controller policy to this exact role ARN.

## SSM boundary

`enable_ssm` defaults to `false`. When enabled, the module attaches the AWS
managed `AmazonSSMManagedInstanceCore` policy so operators can use Systems
Manager without SSH. Review that policy and the account's SSM endpoint/network
controls before enabling it. No SSM permissions are created by default.

## OIDC and workload credentials

Do not put GitHub tokens, JIT configuration, cloud access keys, or customer
secrets in this role, user data, the AMI, or Terraform state. GitHub Actions OIDC
should be configured as a separate trust relationship for a separately named
workload role, with `aud` and repository/branch or environment conditions scoped
to the intended workflow. The ephemeral runner should obtain only the short-lived
credential required for its approved workload, preferably through an external
identity exchange or a narrowly scoped role assumption designed and reviewed for
that workload.

This EC2 trust policy is deliberately not an OIDC trust policy. Mixing GitHub OIDC
and the instance-profile trust would allow unrelated workloads to obtain the
runner identity.

## Untrusted fork isolation

Treat pull requests from forks as untrusted code. Do not place customer-owned
secrets, privileged cloud credentials, production network access, or an approved
OIDC trust path on a runner that can execute untrusted fork jobs. Use separate
capacity pools, subnets/security groups, accounts, AMIs, and IAM roles for
untrusted workloads, or deny those jobs. Approval and workflow policy must happen
before provisioning; this module does not infer trust from repository labels.

## Validation

From the repository root, run Terraform formatting and validation in a root module
that supplies the AWS provider and this module. Review the planned policy and trust
documents before applying:

```bash
terraform fmt -check -recursive terraform/modules/runner-runtime
terraform init
terraform validate
terraform plan
```

The module does not create customer workload policies, networking, security groups,
or the controller role.
