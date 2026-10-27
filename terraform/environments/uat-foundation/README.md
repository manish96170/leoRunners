# LeoRunners UAT Foundation

This composition creates only the isolated UAT foundation: an immutable ECR
repository, ECS cluster, CloudWatch log group, DynamoDB state table, dedicated
execution/controller/runner identities, and ingress-free controller/runner
security groups. It deliberately does **not** create a CloudFront distribution,
ALB, ECS service/task, Secrets Manager secret, GitHub configuration, runner AMI,
launch template, or EC2 runner.

## Planned webhook service layer

The later UAT webhook layer is part of this same foundation design and will add
a dedicated CloudFront distribution, ALB, target group, ECS Fargate service,
and Secrets Manager references. It remains disabled until review approves an
ALB origin hostname and an ACM certificate for HTTPS from CloudFront to the
ALB. CloudFront's `cloudfront.net` certificate is sufficient for GitHub's
viewer connection, but it does not provide an ALB origin certificate. Do not
substitute CloudFront-to-ALB HTTP for the approved end-to-end TLS path.

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
