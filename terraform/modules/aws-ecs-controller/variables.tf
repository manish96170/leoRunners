###############################################################################
# Identity and phases
###############################################################################

variable "name_prefix" {
  description = "Prefix for every resource name created by this module (for example leo-runners-prod)."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,18}[a-z0-9]$", var.name_prefix))
    error_message = "name_prefix must be 3-20 characters of lowercase letters, digits, and hyphens, starting with a letter (the target group name limit is 32 characters)."
  }
}

variable "create_foundation" {
  description = "Phase 1. Create the ECR repository, log group, state table, secret containers, IAM roles, task security group, and target group. Never starts the controller."
  type        = bool
  default     = false
}

variable "enable_listener_rule" {
  description = "Phase 2. Create the ALB listener rule that forwards the configured paths to the controller target group. Requires create_foundation."
  type        = bool
  default     = false
}

variable "enable_service" {
  description = "Phase 2. Create the ECS task definition and Fargate service. Requires create_foundation, enable_listener_rule, an immutable image digest, populated secrets, and a reviewed runtime IAM policy."
  type        = bool
  default     = false
}

variable "tags" {
  description = "Tags applied to every taggable resource. Put ownership, request, and cleanup-deadline tags here."
  type        = map(string)
  default     = {}
}

variable "iam_path" {
  description = "IAM path for the roles created by this module."
  type        = string
  default     = "/"

  validation {
    condition     = can(regex("^/([A-Za-z0-9+=,.@_-]+/)*$", var.iam_path))
    error_message = "iam_path must start and end with / and contain only valid IAM path characters."
  }
}

variable "execution_role_name" {
  description = "Name of the ECS task execution role. Defaults to <name_prefix>-controller-execution. Set it to adopt an existing role name without replacement."
  type        = string
  default     = null
}

variable "task_role_name" {
  description = "Name of the controller task role. Defaults to <name_prefix>-controller."
  type        = string
  default     = null
}

variable "task_security_group_description" {
  description = "Description of the task security group. Changing it forces replacement, so set it to the existing value when adopting a security group."
  type        = string
  default     = "LeoRunners controller task: ALB-only ingress, explicit egress."
}

variable "permissions_boundary_arn" {
  description = "Optional IAM permissions boundary applied to both roles created by this module."
  type        = string
  default     = null

  validation {
    condition     = var.permissions_boundary_arn == null || can(regex("^arn:[^:]+:iam::[0-9]{12}:policy/.+$", var.permissions_boundary_arn))
    error_message = "permissions_boundary_arn must be an IAM policy ARN or null."
  }
}

###############################################################################
# Container image
###############################################################################

variable "create_ecr_repository" {
  description = "Create an ECR repository (immutable tags, scan on push, lifecycle policy) during the foundation phase."
  type        = bool
  default     = true
}

variable "ecr_kms_key_arn" {
  description = "Optional customer-managed KMS key ARN for ECR encryption. Null uses AES256."
  type        = string
  default     = null
}

variable "ecr_untagged_image_retention_count" {
  description = "Number of untagged images to keep before older ones expire."
  type        = number
  default     = 10

  validation {
    condition     = var.ecr_untagged_image_retention_count >= 1
    error_message = "ecr_untagged_image_retention_count must be at least 1."
  }
}

variable "ecr_max_image_count" {
  description = "Maximum number of images kept in the repository. Older images expire, so keep this above the number of digests you may roll back to."
  type        = number
  default     = 50

  validation {
    condition     = var.ecr_max_image_count >= 5
    error_message = "ecr_max_image_count must be at least 5."
  }
}

variable "image_repository" {
  description = "Image repository URI (without tag or digest). Required when create_ecr_repository is false and the service is enabled."
  type        = string
  default     = null
}

variable "image_digest" {
  description = "Immutable image digest (sha256:<64 hex>). Required when enable_service is true. Tags are not accepted."
  type        = string
  default     = null

  validation {
    condition     = var.image_digest == null || can(regex("^sha256:[a-f0-9]{64}$", var.image_digest))
    error_message = "image_digest must be an immutable digest of the form sha256:<64 lowercase hex characters>."
  }
}

###############################################################################
# Logging
###############################################################################

variable "log_retention_days" {
  description = "CloudWatch Logs retention for the controller log group."
  type        = number
  default     = 30

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653], var.log_retention_days)
    error_message = "log_retention_days must be a retention value supported by CloudWatch Logs."
  }
}

variable "log_kms_key_arn" {
  description = "Optional KMS key ARN for the log group. The key policy must allow the CloudWatch Logs service."
  type        = string
  default     = null
}

###############################################################################
# Controller state
###############################################################################

variable "create_state_table" {
  description = "Create the DynamoDB controller-state table using the repository's controller-state module."
  type        = bool
  default     = true
}

variable "state_table_name" {
  description = "Name for the created state table. Defaults to <name_prefix>-state."
  type        = string
  default     = null
}

variable "state_kms_key_arn" {
  description = "Optional customer-managed KMS key ARN for the state table. Null uses the AWS-managed DynamoDB key."
  type        = string
  default     = null
}

variable "state_enable_point_in_time_recovery" {
  description = "Enable DynamoDB point-in-time recovery on the created state table."
  type        = bool
  default     = true
}

variable "state_enable_deletion_protection" {
  description = "Enable DynamoDB deletion protection on the created state table."
  type        = bool
  default     = true
}

variable "existing_state_table" {
  description = "Use an existing controller-state table instead of creating one (set create_state_table = false). The table must follow the controller-state schema."
  type = object({
    name              = string
    arn               = string
    state_index_name  = optional(string, "gsi1")
    expiry_index_name = optional(string, "gsi2")
  })
  default = null
}

###############################################################################
# Secrets (ARN references only; values are never handled by Terraform)
###############################################################################

variable "create_secret_containers" {
  description = "Create empty Secrets Manager secrets (no secret version) for the webhook secret and, when GitHub JIT is enabled, the installation token. Populate them outside Terraform."
  type        = bool
  default     = true
}

variable "secrets_kms_key_arn" {
  description = "Optional customer-managed KMS key ARN used by the secrets. The execution role is granted kms:Decrypt on it."
  type        = string
  default     = null
}

variable "secret_recovery_window_in_days" {
  description = "Recovery window for created secret containers (0 deletes immediately)."
  type        = number
  default     = 7

  validation {
    condition     = var.secret_recovery_window_in_days == 0 || (var.secret_recovery_window_in_days >= 7 && var.secret_recovery_window_in_days <= 30)
    error_message = "secret_recovery_window_in_days must be 0 or between 7 and 30."
  }
}

variable "create_github_token_secret" {
  description = "Create the GitHub installation-token secret container even while github.jit_enabled is false (it is always created when JIT is enabled and create_secret_containers is true). Useful to keep an existing container during a staged rollout."
  type        = bool
  default     = false
}

variable "existing_webhook_secret_arn" {
  description = "ARN of an existing Secrets Manager secret holding the GitHub webhook secret (when create_secret_containers = false)."
  type        = string
  default     = null
}

variable "existing_github_token_secret_arn" {
  description = "ARN of an existing Secrets Manager secret holding the GitHub installation token (when create_secret_containers = false)."
  type        = string
  default     = null
}

variable "existing_github_app_private_key_secret_arn" {
  description = "ARN of an existing Secrets Manager secret holding the GitHub App private key in PEM form (when create_secret_containers = false and github.app is set)."
  type        = string
  default     = null
}

variable "secrets_ready" {
  description = "Operator assertion that every referenced secret has an approved, non-empty value. Terraform cannot verify this without reading the secret, so the service refuses to start until it is true."
  type        = bool
  default     = false
}

###############################################################################
# Network and ALB integration
###############################################################################

variable "vpc_id" {
  description = "VPC for the task security group and target group."
  type        = string
  default     = null
}

variable "subnet_ids" {
  description = "Subnets for the Fargate tasks."
  type        = list(string)
  default     = []
}

variable "assign_public_ip" {
  description = "Assign a public IP to tasks. Leave false when tasks reach AWS and GitHub through NAT or VPC endpoints."
  type        = bool
  default     = false
}

variable "alb_security_group_id" {
  description = "Security group of the ALB that forwards to the controller. The only ingress source allowed to the task."
  type        = string
  default     = null
}

variable "container_port" {
  description = "Controller listen port."
  type        = number
  default     = 8080

  validation {
    condition     = var.container_port >= 1 && var.container_port <= 65535
    error_message = "container_port must be a valid TCP port."
  }
}

variable "egress_https_cidr_blocks" {
  description = "IPv4 CIDRs the task may reach on TCP/443 (GitHub API, AWS APIs). No default: choose explicitly, for example your NAT or VPC endpoint ranges."
  type        = list(string)

  validation {
    condition     = alltrue([for c in var.egress_https_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every egress_https_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

variable "egress_https_prefix_list_ids" {
  description = "Managed prefix list IDs the task may reach on TCP/443 (for example S3/DynamoDB gateway endpoints)."
  type        = list(string)
  default     = []
}

variable "egress_dns_cidr_blocks" {
  description = "IPv4 CIDRs the task may reach on TCP/UDP 53. Needed only for custom resolvers; the Amazon-provided VPC resolver is not filtered by security groups."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.egress_dns_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every egress_dns_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

variable "health_check_path" {
  description = "Target group health check path."
  type        = string
  default     = "/healthz"

  validation {
    condition     = startswith(var.health_check_path, "/")
    error_message = "health_check_path must start with /."
  }
}

variable "health_check_matcher" {
  description = "HTTP codes treated as healthy."
  type        = string
  default     = "200"
}

variable "health_check_interval_seconds" {
  description = "Seconds between health checks."
  type        = number
  default     = 15
}

variable "health_check_timeout_seconds" {
  description = "Health check timeout in seconds."
  type        = number
  default     = 5
}

variable "healthy_threshold" {
  description = "Consecutive successes before a target is healthy."
  type        = number
  default     = 2
}

variable "unhealthy_threshold" {
  description = "Consecutive failures before a target is unhealthy."
  type        = number
  default     = 3
}

variable "deregistration_delay_seconds" {
  description = "Target deregistration delay."
  type        = number
  default     = 30
}

variable "listener_arn" {
  description = "ARN of the existing HTTPS listener the rule attaches to. Required when enable_listener_rule is true."
  type        = string
  default     = null
}

variable "listener_path_patterns" {
  description = "Path patterns forwarded to the controller (for example the webhook path). No default; nothing is hardcoded."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for p in var.listener_path_patterns : startswith(p, "/")])
    error_message = "Every listener path pattern must start with /."
  }
}

variable "listener_host_headers" {
  description = "Optional host header conditions for the rule."
  type        = list(string)
  default     = []
}

variable "listener_priority" {
  description = "Listener rule priority. Required when enable_listener_rule is true."
  type        = number
  default     = null

  validation {
    condition     = var.listener_priority == null || (var.listener_priority >= 1 && var.listener_priority <= 50000)
    error_message = "listener_priority must be between 1 and 50000."
  }
}

###############################################################################
# ECS service
###############################################################################

variable "create_cluster" {
  description = "Create a dedicated ECS cluster during the foundation phase. Leave false and set cluster_arn to use an existing cluster."
  type        = bool
  default     = false
}

variable "cluster_arn" {
  description = "ARN of an existing ECS cluster to run the service in."
  type        = string
  default     = null
}

variable "cpu" {
  description = "Fargate task CPU units."
  type        = number
  default     = 512

  validation {
    condition     = contains([256, 512, 1024, 2048, 4096], var.cpu)
    error_message = "cpu must be one of 256, 512, 1024, 2048, 4096."
  }
}

variable "memory" {
  description = "Fargate task memory in MiB. Must be a valid pairing for the chosen cpu."
  type        = number
  default     = 1024

  validation {
    condition     = var.memory >= 512 && var.memory <= 30720
    error_message = "memory must be between 512 and 30720 MiB."
  }
}

variable "cpu_architecture" {
  description = "Task CPU architecture. Must match the architecture the published image was built for."
  type        = string
  default     = "X86_64"

  validation {
    condition     = contains(["X86_64", "ARM64"], var.cpu_architecture)
    error_message = "cpu_architecture must be X86_64 or ARM64."
  }
}

variable "desired_count" {
  description = "Controller replicas. The current controller supports a single replica; 0 keeps the service defined but stopped."
  type        = number
  default     = 1

  validation {
    condition     = var.desired_count == 0 || var.desired_count == 1
    error_message = "desired_count must be 0 or 1 until the controller wires multi-replica coordination."
  }
}

variable "task_role_policy_json" {
  description = "Reviewed runtime IAM policy JSON generated with IAM Policy Autopilot. Wildcard iam:PassRole and Allow+NotAction statements are rejected."
  type        = string
  default     = null
}

variable "task_role_policy_arns" {
  description = "ARNs of reviewed customer-managed policies attached to the task role. These cannot be inspected by Terraform, so runner lifecycle requires task_role_policy_json."
  type        = set(string)
  default     = []
}

variable "extra_environment" {
  description = "Additional non-secret environment variables. Keys that look like credentials are rejected; use Secrets Manager references instead."
  type        = map(string)
  default     = {}

  validation {
    condition     = alltrue([for k in keys(var.extra_environment) : !can(regex("(?i)(secret|token|password|private_?key|api_?key|credential)", k))])
    error_message = "extra_environment keys must not look like credentials (secret, token, password, private key, api key, credential)."
  }
}

###############################################################################
# GitHub runtime contract
###############################################################################

variable "github" {
  description = <<-EOT
    GitHub runtime settings. With jit_enabled = false the controller accepts
    and verifies webhooks but provisions nothing real. With jit_enabled = true
    the runner group, an organization or repository allowlist, a label
    allowlist, and one way to authenticate are all required, matching the
    controller's startup validation.

    runner_scope picks where runners register. "repository" (default) uses the
    repository endpoint and needs the App to hold repository Administration:
    write. "organization" uses the organization endpoint and needs only the
    organization "Self-hosted runners: write" permission; runner_group_id must
    then be an organization runner group, and that group's repository access
    decides which repositories may use the runners.

    Authentication is either a GitHub App (set `app`; the controller mints its
    own short-lived installation tokens from a private key held in Secrets
    Manager) or an externally rotated installation token (leave `app` null).
    The two are mutually exclusive. `app.token_repositories` limits every minted
    token to those repositories (names, or owner/name; one owner). Left empty,
    the controller derives the limit from `allowed_repositories` when no
    organization is allowed, so a token cannot reach the App's other
    repositories. `app.token_permissions` optionally narrows
    each minted token below what the App was granted, for example
    { administration = "write" }.
  EOT
  type = object({
    jit_enabled           = optional(bool, false)
    runner_group_id       = optional(number)
    allowed_organizations = optional(list(string), [])
    allowed_repositories  = optional(list(string), [])
    allowed_labels        = optional(list(string), [])
    allow_forks           = optional(bool, false)
    api_base_url          = optional(string)
    runner_scope          = optional(string, "repository")
    app = optional(object({
      app_id             = string
      installation_id    = number
      token_permissions  = optional(map(string), {})
      token_repositories = optional(list(string), [])
    }))
  })
  default = {}

  validation {
    condition     = var.github.app == null || can(regex("^[A-Za-z0-9._-]{1,64}$", var.github.app.app_id))
    error_message = "github.app.app_id must be the GitHub App's numeric ID or client ID."
  }

  validation {
    condition     = var.github.app == null || var.github.app.installation_id > 0
    error_message = "github.app.installation_id must be a positive installation ID."
  }

  validation {
    condition = var.github.app == null || alltrue([
      for name, level in var.github.app.token_permissions :
      can(regex("^[a-z][a-z0-9_]{0,63}$", name)) && contains(["read", "write", "admin"], level)
    ])
    error_message = "github.app.token_permissions must map lowercase permission names to read, write, or admin."
  }

  validation {
    condition = var.github.app == null || alltrue([
      for r in var.github.app.token_repositories :
      can(regex("^([A-Za-z0-9._-]{1,100}/)?[A-Za-z0-9._-]{1,100}$", r))
    ])
    error_message = "github.app.token_repositories entries must be repository names or owner/name."
  }

  validation {
    condition     = var.github.app == null || var.github.jit_enabled
    error_message = "github.app requires github.jit_enabled = true."
  }

  validation {
    condition     = contains(["repository", "organization"], var.github.runner_scope)
    error_message = "github.runner_scope must be repository or organization."
  }

  validation {
    condition     = var.github.runner_scope == "repository" || var.github.jit_enabled
    error_message = "github.runner_scope = organization requires github.jit_enabled = true."
  }
}

###############################################################################
# Future runner lifecycle (off by default)
###############################################################################

variable "runner" {
  description = <<-EOT
    Ephemeral EC2 runner lifecycle settings. Disabled by default; the module
    never creates runner infrastructure (the aws-runner-layer module does).
    Enabling it requires the dedicated runner role and instance profile ARNs,
    a launch template, user data, GitHub JIT, and a reviewed task policy that
    scopes iam:PassRole to exactly runner.role_arn.

    user_data is the runner bootstrap script and must contain the
    {{GITHUB_JIT_CONFIG_B64}} placeholder exactly once. max_lifetime_minutes is
    the hard runner lifetime the reaper enforces. max_runners caps concurrent
    runners and defaults to 1 so a first validation cannot fan out. tags are
    extra instance tags (also required to match by the reaper backstop). Leave
    subnet_id and security_group_ids null when the launch template defines its
    own network interface: EC2 refuses both.
  EOT
  type = object({
    enabled                 = optional(bool, false)
    launch_template_id      = optional(string)
    launch_template_version = optional(string)
    subnet_id               = optional(string)
    security_group_ids      = optional(list(string), [])
    instance_profile_arn    = optional(string)
    role_arn                = optional(string)
    instance_type           = optional(string)
    pool_id                 = optional(string, "default")
    labels                  = optional(list(string), [])
    user_data               = optional(string)
    max_lifetime_minutes    = optional(number, 120)
    max_runners             = optional(number, 1)
    tags                    = optional(map(string), {})
  })
  default = {}

  validation {
    condition     = var.runner.user_data == null || length(split("{{GITHUB_JIT_CONFIG_B64}}", var.runner.user_data)) == 2
    error_message = "runner.user_data must contain the {{GITHUB_JIT_CONFIG_B64}} placeholder exactly once; the controller replaces it with the one-time JIT configuration."
  }

  validation {
    condition     = var.runner.user_data == null || length(var.runner.user_data) <= 12000
    error_message = "runner.user_data must be at most 12000 characters (EC2 allows 16 KB before encoding)."
  }

  validation {
    condition     = var.runner.max_lifetime_minutes >= 5 && var.runner.max_lifetime_minutes <= 1440
    error_message = "runner.max_lifetime_minutes must be between 5 and 1440."
  }

  validation {
    condition     = var.runner.max_runners >= 1 && var.runner.max_runners <= 1000
    error_message = "runner.max_runners must be between 1 and 1000."
  }

  validation {
    condition = alltrue([
      for k, v in var.runner.tags :
      !startswith(lower(k), "leo-runners:") && !startswith(lower(k), "aws:") && length(k) <= 128 && length(v) >= 1 && length(v) <= 256 && !strcontains(k, ",") && !strcontains(k, "=") && !strcontains(v, ",")
    ])
    error_message = "runner.tags keys must not use the reserved leo-runners: or aws: prefixes, keys and values must be non-empty, and neither may contain a comma (keys may not contain an equals sign either)."
  }

  validation {
    condition     = var.runner.role_arn == null || can(regex("^arn:[^:]+:iam::[0-9]{12}:role/.+$", var.runner.role_arn))
    error_message = "runner.role_arn must be an IAM role ARN."
  }

  validation {
    condition     = var.runner.instance_profile_arn == null || can(regex("^arn:[^:]+:iam::[0-9]{12}:instance-profile/.+$", var.runner.instance_profile_arn))
    error_message = "runner.instance_profile_arn must be an IAM instance profile ARN."
  }
}
