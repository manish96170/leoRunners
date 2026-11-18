mock_provider "aws" {}

variables {
  alb_arn               = "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/app/example/0123456789abcdef"
  alb_security_group_id = "sg-0123456789abcdef0"
}

run "inert_by_default" {
  command = plan

  assert {
    condition     = length(aws_lb_listener.https) == 0 && length(aws_lb_listener.http_redirect) == 0
    error_message = "With no flags the module must create no listener."
  }

  assert {
    condition     = output.https_listener_arn == null
    error_message = "No listener ARN is published when none is configured."
  }
}

run "https_listener_requires_certificate" {
  command = plan

  variables {
    enable_https_listener     = true
    https_ingress_cidr_blocks = ["0.0.0.0/0"]
  }

  expect_failures = [terraform_data.guard]
}

run "https_listener_requires_explicit_ingress_sources" {
  command = plan

  variables {
    enable_https_listener = true
    certificate_arn       = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"
  }

  expect_failures = [terraform_data.guard]
}

run "https_listener_with_fixed_404" {
  command = plan

  variables {
    enable_https_listener     = true
    certificate_arn           = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"
    https_ingress_cidr_blocks = ["192.0.2.0/24", "198.51.100.0/24"]
  }

  assert {
    condition     = length(aws_lb_listener.https) == 1
    error_message = "The HTTPS listener must be created."
  }

  assert {
    condition     = aws_lb_listener.https[0].default_action[0].fixed_response[0].status_code == "404"
    error_message = "The default response must be a fixed 404."
  }

  assert {
    condition     = length(aws_vpc_security_group_ingress_rule.https_cidr) == 2
    error_message = "One ingress rule is expected per CIDR."
  }
}

run "adopting_an_existing_listener_creates_none" {
  command = plan

  variables {
    existing_https_listener_arn   = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    manage_security_group_ingress = false
  }

  assert {
    condition     = length(aws_lb_listener.https) == 0 && output.https_listener_arn == "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    error_message = "An adopted listener must be published without creating another."
  }
}

run "creating_and_adopting_are_exclusive" {
  command = plan

  variables {
    enable_https_listener         = true
    certificate_arn               = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"
    existing_https_listener_arn   = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    manage_security_group_ingress = false
  }

  expect_failures = [terraform_data.guard]
}

run "http_redirect_requires_https" {
  command = plan

  variables {
    enable_http_redirect          = true
    manage_security_group_ingress = false
  }

  expect_failures = [terraform_data.guard]
}

run "http_redirect_listener" {
  command = plan

  variables {
    enable_https_listener     = true
    enable_http_redirect      = true
    certificate_arn           = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"
    https_ingress_cidr_blocks = ["192.0.2.0/24"]
  }

  assert {
    condition     = length(aws_lb_listener.http_redirect) == 1 && length(aws_vpc_security_group_ingress_rule.http_redirect) == 1
    error_message = "The redirect listener and its ingress rule must be created."
  }
}

run "overlapping_priority_ranges_are_rejected" {
  command = plan

  variables {
    listener_rule_priority_ranges = {
      controller = { start = 100, end = 199 }
      other      = { start = 150, end = 250 }
    }
  }

  expect_failures = [var.listener_rule_priority_ranges]
}

run "disjoint_priority_ranges_are_published" {
  command = plan

  variables {
    listener_rule_priority_ranges = {
      controller = { start = 100, end = 199 }
      other      = { start = 200, end = 299 }
    }
  }

  assert {
    condition     = output.listener_rule_priority_ranges["controller"].start == 100
    error_message = "Valid ranges must be republished."
  }
}
