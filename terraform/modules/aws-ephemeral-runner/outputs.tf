output "ownership_tags" {
  description = "Tags every runner instance and volume must carry. The controller adds per-job tags under the leo-runners: prefix (runner-id, job-id, repository, created-at, expires-at, and so on) at launch."
  value       = local.ownership_tags

  precondition {
    condition     = !anytrue([for k in keys(var.tags) : startswith(k, "leo-runners:")])
    error_message = "tags must not use the reserved leo-runners: prefix; the controller sets those keys."
  }
}

output "launch_template_spec" {
  description = "Security-relevant launch template arguments for the future runner layer. IMDSv2 required, encrypted gp3 root volume, terminate-on-shutdown, no public IP, dedicated instance profile."
  value = {
    name                                 = "${var.name_prefix}-runner"
    image_id                             = var.ami_id
    instance_types                       = var.instance_types
    subnet_ids                           = var.subnet_ids
    vpc_security_group_ids               = var.security_group_ids
    iam_instance_profile_arn             = var.runner_instance_profile_arn
    instance_initiated_shutdown_behavior = "terminate"
    associate_public_ip_address          = false
    metadata_options = {
      http_endpoint               = "enabled"
      http_tokens                 = "required"
      http_put_response_hop_limit = 1
      instance_metadata_tags      = "disabled"
    }
    root_block_device = {
      device_name           = var.root_device_name
      volume_type           = "gp3"
      volume_size_gb        = var.root_volume_size_gb
      iops                  = var.root_volume_iops
      throughput_mibps      = var.root_volume_throughput_mibps
      encrypted             = true
      kms_key_id            = var.root_volume_kms_key_arn
      delete_on_termination = true
    }
    tag_specifications = {
      instance = local.ownership_tags
      volume   = local.ownership_tags
    }
  }

  precondition {
    condition     = var.controller_role_arn == null || var.runner_role_arn != var.controller_role_arn
    error_message = "runner_role_arn must be a dedicated role, different from controller_role_arn."
  }

  precondition {
    condition     = length(local.account_ids) == 1
    error_message = "The runner role, instance profile, and controller role must belong to the same AWS account."
  }
}

output "max_lifetime_minutes" {
  description = "Hard runner lifetime enforced through the ExpiresAt tag and the reaper."
  value       = var.max_lifetime_minutes
}

output "controller_policy_constraints" {
  description = "Constraints a reviewed controller runtime policy must satisfy before runner lifecycle is enabled. Use them as the review checklist for IAM Policy Autopilot output; the module does not generate or attach permissions."
  value = {
    pass_role = {
      principal = var.controller_role_arn
      action    = "iam:PassRole"
      resources = [var.runner_role_arn]
      conditions = {
        "iam:PassedToService" = "ec2.amazonaws.com"
      }
    }
    run_instances = {
      action                     = "ec2:RunInstances"
      required_request_tags      = local.fixed_tags
      allowed_subnet_ids         = var.subnet_ids
      allowed_security_group_ids = var.security_group_ids
      allowed_image_id           = var.ami_id
    }
    terminate_and_describe = {
      actions                = ["ec2:TerminateInstances", "ec2:DescribeInstances"]
      required_resource_tags = local.fixed_tags
    }
    forbidden = [
      "iam:PassRole with a wildcard or role-prefix resource",
      "ec2:RunInstances without the ownership request tags",
      "ec2:TerminateInstances without the ownership resource tags",
    ]
  }
}

output "cleanup_selector" {
  description = "Tag intersection that identifies exactly the runners belonging to this request. Null when request_id is unset, in which case request-scoped cleanup is not available and nothing may be selected by name, VPC, subnet, or instance type."
  value = var.request_id == null ? null : {
    required_tags = merge(local.fixed_tags, { RequestId = var.request_id })
    match         = "all tags must match; never discover by name, VPC, subnet, or instance type"
  }
}
