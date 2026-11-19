data "aws_partition" "current" {}

data "aws_region" "current" {}

data "aws_caller_identity" "current" {}

locals {
  common_tags = merge(
    {
      "managed-by" = "terraform"
      "component"  = "leo-runners-ecs-controller"
    },
    var.tags,
  )

  # `region` exists on provider v6 and later; `name` on v5.
  region     = try(data.aws_region.current.region, data.aws_region.current.name)
  foundation = var.create_foundation
  phase      = var.enable_service ? "service" : (var.create_foundation ? "foundation" : "disabled")

  # ---------------------------------------------------------------------------
  # Resolved resource references (null until the owning phase is enabled)
  # ---------------------------------------------------------------------------
  state = local.foundation ? (
    var.create_state_table ? {
      name              = module.state[0].table_name
      arn               = module.state[0].table_arn
      state_index_name  = module.state[0].state_index_name
      expiry_index_name = module.state[0].expiry_index_name
    } : var.existing_state_table
  ) : null

  webhook_secret_arn = local.foundation ? (
    var.create_secret_containers ? aws_secretsmanager_secret.webhook[0].arn : var.existing_webhook_secret_arn
  ) : null

  # GitHub authentication is either an App (the controller mints its own
  # tokens from a private key) or an externally rotated installation token.
  use_github_app   = var.github.jit_enabled && var.github.app != null
  use_github_token = var.github.jit_enabled && var.github.app == null

  create_github_token_container   = local.foundation && var.create_secret_containers && (local.use_github_token || var.create_github_token_secret)
  create_github_app_key_container = local.foundation && var.create_secret_containers && local.use_github_app

  github_token_secret_arn = local.foundation && local.use_github_token ? (
    var.create_secret_containers ? aws_secretsmanager_secret.github_token[0].arn : var.existing_github_token_secret_arn
  ) : null

  github_app_private_key_secret_arn = local.foundation && local.use_github_app ? (
    var.create_secret_containers ? aws_secretsmanager_secret.github_app_key[0].arn : var.existing_github_app_private_key_secret_arn
  ) : null

  execution_role_name = var.execution_role_name != null ? var.execution_role_name : "${var.name_prefix}-controller-execution"
  task_role_name      = var.task_role_name != null ? var.task_role_name : "${var.name_prefix}-controller"

  # Built from known flags, never from ARN null-checks, so list lengths stay
  # known at plan time even though the ARNs themselves are not.
  secret_arns = concat(
    local.foundation ? [local.webhook_secret_arn] : [],
    local.foundation && local.use_github_token ? [local.github_token_secret_arn] : [],
    local.foundation && local.use_github_app ? [local.github_app_private_key_secret_arn] : [],
  )

  cluster_arn = var.create_cluster ? (local.foundation ? aws_ecs_cluster.controller[0].arn : null) : var.cluster_arn

  image_repository = local.foundation && var.create_ecr_repository ? aws_ecr_repository.controller[0].repository_url : var.image_repository
  image_uri        = var.image_digest != null && ((local.foundation && var.create_ecr_repository) || var.image_repository != null) ? "${local.image_repository}@${var.image_digest}" : null

  # listener_arn may be a value that is only known after apply (for example a
  # listener created by aws-shared-gateway in the same run), so it must never
  # decide a resource count. Only the literal path and priority inputs do.
  listener_route_inputs_ok = length(var.listener_path_patterns) > 0 && var.listener_priority != null

  has_runtime_policy = var.task_role_policy_json != null || length(var.task_role_policy_arns) > 0

  # ---------------------------------------------------------------------------
  # Runtime policy inspection. Terraform cannot prove a policy is least
  # privilege; it only rejects the shapes that are never acceptable here.
  # ---------------------------------------------------------------------------
  policy_doc   = var.task_role_policy_json == null ? null : try(jsondecode(var.task_role_policy_json), null)
  policy_valid = local.policy_doc != null && try(local.policy_doc.Version == "2012-10-17" && local.policy_doc.Statement != null, false)

  policy_raw_statement     = local.policy_valid ? local.policy_doc.Statement : null
  policy_statement_is_list = local.policy_raw_statement != null && try(local.policy_raw_statement[0] != null, false)

  policy_statement_json = local.policy_raw_statement == null ? [] : (
    local.policy_statement_is_list ?
    [for i in range(length(local.policy_raw_statement)) : jsonencode(local.policy_raw_statement[i])] :
    [jsonencode(local.policy_raw_statement)]
  )

  policy_statements = [
    for s in local.policy_statement_json : {
      effect     = try(jsondecode(s).Effect, "")
      actions    = try(tolist(jsondecode(s).Action), [tostring(jsondecode(s).Action)], [])
      resources  = try(tolist(jsondecode(s).Resource), [tostring(jsondecode(s).Resource)], [])
      not_action = can(jsondecode(s).NotAction)
    }
  ]

  policy_allow_statements = [for s in local.policy_statements : s if s.effect == "Allow"]

  policy_has_allow_not_action = anytrue([for s in local.policy_allow_statements : s.not_action])
  policy_has_broad_action = anytrue([
    for s in local.policy_allow_statements : anytrue([
      for a in s.actions : a == "*" || (startswith(lower(a), "iam:") && strcontains(a, "*"))
    ])
  ])
  policy_has_wildcard_passrole = anytrue([
    for s in local.policy_allow_statements :
    anytrue([for a in s.actions : lower(a) == "iam:passrole"]) &&
    anytrue([for r in s.resources : strcontains(r, "*")])
  ])
  policy_passrole_scoped_to_runner = var.runner.role_arn != null && anytrue([
    for s in local.policy_allow_statements :
    anytrue([for a in s.actions : lower(a) == "iam:passrole"]) &&
    contains(s.resources, var.runner.role_arn)
  ])

  # ---------------------------------------------------------------------------
  # Container environment. Module-owned keys always win over extra_environment.
  # ---------------------------------------------------------------------------
  base_environment = local.foundation ? {
    LISTEN_ADDR           = ":${var.container_port}"
    AWS_REGION            = local.region
    DYNAMODB_TABLE_NAME   = try(local.state.name, "")
    DYNAMODB_STATE_INDEX  = try(local.state.state_index_name, "")
    DYNAMODB_EXPIRY_INDEX = try(local.state.expiry_index_name, "")
    AI_ENABLED            = "false"
    CAPACITY_POOL_ID      = var.runner.pool_id
  } : {}

  github_environment = var.github.jit_enabled ? {
    GITHUB_RUNNER_GROUP_ID       = try(tostring(var.github.runner_group_id), "")
    GITHUB_ALLOWED_ORGANIZATIONS = join(",", var.github.allowed_organizations)
    GITHUB_ALLOWED_REPOSITORIES  = join(",", var.github.allowed_repositories)
    GITHUB_ALLOWED_LABELS        = join(",", var.github.allowed_labels)
    GITHUB_ALLOW_FORKS           = tostring(var.github.allow_forks)
    GITHUB_API_BASE_URL          = var.github.api_base_url != null ? var.github.api_base_url : ""
  } : {}

  github_app_environment = local.use_github_app ? {
    GITHUB_APP_ID                = try(var.github.app.app_id, "")
    GITHUB_APP_INSTALLATION_ID   = try(tostring(var.github.app.installation_id), "")
    GITHUB_APP_TOKEN_PERMISSIONS = join(",", [for name in sort(keys(try(var.github.app.token_permissions, {}))) : "${name}=${var.github.app.token_permissions[name]}"])
  } : {}

  runner_environment = var.runner.enabled ? {
    CAPACITY_PROVIDER               = "aws"
    AWS_LAUNCH_TEMPLATE_ID          = var.runner.launch_template_id != null ? var.runner.launch_template_id : ""
    AWS_LAUNCH_TEMPLATE_VERSION     = var.runner.launch_template_version != null ? var.runner.launch_template_version : ""
    AWS_SUBNET_ID                   = var.runner.subnet_id != null ? var.runner.subnet_id : ""
    AWS_SECURITY_GROUP_IDS          = join(",", var.runner.security_group_ids)
    AWS_RUNNER_INSTANCE_PROFILE_ARN = var.runner.instance_profile_arn != null ? var.runner.instance_profile_arn : ""
    CAPACITY_LABELS                 = join(",", var.runner.labels)
  } : {}

  environment = {
    for k, v in merge(var.extra_environment, local.base_environment, local.github_environment, local.github_app_environment, local.runner_environment) : k => v if v != ""
  }

  container_secrets = concat(
    local.foundation ? [{ name = "GITHUB_WEBHOOK_SECRET", valueFrom = local.webhook_secret_arn }] : [],
    local.foundation && local.use_github_token ? [{ name = "GITHUB_APP_INSTALLATION_TOKEN", valueFrom = local.github_token_secret_arn }] : [],
    local.foundation && local.use_github_app ? [{ name = "GITHUB_APP_PRIVATE_KEY", valueFrom = local.github_app_private_key_secret_arn }] : [],
  )

  ecs_tasks_trust = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.${data.aws_partition.current.dns_suffix}" }
      Action    = "sts:AssumeRole"
      Condition = {
        StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.current.account_id }
      }
    }]
  })
}

###############################################################################
# Phase guards. Every cross-input rule lives here so a bad combination fails
# at plan time, before any AWS API call.
###############################################################################

resource "terraform_data" "guard" {
  input = local.phase

  lifecycle {
    precondition {
      condition     = !var.enable_listener_rule || var.create_foundation
      error_message = "enable_listener_rule requires create_foundation = true."
    }

    precondition {
      condition     = !var.enable_service || (var.create_foundation && var.enable_listener_rule)
      error_message = "enable_service requires create_foundation = true and enable_listener_rule = true."
    }

    precondition {
      condition     = !var.create_foundation || (var.vpc_id != null && var.alb_security_group_id != null)
      error_message = "create_foundation requires vpc_id and alb_security_group_id."
    }

    precondition {
      condition     = !var.create_foundation || var.create_state_table || var.existing_state_table != null
      error_message = "Provide existing_state_table when create_state_table = false."
    }

    precondition {
      condition     = !(var.create_state_table && var.existing_state_table != null)
      error_message = "create_state_table and existing_state_table are mutually exclusive."
    }

    precondition {
      condition     = !var.create_foundation || var.create_secret_containers || var.existing_webhook_secret_arn != null
      error_message = "Provide existing_webhook_secret_arn when create_secret_containers = false."
    }

    precondition {
      condition     = !var.create_foundation || !local.use_github_token || var.create_secret_containers || var.existing_github_token_secret_arn != null
      error_message = "GitHub JIT with a static installation token requires a token secret: set create_secret_containers = true or provide existing_github_token_secret_arn."
    }

    precondition {
      condition     = !var.create_foundation || !local.use_github_app || var.create_secret_containers || var.existing_github_app_private_key_secret_arn != null
      error_message = "GitHub App authentication requires a private-key secret: set create_secret_containers = true or provide existing_github_app_private_key_secret_arn."
    }

    precondition {
      condition     = !(var.create_secret_containers && (var.existing_webhook_secret_arn != null || var.existing_github_token_secret_arn != null || var.existing_github_app_private_key_secret_arn != null))
      error_message = "create_secret_containers and existing_*_secret_arn inputs are mutually exclusive."
    }

    precondition {
      condition     = !(var.create_cluster && var.cluster_arn != null)
      error_message = "create_cluster and cluster_arn are mutually exclusive."
    }

    precondition {
      condition     = !var.enable_listener_rule || (var.listener_arn != null && local.listener_route_inputs_ok)
      error_message = "enable_listener_rule requires listener_arn, at least one listener_path_patterns entry, and listener_priority."
    }

    precondition {
      condition     = !var.enable_service || var.image_digest != null
      error_message = "enable_service requires image_digest (sha256:<64 hex>). Mutable tags are not accepted."
    }

    precondition {
      condition     = !var.enable_service || (local.foundation && var.create_ecr_repository) || var.image_repository != null
      error_message = "enable_service requires an image repository: create_ecr_repository = true or image_repository."
    }

    precondition {
      condition     = !var.enable_service || var.secrets_ready
      error_message = "enable_service requires secrets_ready = true after the secret containers hold approved values."
    }

    precondition {
      condition     = !var.enable_service || local.has_runtime_policy
      error_message = "enable_service requires a reviewed runtime policy: task_role_policy_json or task_role_policy_arns."
    }

    precondition {
      condition     = !var.enable_service || length(var.subnet_ids) > 0
      error_message = "enable_service requires at least one subnet in subnet_ids."
    }

    precondition {
      condition     = !var.enable_service || var.create_cluster || var.cluster_arn != null
      error_message = "enable_service requires create_cluster = true or cluster_arn."
    }

    precondition {
      condition     = var.task_role_policy_json == null || (local.policy_valid && length(local.policy_statements) > 0 && alltrue([for s in local.policy_statements : contains(["Allow", "Deny"], s.effect)]))
      error_message = "task_role_policy_json must be a valid IAM policy document (Version 2012-10-17) with at least one statement, each with Effect Allow or Deny."
    }

    precondition {
      condition     = !local.policy_has_allow_not_action
      error_message = "task_role_policy_json must not contain an Allow statement that uses NotAction."
    }

    precondition {
      condition     = !local.policy_has_broad_action
      error_message = "task_role_policy_json must not allow Action \"*\" or wildcard iam: actions."
    }

    precondition {
      condition     = !local.policy_has_wildcard_passrole
      error_message = "task_role_policy_json must not allow iam:PassRole on a Resource containing a wildcard. Scope it to the dedicated runner role ARN."
    }

    precondition {
      condition     = !var.github.jit_enabled || try(var.github.runner_group_id > 0, false)
      error_message = "github.jit_enabled requires github.runner_group_id > 0."
    }

    precondition {
      condition     = !var.github.jit_enabled || (length(var.github.allowed_organizations) + length(var.github.allowed_repositories) > 0 && length(var.github.allowed_labels) > 0)
      error_message = "github.jit_enabled requires an organization or repository allowlist and a label allowlist."
    }

    precondition {
      condition     = var.github.jit_enabled || var.github.runner_group_id == null
      error_message = "github.runner_group_id must be unset when github.jit_enabled is false (the controller rejects it)."
    }

    precondition {
      condition     = !var.runner.enabled || var.github.jit_enabled
      error_message = "runner.enabled requires github.jit_enabled = true."
    }

    precondition {
      condition     = !var.runner.enabled || (var.runner.launch_template_id != null && var.runner.role_arn != null && var.runner.instance_profile_arn != null)
      error_message = "runner.enabled requires runner.launch_template_id, runner.role_arn, and runner.instance_profile_arn."
    }

    precondition {
      condition     = !var.runner.enabled || (var.task_role_policy_json != null && local.policy_passrole_scoped_to_runner)
      error_message = "runner.enabled requires task_role_policy_json containing an iam:PassRole statement whose Resource includes exactly runner.role_arn."
    }
  }
}

###############################################################################
# Foundation: image registry, logs, state, secrets
###############################################################################

resource "aws_ecr_repository" "controller" {
  count = local.foundation && var.create_ecr_repository ? 1 : 0

  name                 = "${var.name_prefix}-controller"
  image_tag_mutability = "IMMUTABLE"
  tags                 = merge(local.common_tags, { "component" = "controller-image" })

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = var.ecr_kms_key_arn == null ? "AES256" : "KMS"
    kms_key         = var.ecr_kms_key_arn
  }
}

resource "aws_ecr_lifecycle_policy" "controller" {
  count      = local.foundation && var.create_ecr_repository ? 1 : 0
  repository = aws_ecr_repository.controller[0].name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images beyond the retention count."
        selection = {
          tagStatus   = "untagged"
          countType   = "imageCountMoreThan"
          countNumber = var.ecr_untagged_image_retention_count
        }
        action = { type = "expire" }
      },
      {
        rulePriority = 2
        description  = "Cap the total number of images kept."
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = var.ecr_max_image_count
        }
        action = { type = "expire" }
      },
    ]
  })
}

resource "aws_cloudwatch_log_group" "controller" {
  count = local.foundation ? 1 : 0

  name              = "/ecs/${var.name_prefix}-controller"
  retention_in_days = var.log_retention_days
  kms_key_id        = var.log_kms_key_arn
  tags              = merge(local.common_tags, { "component" = "controller-logs" })
}

module "state" {
  count  = local.foundation && var.create_state_table ? 1 : 0
  source = "../controller-state"

  table_name                    = var.state_table_name != null ? var.state_table_name : "${var.name_prefix}-state"
  enable_point_in_time_recovery = var.state_enable_point_in_time_recovery
  enable_deletion_protection    = var.state_enable_deletion_protection
  kms_key_arn                   = var.state_kms_key_arn
  tags                          = local.common_tags
}

# Secret containers are created empty. Terraform never sees a secret value.
resource "aws_secretsmanager_secret" "webhook" {
  count = local.foundation && var.create_secret_containers ? 1 : 0

  name                    = "${var.name_prefix}/github-webhook-secret"
  description             = "GitHub webhook secret for the LeoRunners controller. Populate outside Terraform."
  kms_key_id              = var.secrets_kms_key_arn
  recovery_window_in_days = var.secret_recovery_window_in_days
  tags                    = merge(local.common_tags, { "component" = "controller-webhook-secret" })
}

# The App private key is created empty like every other secret. Terraform never
# sees the key; the deployer stores it in their own account.
resource "aws_secretsmanager_secret" "github_app_key" {
  count = local.create_github_app_key_container ? 1 : 0

  name                    = "${var.name_prefix}/github-app-private-key"
  description             = "GitHub App private key (PEM) for the LeoRunners controller. Populate outside Terraform."
  kms_key_id              = var.secrets_kms_key_arn
  recovery_window_in_days = var.secret_recovery_window_in_days
  tags                    = merge(local.common_tags, { "component" = "controller-github-app-key" })
}

resource "aws_secretsmanager_secret" "github_token" {
  count = local.create_github_token_container ? 1 : 0

  name                    = "${var.name_prefix}/github-installation-token"
  description             = "GitHub installation token for the LeoRunners controller. Populate and rotate outside Terraform."
  kms_key_id              = var.secrets_kms_key_arn
  recovery_window_in_days = var.secret_recovery_window_in_days
  tags                    = merge(local.common_tags, { "component" = "controller-github-token" })
}

###############################################################################
# Foundation: IAM (separate execution and task roles)
###############################################################################

resource "aws_iam_role" "execution" {
  count = local.foundation ? 1 : 0

  name                 = local.execution_role_name
  path                 = var.iam_path
  permissions_boundary = var.permissions_boundary_arn
  assume_role_policy   = local.ecs_tasks_trust
  description          = "ECS agent role: pulls the controller image, writes logs, reads the referenced secrets."
  tags                 = merge(local.common_tags, { "component" = "controller-execution-role" })
}

resource "aws_iam_role_policy_attachment" "execution" {
  count = local.foundation ? 1 : 0

  role       = aws_iam_role.execution[0].name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_secrets" {
  count = local.foundation ? 1 : 0

  name = "${local.execution_role_name}-secrets"
  role = aws_iam_role.execution[0].name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [{
        Sid      = "ReadReferencedSecrets"
        Effect   = "Allow"
        Action   = ["secretsmanager:GetSecretValue"]
        Resource = local.secret_arns
      }],
      var.secrets_kms_key_arn == null ? [] : [{
        Sid      = "DecryptSecrets"
        Effect   = "Allow"
        Action   = ["kms:Decrypt"]
        Resource = [var.secrets_kms_key_arn]
      }],
    )
  })
}

resource "aws_iam_role" "task" {
  count = local.foundation ? 1 : 0

  name                 = local.task_role_name
  path                 = var.iam_path
  permissions_boundary = var.permissions_boundary_arn
  assume_role_policy   = local.ecs_tasks_trust
  description          = "Controller runtime role. Permissions come only from the reviewed policy inputs."
  tags                 = merge(local.common_tags, { "component" = "controller-task-role" })
}

resource "aws_iam_role_policy" "task_runtime" {
  count = local.foundation && var.task_role_policy_json != null ? 1 : 0

  name   = "${var.name_prefix}-controller-runtime"
  role   = aws_iam_role.task[0].name
  policy = var.task_role_policy_json
}

resource "aws_iam_role_policy_attachment" "task_runtime" {
  for_each = local.foundation ? var.task_role_policy_arns : toset([])

  role       = aws_iam_role.task[0].name
  policy_arn = each.value
}

###############################################################################
# Foundation: network, cluster, target group
###############################################################################

resource "aws_security_group" "task" {
  count = local.foundation ? 1 : 0

  name        = "${var.name_prefix}-controller"
  description = var.task_security_group_description
  vpc_id      = var.vpc_id
  tags        = merge(local.common_tags, { "component" = "controller-task-security-group" })
}

resource "aws_vpc_security_group_ingress_rule" "from_alb" {
  count = local.foundation ? 1 : 0

  security_group_id            = aws_security_group.task[0].id
  referenced_security_group_id = var.alb_security_group_id
  description                  = "Controller traffic from the ALB only."
  from_port                    = var.container_port
  to_port                      = var.container_port
  ip_protocol                  = "tcp"
  tags                         = local.common_tags
}

resource "aws_vpc_security_group_egress_rule" "https_cidr" {
  for_each = local.foundation ? toset(var.egress_https_cidr_blocks) : toset([])

  security_group_id = aws_security_group.task[0].id
  cidr_ipv4         = each.value
  description       = "HTTPS egress to ${each.value}."
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_egress_rule" "https_prefix_list" {
  for_each = local.foundation ? toset(var.egress_https_prefix_list_ids) : toset([])

  security_group_id = aws_security_group.task[0].id
  prefix_list_id    = each.value
  description       = "HTTPS egress to prefix list ${each.value}."
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_egress_rule" "dns_udp" {
  for_each = local.foundation ? toset(var.egress_dns_cidr_blocks) : toset([])

  security_group_id = aws_security_group.task[0].id
  cidr_ipv4         = each.value
  description       = "DNS (UDP) egress to ${each.value}."
  from_port         = 53
  to_port           = 53
  ip_protocol       = "udp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_egress_rule" "dns_tcp" {
  for_each = local.foundation ? toset(var.egress_dns_cidr_blocks) : toset([])

  security_group_id = aws_security_group.task[0].id
  cidr_ipv4         = each.value
  description       = "DNS (TCP) egress to ${each.value}."
  from_port         = 53
  to_port           = 53
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

resource "aws_ecs_cluster" "controller" {
  count = local.foundation && var.create_cluster ? 1 : 0

  name = "${var.name_prefix}-controller"
  tags = merge(local.common_tags, { "component" = "controller-cluster" })
}

resource "aws_lb_target_group" "controller" {
  count = local.foundation ? 1 : 0

  name                 = "${var.name_prefix}-controller"
  port                 = var.container_port
  protocol             = "HTTP"
  target_type          = "ip"
  vpc_id               = var.vpc_id
  deregistration_delay = var.deregistration_delay_seconds
  tags                 = merge(local.common_tags, { "component" = "controller-target-group" })

  health_check {
    enabled             = true
    path                = var.health_check_path
    protocol            = "HTTP"
    matcher             = var.health_check_matcher
    interval            = var.health_check_interval_seconds
    timeout             = var.health_check_timeout_seconds
    healthy_threshold   = var.healthy_threshold
    unhealthy_threshold = var.unhealthy_threshold
  }
}

###############################################################################
# Phase 2: listener rule and service
###############################################################################

resource "aws_lb_listener_rule" "controller" {
  count = var.enable_listener_rule && local.foundation && local.listener_route_inputs_ok ? 1 : 0

  listener_arn = var.listener_arn
  priority     = var.listener_priority
  tags         = merge(local.common_tags, { "component" = "controller-route" })

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.controller[0].arn
  }

  condition {
    path_pattern {
      values = var.listener_path_patterns
    }
  }

  dynamic "condition" {
    for_each = length(var.listener_host_headers) > 0 ? [1] : []
    content {
      host_header {
        values = var.listener_host_headers
      }
    }
  }
}

resource "aws_ecs_task_definition" "controller" {
  count = var.enable_service && local.foundation ? 1 : 0

  family                   = "${var.name_prefix}-controller"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.cpu)
  memory                   = tostring(var.memory)
  execution_role_arn       = aws_iam_role.execution[0].arn
  task_role_arn            = aws_iam_role.task[0].arn
  tags                     = merge(local.common_tags, { "component" = "controller-task-definition" })

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = var.cpu_architecture
  }

  container_definitions = jsonencode([{
    name         = "controller"
    image        = local.image_uri
    essential    = true
    portMappings = [{ containerPort = var.container_port, protocol = "tcp" }]
    environment  = [for k in sort(keys(local.environment)) : { name = k, value = local.environment[k] }]
    secrets      = local.container_secrets
    stopTimeout  = 35
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.controller[0].name
        "awslogs-region"        = local.region
        "awslogs-stream-prefix" = "controller"
      }
    }
    readonlyRootFilesystem = true
    linuxParameters = {
      capabilities = { drop = ["ALL"] }
    }
  }])
}

resource "aws_ecs_service" "controller" {
  count = var.enable_service && local.foundation ? 1 : 0

  name                               = "${var.name_prefix}-controller"
  cluster                            = local.cluster_arn
  task_definition                    = aws_ecs_task_definition.controller[0].arn
  desired_count                      = var.desired_count
  launch_type                        = "FARGATE"
  propagate_tags                     = "SERVICE"
  health_check_grace_period_seconds  = 60
  deployment_minimum_healthy_percent = 0
  deployment_maximum_percent         = 100
  tags                               = merge(local.common_tags, { "component" = "controller-service" })

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = [aws_security_group.task[0].id]
    assign_public_ip = var.assign_public_ip
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.controller[0].arn
    container_name   = "controller"
    container_port   = var.container_port
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  depends_on = [aws_lb_listener_rule.controller]
}
