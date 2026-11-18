mock_provider "aws" {
  mock_data "aws_partition" {
    defaults = {
      partition  = "aws"
      dns_suffix = "amazonaws.com"
    }
  }
}

variables {
  environment         = "example"
  owner               = "example-team"
  repository          = "example-org/example-repo"
  runner_group        = "leo-runners-example"
  controller_vpc_id   = "vpc-0123456789abcdef0"
  controller_vpc_cidr = "10.0.0.0/16"
  runner_vpc_id       = "vpc-0fedcba9876543210"
  runner_vpc_cidr     = "10.1.0.0/16"
  runner_subnet_ids   = ["subnet-0123456789abcdef0"]
}

run "foundation_is_gateway_free_by_default" {
  command = plan

  assert {
    condition     = output.gateway_https_listener_arn == null
    error_message = "The gateway layer must be off by default."
  }

  assert {
    condition     = aws_ecr_repository.controller.image_tag_mutability == "IMMUTABLE"
    error_message = "The controller image repository must have immutable tags."
  }
}

run "gateway_requires_all_of_its_inputs" {
  command = plan

  variables {
    enable_https_gateway = true
    gateway_alb_arn      = "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/app/example/0123456789abcdef"
  }

  expect_failures = [terraform_data.gateway_inputs]
}

run "gateway_adds_only_a_listener_and_ingress" {
  command = plan

  variables {
    enable_https_gateway              = true
    gateway_alb_arn                   = "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/app/example/0123456789abcdef"
    gateway_alb_security_group_id     = "sg-0123456789abcdef0"
    gateway_certificate_arn           = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"
    gateway_https_ingress_cidr_blocks = ["192.0.2.0/24"]
  }

  assert {
    condition     = length(module.gateway) == 1
    error_message = "Enabling the gateway must create the HTTPS listener."
  }
}

run "state_table_is_protected_by_default" {
  command = plan

  assert {
    condition     = module.controller_state.table_name == "leo-runners-state"
    error_message = "The state table name must follow the prefix."
  }
}

run "bad_prefix_is_rejected" {
  command = plan

  variables {
    name_prefix = "Bad_Prefix"
  }

  expect_failures = [var.name_prefix]
}
