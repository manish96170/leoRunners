output "phase" {
  description = "Active phase: disabled, foundation, or service."
  value       = local.phase
}

output "ecr_repository_url" {
  description = "ECR repository URL for the controller image, or null when not created."
  value       = try(aws_ecr_repository.controller[0].repository_url, null)
}

output "ecr_repository_arn" {
  description = "ECR repository ARN, or null when not created."
  value       = try(aws_ecr_repository.controller[0].arn, null)
}

output "log_group_name" {
  description = "Controller CloudWatch log group name."
  value       = try(aws_cloudwatch_log_group.controller[0].name, null)
}

output "log_group_arn" {
  description = "Controller CloudWatch log group ARN."
  value       = try(aws_cloudwatch_log_group.controller[0].arn, null)
}

output "state_table_name" {
  description = "Controller-state DynamoDB table name."
  value       = try(local.state.name, null)
}

output "state_table_arn" {
  description = "Controller-state DynamoDB table ARN."
  value       = try(local.state.arn, null)
}

output "state_index_arns" {
  description = "ARNs of the sparse state and expiry indexes, for scoped data-plane policy review."
  value = local.state == null ? [] : [
    "${local.state.arn}/index/${local.state.state_index_name}",
    "${local.state.arn}/index/${local.state.expiry_index_name}",
  ]
}

output "webhook_secret_arn" {
  description = "Secrets Manager ARN for the GitHub webhook secret. Populate its value outside Terraform."
  value       = local.webhook_secret_arn
}

output "github_token_secret_arn" {
  description = "Secrets Manager ARN for the GitHub installation token container, or null when none exists."
  value       = try(aws_secretsmanager_secret.github_token[0].arn, local.github_token_secret_arn)
}

output "execution_role_arn" {
  description = "ECS task execution role ARN."
  value       = try(aws_iam_role.execution[0].arn, null)
}

output "task_role_arn" {
  description = "Controller task role ARN. Grant runtime access only through the reviewed policy inputs."
  value       = try(aws_iam_role.task[0].arn, null)
}

output "task_role_name" {
  description = "Controller task role name."
  value       = try(aws_iam_role.task[0].name, null)
}

output "task_security_group_id" {
  description = "Security group attached to the controller tasks."
  value       = try(aws_security_group.task[0].id, null)
}

output "target_group_arn" {
  description = "Controller target group ARN."
  value       = try(aws_lb_target_group.controller[0].arn, null)
}

output "cluster_arn" {
  description = "ECS cluster ARN used by the service, or null when none is configured."
  value       = local.cluster_arn
}

output "listener_rule_arn" {
  description = "Listener rule ARN, or null until enable_listener_rule is true."
  value       = try(aws_lb_listener_rule.controller[0].arn, null)
}

output "service_name" {
  description = "ECS service name, or null until enable_service is true."
  value       = try(aws_ecs_service.controller[0].name, null)
}

output "task_definition_arn" {
  description = "ECS task definition ARN, or null until enable_service is true."
  value       = try(aws_ecs_task_definition.controller[0].arn, null)
}

output "image_uri" {
  description = "Immutable image reference the service runs, or null when no digest is configured."
  value       = local.image_uri
}

output "policy_review_scope" {
  description = "Resource identifiers to use when generating and reviewing the controller runtime policy with IAM Policy Autopilot."
  value = {
    account_id          = data.aws_caller_identity.current.account_id
    region              = local.region
    state_table_arn     = try(local.state.arn, null)
    state_index_arns    = local.state == null ? [] : ["${local.state.arn}/index/${local.state.state_index_name}", "${local.state.arn}/index/${local.state.expiry_index_name}"]
    runner_role_arn     = var.runner.role_arn
    launch_template_id  = var.runner.launch_template_id
    runner_lifecycle_on = var.runner.enabled
  }
}

output "container_environment" {
  description = "Non-secret environment variables the controller container receives. Secrets are injected by ARN reference and are not listed."
  value       = local.environment
}
