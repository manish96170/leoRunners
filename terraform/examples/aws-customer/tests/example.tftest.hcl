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
