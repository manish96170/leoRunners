mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "111122223333"
    }
  }

  mock_data "aws_region" {
    defaults = {
      region = "us-east-1"
    }
  }

  mock_data "aws_partition" {
    defaults = {
      partition  = "aws"
      dns_suffix = "amazonaws.com"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/mock-role"
    }
  }

  mock_resource "aws_iam_instance_profile" {
    defaults = {
      arn = "arn:aws:iam::111122223333:instance-profile/mock-profile"
    }
  }

  mock_resource "aws_lb_listener" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    }
  }

  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:us-east-1:111122223333:targetgroup/example/0123456789abcdef"
    }
  }

  mock_resource "aws_security_group" {
    defaults = {
      id = "sg-0123456789abcdef0"
    }
  }

  mock_resource "aws_launch_template" {
    defaults = {
      id             = "lt-0123456789abcdef0"
      arn            = "arn:aws:ec2:us-east-1:111122223333:launch-template/lt-0123456789abcdef0"
      latest_version = 1
    }
  }
}

variables {
  region                    = "us-east-1"
  vpc_id                    = "vpc-0123456789abcdef0"
  subnet_ids                = ["subnet-0123456789abcdef0"]
  alb_arn                   = "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/app/example/0123456789abcdef"
  alb_security_group_id     = "sg-0123456789abcdef0"
  certificate_arn           = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"
  https_ingress_cidr_blocks = ["192.0.2.0/24"]
  egress_https_cidr_blocks  = ["10.0.0.0/8"]
}

run "defaults_plan_as_a_no_op" {
  command = plan

  assert {
    condition     = output.phase == "disabled"
    error_message = "The example must be inert until a phase flag is set."
  }
}

run "all_phases_in_one_plan_wire_gateway_to_controller" {
  command = plan

  variables {
    enable_gateway       = true
    create_foundation    = true
    enable_listener_rule = true
    enable_service       = true
    image_digest         = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    secrets_ready        = true
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem"]
        Resource = ["arn:aws:dynamodb:us-east-1:111122223333:table/leo-runners-state"]
      }]
    })
  }

  assert {
    condition     = output.phase == "service"
    error_message = "A listener created in the same plan must satisfy the controller's listener input."
  }
}

run "runner_layer_composes_with_the_controller_without_a_cycle" {
  command = apply

  override_resource {
    target = module.controller.aws_iam_role.task[0]
    values = {
      arn = "arn:aws:iam::111122223333:role/controller-task"
    }
  }

  override_resource {
    target = module.runner_layer[0].aws_iam_role.runner
    values = {
      arn = "arn:aws:iam::111122223333:role/runner"
    }
  }

  variables {
    enable_gateway       = true
    create_foundation    = true
    enable_listener_rule = true
    enable_service       = true
    image_digest         = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    secrets_ready        = true
    github = {
      jit_enabled           = true
      runner_group_id       = 5
      runner_scope          = "organization"
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted", "linux"]
      app = {
        app_id            = "12345"
        installation_id   = 678
        token_permissions = { organization_self_hosted_runners = "write" }
      }
    }
    runner_layer = {
      ami_id                   = "ami-0123456789abcdef0"
      instance_type            = "c7i.xlarge"
      vpc_id                   = "vpc-0123456789abcdef0"
      subnet_id                = "subnet-0123456789abcdef0"
      egress_https_cidr_blocks = ["10.0.0.0/8"]
      labels                   = ["self-hosted", "linux"]
      request_id               = "req-1"
    }
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem"]
        Resource = ["arn:aws:dynamodb:us-east-1:111122223333:table/leo-runners-state"]
      }]
    })
  }

  assert {
    condition     = output.phase == "service" && output.runner_layer != null
    error_message = "Layer, gateway and controller must compose in one plan."
  }

  assert {
    condition = (
      module.controller.container_environment["AWS_LAUNCH_TEMPLATE_ID"] == "lt-0123456789abcdef0" &&
      module.controller.container_environment["AWS_LAUNCH_TEMPLATE_VERSION"] == "1" &&
      module.controller.container_environment["CAPACITY_LABELS"] == "self-hosted,linux" &&
      module.controller.container_environment["CAPACITY_MAX_RUNNERS"] == "1" &&
      module.controller.container_environment["GITHUB_RUNNER_SCOPE"] == "organization" &&
      module.controller.container_environment["AWS_RUNNER_TAGS"] == "RequestId=req-1"
    )
    error_message = "The layer's outputs must reach the controller's environment."
  }

  assert {
    condition     = !contains(keys(module.controller.container_environment), "AWS_SUBNET_ID") && !contains(keys(module.controller.container_environment), "AWS_SECURITY_GROUP_IDS")
    error_message = "The controller must not be given a subnet or security groups next to a template network interface."
  }
}
