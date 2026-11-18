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
  task_role_policy_json = var.task_role_policy_json
  github                = var.github
}
