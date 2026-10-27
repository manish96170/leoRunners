data "aws_partition" "current" {}

data "aws_caller_identity" "current" {}

locals {
  common_tags = merge(
    {
      ManagedBy       = "leo-runners"
      System          = "leo-runners"
      Environment     = "uat"
      Component       = "foundation"
      Owner           = var.owner
      Repository      = var.repository
      RunnerGroup     = var.runner_group
      RequestId       = var.request_id
      CleanupDeadline = var.cleanup_deadline
    },
    var.tags,
  )
}

resource "aws_ecr_repository" "controller" {
  name                 = "${var.name_prefix}-controller"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = merge(local.common_tags, { Component = "image-registry" })
}

resource "aws_ecr_lifecycle_policy" "controller" {
  repository = aws_ecr_repository.controller.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep the ten most recent untagged controller images."
      selection = {
        tagStatus   = "untagged"
        countType   = "imageCountMoreThan"
        countNumber = 10
      }
      action = { type = "expire" }
    }]
  })
}

resource "aws_cloudwatch_log_group" "controller" {
  name              = "/ecs/${var.name_prefix}-controller"
  retention_in_days = var.log_retention_days
  skip_destroy      = false
  tags              = merge(local.common_tags, { Component = "controller-logs" })
}

resource "aws_ecs_cluster" "controller" {
  name = var.name_prefix

  setting {
    name  = "containerInsights"
    value = "disabled"
  }

  tags = merge(local.common_tags, { Component = "controller-cluster" })
}

resource "aws_iam_role" "execution" {
  name        = "${var.name_prefix}-execution"
  description = "ECS execution role for the isolated LeoRunners UAT controller."

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.${data.aws_partition.current.dns_suffix}" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, { Component = "controller-execution-role" })
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

module "controller_state" {
  source = "../../modules/controller-state"

  table_name                    = "${var.name_prefix}-state"
  enable_point_in_time_recovery = var.enable_point_in_time_recovery
  enable_deletion_protection    = false
  tags                          = merge(local.common_tags, { Component = "controller-state" })
}

module "controller_iam" {
  source = "../../modules/controller-iam"

  role_name                  = "${var.name_prefix}-controller"
  trusted_service_principals = ["ecs-tasks.${data.aws_partition.current.dns_suffix}"]
  generated_policy_json      = var.controller_generated_policy_json
  tags                       = merge(local.common_tags, { Component = "controller-runtime-role" })
}

module "runner_runtime" {
  source = "../../modules/runner-runtime"

  name       = "${var.name_prefix}-runner"
  enable_ssm = false
  tags       = merge(local.common_tags, { Component = "runner-runtime-role" })
}

resource "aws_security_group" "controller" {
  name        = "${var.name_prefix}-controller"
  description = "Ingress-free foundation security group for the LeoRunners UAT controller."
  vpc_id      = var.controller_vpc_id

  egress {
    description = "HTTPS egress for GitHub, AWS APIs, image pulls, and approved package endpoints."
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description = "DNS UDP resolution within the controller VPC."
    from_port   = 53
    to_port     = 53
    protocol    = "udp"
    cidr_blocks = [var.controller_vpc_cidr]
  }

  egress {
    description = "DNS TCP resolution within the controller VPC."
    from_port   = 53
    to_port     = 53
    protocol    = "tcp"
    cidr_blocks = [var.controller_vpc_cidr]
  }

  tags = merge(local.common_tags, { Component = "controller-security-group" })
}

module "runner_network" {
  source = "../../modules/runner-network"

  name                         = var.name_prefix
  vpc_id                       = var.runner_vpc_id
  vpc_cidr_block               = var.runner_vpc_cidr
  runner_subnet_ids            = var.runner_subnet_ids
  create_runner_security_group = true
  runner_security_group_name   = "${var.name_prefix}-runner"
  egress_mode                  = "nat"
  tags                         = merge(local.common_tags, { Component = "runner-security-group" })
}
