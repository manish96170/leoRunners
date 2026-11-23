# Example customer composition. A real deployment copies this into the
# customer's own infrastructure repository and replaces the relative module
# sources with an immutable Git source, for example:
#
#   source = "git::https://github.com/manish96170/leoRunners.git//terraform/modules/aws-ecs-controller?ref=tf-aws-v0.1.0"
#
# Nothing here is applied by the LeoRunners repository.

provider "aws" {
  region = var.region
}

module "gateway" {
  source = "../../modules/aws-shared-gateway"

  alb_arn                   = var.alb_arn
  alb_security_group_id     = var.alb_security_group_id
  certificate_arn           = var.certificate_arn
  enable_https_listener     = var.enable_gateway
  https_ingress_cidr_blocks = var.https_ingress_cidr_blocks
  tags                      = var.tags

  listener_rule_priority_ranges = {
    controller = { start = 100, end = 199 }
  }
}

# Optional runner layer: the launch template, runner role and instance profile,
# and an ingress-free security group. It is a separate module so the controller
# can be deployed and proven first. It takes the controller's task role only to
# check the two stay distinct and in one account; the controller's own inputs
# depend on the layer, not the other way round.
module "runner_layer" {
  count  = var.runner_layer == null ? 0 : 1
  source = "../../modules/aws-runner-layer"

  name_prefix              = var.name_prefix
  ami_id                   = var.runner_layer.ami_id
  instance_type            = var.runner_layer.instance_type
  vpc_id                   = var.runner_layer.vpc_id
  subnet_id                = var.runner_layer.subnet_id
  egress_https_cidr_blocks = var.runner_layer.egress_https_cidr_blocks
  request_id               = var.runner_layer.request_id
  cleanup_deadline         = var.runner_layer.cleanup_deadline
  controller_role_arn      = module.controller.task_role_arn
  tags                     = var.tags
}

locals {
  # The controller may pass exactly the runner role to EC2 and nothing else.
  # The reviewed policy is the caller's; this appends the one statement the
  # runner lifecycle needs, scoped to the layer's role.
  runner_passrole_statement = var.runner_layer == null ? [] : [{
    Effect    = "Allow"
    Action    = ["iam:PassRole"]
    Resource  = [module.runner_layer[0].role_arn]
    Condition = { StringEquals = { "iam:PassedToService" = "ec2.amazonaws.com" } }
  }]

  task_role_policy_json = var.runner_layer == null || var.task_role_policy_json == null ? var.task_role_policy_json : jsonencode({
    Version   = "2012-10-17"
    Statement = concat(jsondecode(var.task_role_policy_json).Statement, local.runner_passrole_statement)
  })
}

module "controller" {
  source = "../../modules/aws-ecs-controller"

  name_prefix              = var.name_prefix
  create_foundation        = var.create_foundation
  enable_listener_rule     = var.enable_listener_rule
  enable_service           = var.enable_service
  tags                     = var.tags
  vpc_id                   = var.vpc_id
  subnet_ids               = var.subnet_ids
  alb_security_group_id    = var.alb_security_group_id
  egress_https_cidr_blocks = var.egress_https_cidr_blocks
  create_cluster           = true

  listener_arn           = module.gateway.https_listener_arn
  listener_path_patterns = [var.webhook_path]
  listener_priority      = module.gateway.listener_rule_priority_ranges["controller"].start

  image_digest          = var.image_digest
  secrets_ready         = var.secrets_ready
  task_role_policy_json = local.task_role_policy_json
  github                = var.github

  # With the layer: its ready-made runner object plus the labels workflows use.
  # Without it: {} (lifecycle off). merge() over a list keeps both branches the
  # same type.
  runner = merge(concat([{}], [for layer in module.runner_layer : merge(layer.controller_runner, { labels = var.runner_layer.labels })])...)
}
