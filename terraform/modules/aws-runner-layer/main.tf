data "aws_partition" "current" {}

data "aws_iam_policy" "ssm_core" {
  count = var.enable_ssm ? 1 : 0
  name  = "AmazonSSMManagedInstanceCore"
}

locals {
  role_name = var.role_name != null ? var.role_name : "${var.name_prefix}-runner"

  # Tags that tell one deployment's runners from another's. The controller is
  # given the same set, so every launch carries them whatever the template says,
  # and its reaper backstop only considers instances that carry all of them.
  deployment_tags = merge(
    var.tags,
    var.request_id == null ? {} : { RequestId = var.request_id },
    var.cleanup_deadline == null ? {} : { CleanupDeadline = var.cleanup_deadline },
  )

  security_group_ids = concat(var.security_group_ids, var.create_security_group ? [aws_security_group.runner[0].id] : [])

  user_data = var.user_data != null ? var.user_data : file("${path.module}/../../../bootstrap/runner/user-data.sh")

  common_tags = merge(
    {
      "managed-by" = "terraform"
      "component"  = "leo-runners-runner-layer"
    },
    var.tags,
  )

  # ---------------------------------------------------------------------------
  # Inline policy inspection. Terraform cannot prove a policy is least
  # privilege; it only rejects the shapes that are never acceptable on a role
  # that runs pull-request code.
  # ---------------------------------------------------------------------------
  policy_doc   = var.role_policy_json == null ? null : try(jsondecode(var.role_policy_json), null)
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
}

# The contract module is the single source of truth for the ownership tag, the
# fixed security settings, the pass-role constraint and the cleanup selector.
# It creates nothing; this module implements it.
module "contract" {
  source = "../aws-ephemeral-runner"

  name_prefix                 = var.name_prefix
  ami_id                      = var.ami_id
  instance_types              = [var.instance_type]
  subnet_ids                  = [var.subnet_id]
  security_group_ids          = local.security_group_ids
  runner_role_arn             = aws_iam_role.runner.arn
  runner_instance_profile_arn = aws_iam_instance_profile.runner.arn
  controller_role_arn         = var.controller_role_arn

  root_device_name             = var.root_device_name
  root_volume_size_gb          = var.root_volume_size_gb
  root_volume_iops             = var.root_volume_iops
  root_volume_throughput_mibps = var.root_volume_throughput_mibps
  root_volume_kms_key_arn      = var.root_volume_kms_key_arn
  max_lifetime_minutes         = var.max_lifetime_minutes
  request_id                   = var.request_id
  cleanup_deadline             = var.cleanup_deadline
  tags                         = var.tags
}

resource "terraform_data" "guard" {
  input = "runner-layer"

  lifecycle {
    precondition {
      condition     = !var.create_security_group || var.vpc_id != null
      error_message = "create_security_group = true requires vpc_id."
    }

    precondition {
      condition     = length(var.security_group_ids) > 0 || var.create_security_group
      error_message = "Runners need a security group: set create_security_group = true or provide security_group_ids."
    }

    precondition {
      condition     = !var.create_security_group || length(var.egress_https_cidr_blocks) > 0 || length(var.egress_https_prefix_list_ids) > 0
      error_message = "The created security group allows no egress by default, so runners could not reach GitHub. Set egress_https_cidr_blocks or egress_https_prefix_list_ids."
    }

    precondition {
      condition     = var.role_policy_json == null || (local.policy_valid && length(local.policy_statements) > 0 && alltrue([for s in local.policy_statements : contains(["Allow", "Deny"], s.effect)]))
      error_message = "role_policy_json must be a valid IAM policy document (Version 2012-10-17) with at least one statement, each with Effect Allow or Deny."
    }

    precondition {
      condition     = !local.policy_has_allow_not_action
      error_message = "role_policy_json must not contain an Allow statement that uses NotAction."
    }

    precondition {
      condition     = !local.policy_has_broad_action
      error_message = "role_policy_json must not allow Action \"*\" or wildcard iam: actions."
    }

    precondition {
      condition     = !local.policy_has_wildcard_passrole
      error_message = "role_policy_json must not allow iam:PassRole on a Resource containing a wildcard."
    }
  }
}

###############################################################################
# Runner identity: EC2 may assume it, and it starts with no permissions.
###############################################################################

resource "aws_iam_role" "runner" {
  name                 = local.role_name
  path                 = var.iam_path
  permissions_boundary = var.permissions_boundary_arn
  description          = "Role for ephemeral CI runner instances. Grant only what workflows need."

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "Ec2Only"
      Effect    = "Allow"
      Principal = { Service = "ec2.${data.aws_partition.current.dns_suffix}" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "managed" {
  for_each = toset(var.managed_policy_arns)

  role       = aws_iam_role.runner.name
  policy_arn = each.value
}

resource "aws_iam_role_policy_attachment" "ssm_core" {
  count = var.enable_ssm ? 1 : 0

  role       = aws_iam_role.runner.name
  policy_arn = data.aws_iam_policy.ssm_core[0].arn
}

resource "aws_iam_role_policy" "workload" {
  count = var.role_policy_json != null ? 1 : 0

  name   = "${local.role_name}-workload"
  role   = aws_iam_role.runner.id
  policy = var.role_policy_json
}

resource "aws_iam_instance_profile" "runner" {
  name = local.role_name
  path = var.iam_path
  role = aws_iam_role.runner.name
  tags = local.common_tags
}

###############################################################################
# Runner network: no ingress, ever. Egress is exactly what the caller lists.
###############################################################################

resource "aws_security_group" "runner" {
  count = var.create_security_group ? 1 : 0

  name        = "${var.name_prefix}-runner"
  description = "Ephemeral CI runners. No inbound traffic."
  vpc_id      = var.vpc_id

  tags = merge(local.common_tags, { Name = "${var.name_prefix}-runner" })
}

resource "aws_vpc_security_group_egress_rule" "https_cidr" {
  for_each = var.create_security_group ? toset(var.egress_https_cidr_blocks) : toset([])

  security_group_id = aws_security_group.runner[0].id
  cidr_ipv4         = each.value
  description       = "HTTPS egress to ${each.value}."
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_egress_rule" "https_prefix_list" {
  for_each = var.create_security_group ? toset(var.egress_https_prefix_list_ids) : toset([])

  security_group_id = aws_security_group.runner[0].id
  prefix_list_id    = each.value
  description       = "HTTPS egress to prefix list ${each.value}."
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_egress_rule" "dns_udp" {
  for_each = var.create_security_group ? toset(var.egress_dns_cidr_blocks) : toset([])

  security_group_id = aws_security_group.runner[0].id
  cidr_ipv4         = each.value
  description       = "DNS (UDP) egress to ${each.value}."
  from_port         = 53
  to_port           = 53
  ip_protocol       = "udp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_egress_rule" "dns_tcp" {
  for_each = var.create_security_group ? toset(var.egress_dns_cidr_blocks) : toset([])

  security_group_id = aws_security_group.runner[0].id
  cidr_ipv4         = each.value
  description       = "DNS (TCP) egress to ${each.value}."
  from_port         = 53
  to_port           = 53
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

###############################################################################
# Launch template: the security-relevant settings come from the contract.
###############################################################################

resource "aws_launch_template" "runner" {
  name        = "${var.name_prefix}-runner"
  description = "Ephemeral CI runner. The controller supplies user data and per-job tags at launch."

  image_id                             = module.contract.launch_template_spec.image_id
  instance_type                        = var.instance_type
  instance_initiated_shutdown_behavior = module.contract.launch_template_spec.instance_initiated_shutdown_behavior

  iam_instance_profile {
    arn = module.contract.launch_template_spec.iam_instance_profile_arn
  }

  metadata_options {
    http_endpoint               = module.contract.launch_template_spec.metadata_options.http_endpoint
    http_tokens                 = module.contract.launch_template_spec.metadata_options.http_tokens
    http_put_response_hop_limit = module.contract.launch_template_spec.metadata_options.http_put_response_hop_limit
    instance_metadata_tags      = module.contract.launch_template_spec.metadata_options.instance_metadata_tags
  }

  block_device_mappings {
    device_name = module.contract.launch_template_spec.root_block_device.device_name

    ebs {
      volume_type           = module.contract.launch_template_spec.root_block_device.volume_type
      volume_size           = module.contract.launch_template_spec.root_block_device.volume_size_gb
      iops                  = module.contract.launch_template_spec.root_block_device.iops
      throughput            = module.contract.launch_template_spec.root_block_device.throughput_mibps
      encrypted             = module.contract.launch_template_spec.root_block_device.encrypted
      kms_key_id            = module.contract.launch_template_spec.root_block_device.kms_key_id
      delete_on_termination = module.contract.launch_template_spec.root_block_device.delete_on_termination
    }
  }

  # The template owns the network interface so a public address is impossible
  # whatever the subnet's auto-assign setting says. EC2 does not allow a
  # network interface here together with a subnet or security groups in the
  # launch request, so the controller is given neither.
  network_interfaces {
    device_index                = 0
    associate_public_ip_address = module.contract.launch_template_spec.associate_public_ip_address
    subnet_id                   = var.subnet_id
    security_groups             = local.security_group_ids
    delete_on_termination       = true
  }

  tag_specifications {
    resource_type = "instance"
    tags          = merge(module.contract.launch_template_spec.tag_specifications.instance, { Name = "${var.name_prefix}-runner" })
  }

  tag_specifications {
    resource_type = "volume"
    tags          = module.contract.launch_template_spec.tag_specifications.volume
  }

  tag_specifications {
    resource_type = "network-interface"
    tags          = module.contract.launch_template_spec.tag_specifications.volume
  }

  tags = local.common_tags

  depends_on = [terraform_data.guard]
}
