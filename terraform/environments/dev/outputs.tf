output "state_table_name" {
  description = "DynamoDB table used by the controller."
  value       = module.controller_state.table_name
}

output "state_table_arn" {
  description = "DynamoDB controller-state table ARN."
  value       = module.controller_state.table_arn
}

output "controller_role_arn" {
  description = "Controller runtime role ARN."
  value       = module.controller_iam.role_arn
}

output "runner_role_arn" {
  description = "Ephemeral runner role ARN."
  value       = module.runner_runtime.role_arn
}

output "runner_instance_profile_name" {
  description = "Instance profile name for the EC2 Launch Template."
  value       = module.runner_runtime.instance_profile_name
}

output "observability_log_group_name" {
  description = "Optional CloudWatch telemetry log group; null when observability is disabled."
  value       = var.enable_observability ? module.observability[0].log_group_name : null
}

output "observability_alarm_arns" {
  description = "Optional notification-only alarm ARNs; empty when observability is disabled."
  value       = var.enable_observability ? module.observability[0].alarm_arns : {}
}

output "observability_dashboard_name" {
  description = "Optional CloudWatch dashboard name; null when observability is disabled."
  value       = var.enable_observability ? module.observability[0].dashboard_name : null
}

output "composition_scope" {
  description = "Resources deliberately excluded from this development composition."
  value = [
    "networking",
    "launch-template",
    "ami",
    "github-app",
    "backend-configuration",
    "observability-unless-explicitly-enabled",
  ]
}
