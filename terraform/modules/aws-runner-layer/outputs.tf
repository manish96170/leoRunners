output "launch_template_id" {
  description = "ID of the runner launch template."
  value       = aws_launch_template.runner.id
}

output "launch_template_arn" {
  description = "ARN of the runner launch template, for a reviewed controller policy."
  value       = aws_launch_template.runner.arn
}

output "launch_template_version" {
  description = "Exact launch template version the controller should use. Changing the template creates a new version and changes this value."
  value       = tostring(aws_launch_template.runner.latest_version)
}

output "role_arn" {
  description = "ARN of the runner IAM role. The controller's iam:PassRole must name exactly this."
  value       = aws_iam_role.runner.arn
}

output "role_name" {
  description = "Name of the runner IAM role."
  value       = aws_iam_role.runner.name
}

output "instance_profile_arn" {
  description = "ARN of the runner instance profile."
  value       = aws_iam_instance_profile.runner.arn
}

output "instance_profile_name" {
  description = "Name of the runner instance profile."
  value       = aws_iam_instance_profile.runner.name
}

output "security_group_id" {
  description = "ID of the created runner security group, or null when create_security_group is false."
  value       = var.create_security_group ? aws_security_group.runner[0].id : null
}

output "security_group_ids" {
  description = "Every security group attached to runners."
  value       = local.security_group_ids
}

output "subnet_id" {
  description = "The subnet runners launch into."
  value       = var.subnet_id
}

output "ownership_tags" {
  description = "Tags on every runner instance and volume."
  value       = module.contract.ownership_tags
}

output "cleanup_selector" {
  description = "Tag intersection that identifies exactly this request's runners. Null when request_id is unset. Never discover runners by name, VPC, subnet or instance type."
  value       = module.contract.cleanup_selector
}

output "controller_policy_constraints" {
  description = "Checklist for the reviewed controller policy: iam:PassRole limited to exactly the runner role, RunInstances and terminate conditioned on the ownership tag."
  value       = module.contract.controller_policy_constraints
}

output "user_data" {
  description = "The bootstrap script the controller adds at launch (the bundled one unless user_data was set)."
  value       = local.user_data
}

output "controller_runner" {
  description = <<-EOT
    Ready-made `runner` object for the aws-ecs-controller module. Add the
    labels your workflows will use, and the pool, if you do not want the
    defaults:

      runner = merge(module.runner_layer.controller_runner, { labels = ["self-hosted", "linux"] })

    subnet_id and security_group_ids are deliberately null: the launch template
    defines the network interface, and EC2 refuses a second set in the launch
    request. max_runners is 1 until you raise it.
  EOT
  value = {
    enabled                 = true
    launch_template_id      = aws_launch_template.runner.id
    launch_template_version = tostring(aws_launch_template.runner.latest_version)
    subnet_id               = null
    security_group_ids      = []
    instance_profile_arn    = aws_iam_instance_profile.runner.arn
    role_arn                = aws_iam_role.runner.arn
    instance_type           = null
    pool_id                 = "default"
    labels                  = []
    user_data               = local.user_data
    max_lifetime_minutes    = var.max_lifetime_minutes
    max_runners             = 1
    tags                    = local.deployment_tags
  }
}
