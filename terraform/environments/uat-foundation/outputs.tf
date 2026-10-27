output "controller_ecr_repository_url" {
  description = "Dedicated immutable controller image repository URL."
  value       = aws_ecr_repository.controller.repository_url
}

output "controller_ecs_cluster_name" {
  description = "Dedicated ECS cluster name. No service is created by this foundation."
  value       = aws_ecs_cluster.controller.name
}

output "controller_execution_role_arn" {
  description = "Dedicated ECS task execution role ARN."
  value       = aws_iam_role.execution.arn
}

output "controller_runtime_role_arn" {
  description = "Dedicated controller runtime role ARN."
  value       = module.controller_iam.role_arn
}

output "runner_instance_profile_arn" {
  description = "Dedicated future runner instance profile ARN."
  value       = module.runner_runtime.instance_profile_arn
}

output "state_table_name" {
  description = "Dedicated controller state table name."
  value       = module.controller_state.table_name
}

output "controller_security_group_id" {
  description = "Ingress-free controller security group."
  value       = aws_security_group.controller.id
}

output "runner_security_group_id" {
  description = "Dedicated ingress-free runner security group."
  value       = module.runner_network.runner_security_group_id
}
