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

output "runner_layer" {
  description = "Runner layer identifiers for the reviewed controller policy and for cleanup. Null when the layer is not enabled."
  value = var.runner_layer == null ? null : {
    launch_template_arn  = module.runner_layer[0].launch_template_arn
    role_arn             = module.runner_layer[0].role_arn
    instance_profile_arn = module.runner_layer[0].instance_profile_arn
    security_group_ids   = module.runner_layer[0].security_group_ids
    cleanup_selector     = module.runner_layer[0].cleanup_selector
    policy_constraints   = module.runner_layer[0].controller_policy_constraints
    ownership_tags       = module.runner_layer[0].ownership_tags
  }
}
