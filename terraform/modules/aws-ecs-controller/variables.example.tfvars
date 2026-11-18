# Non-secret example inputs for aws-ecs-controller. Every value is a
# placeholder. Copy to an owner-only location outside Git and replace them.

name_prefix = "leo-runners"

# Phase 1: foundation only. Nothing runs.
create_foundation = true
create_cluster    = true

vpc_id                = "vpc-0123456789abcdef0"
alb_security_group_id = "sg-0123456789abcdef0"

# Choose explicitly. A NAT gateway or VPC endpoint range is typical.
egress_https_cidr_blocks = ["10.0.0.0/8"]

tags = {
  Environment = "example"
  Owner       = "example-team"
  RequestId   = "example-request-1"
}

# Phase 2: leave disabled until the image, secrets, and policy are approved.
enable_listener_rule = false
enable_service       = false

# listener_arn           = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
# listener_path_patterns = ["/webhooks/github"]
# listener_priority      = 100
# subnet_ids             = ["subnet-0123456789abcdef0", "subnet-0fedcba9876543210"]
# image_digest           = "sha256:<64 hex characters from the published image>"
# secrets_ready          = true
# task_role_policy_json  = file("/secure/path/reviewed-controller-policy.json")

# GitHub JIT stays off until the token contract in docs/github-runtime-contract.md is satisfied.
github = {
  jit_enabled = false
}
