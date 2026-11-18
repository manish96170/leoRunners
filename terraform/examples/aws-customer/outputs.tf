output "phase" {
  value = module.controller.phase
}

output "ecr_repository_url" {
  description = "Publish the controller image here, then record its digest."
  value       = module.controller.ecr_repository_url
}

output "webhook_secret_arn" {
  description = "Populate this secret outside Terraform."
  value       = module.controller.webhook_secret_arn
}

output "github_token_secret_arn" {
  value = module.controller.github_token_secret_arn
}

output "state_table_arn" {
  value = module.controller.state_table_arn
}

output "task_role_arn" {
  value = module.controller.task_role_arn
}

output "policy_review_scope" {
  description = "Inputs for generating and reviewing the controller runtime policy."
  value       = module.controller.policy_review_scope
}

output "container_environment" {
  value = module.controller.container_environment
}
