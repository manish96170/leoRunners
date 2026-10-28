# LeoRunners UAT Foundation

This composition creates only the isolated UAT foundation: an immutable ECR
repository, ECS cluster, CloudWatch log group, DynamoDB state table, dedicated
execution/controller/runner identities, and ingress-free controller/runner
security groups. It deliberately does **not** create a CloudFront distribution,
ECS service/task, Secrets Manager secret, GitHub configuration, runner AMI,
launch template, or EC2 runner. It can adopt, but never recreate, the approved
externally bootstrapped UAT ALB and certificate.

## Planned webhook service layer

The approved gateway direction is a direct HTTPS ALB at
`infrastructure.getanchor.io`, without CloudFront or an Elastic IP. The
externally bootstrapped ALB and ACM certificate are adopted only when
`enable_https_gateway=true`. The gateway layer creates a fixed-404 HTTPS
listener and public TCP/443 ingress; it does not make an application route
reachable. Later service plans add a dedicated target group, restricted task
security group, service, Secrets Manager references, and listener-rule path.

With the issued UAT certificate and ALB identifiers supplied through an
owner-only tfvars file, the gateway review plan contains 14 additions and no
changes or destroys. The two gateway-specific additions are the port-443
security-group rule and the fixed-404 HTTPS listener. Applying this plan is a
separate approval and must use a real request ID and bounded cleanup deadline,
not the example values.

The AWS foundation is generic. Repository-specific runner labels, GitHub runner
groups, and workflow changes belong in an approved tenant composition and must
not be applied by this directory.

## Review-only plan

Copy `uat.tfvars.example` to an owner-only location outside this repository and
supply only approved existing network IDs. Do not use Dashboard, production, or
Commerce GitLab runner Terraform state.

~~~sh
cd terraform/environments/uat-foundation
terraform init -backend=false
terraform fmt -check -recursive
terraform validate
terraform plan -refresh=false -var-file=/secure/path/leo-runners-uat.tfvars
~~~

Before an apply, generate the controller policy from the actual Go runtime
source. The role intentionally has no runtime permissions until that reviewed
output is supplied:

~~~sh
uvx iam-policy-autopilot@latest generate-policies \
  /absolute/path/controller/internal/providers/aws/provider.go \
  /absolute/path/controller/internal/state/dynamodb/repository.go \
  --tf-dir /absolute/path/terraform/environments/uat-foundation \
  --region us-west-2 \
  --account 307533710979 \
  --service-hints ec2 dynamodb iam \
  --pretty
~~~

Do not upload the generated policy or run `terraform apply` without separate
review and explicit approval. The later webhook/controller plan must additionally
prove HTTPS ingress, GitHub HMAC verification, Secrets Manager scope, exact
request tags, and cleanup ownership.
