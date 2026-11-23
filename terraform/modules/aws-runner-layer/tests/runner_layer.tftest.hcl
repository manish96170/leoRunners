mock_provider "aws" {
  mock_data "aws_partition" {
    defaults = {
      partition  = "aws"
      dns_suffix = "amazonaws.com"
    }
  }

  mock_data "aws_iam_policy" {
    defaults = {
      arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/leo-test-runner"
    }
  }

  mock_resource "aws_iam_instance_profile" {
    defaults = {
      arn = "arn:aws:iam::111122223333:instance-profile/leo-test-runner"
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
      latest_version = 3
    }
  }
}

variables {
  name_prefix              = "leo-test"
  ami_id                   = "ami-0123456789abcdef0"
  instance_type            = "c7i.xlarge"
  subnet_id                = "subnet-0123456789abcdef0"
  vpc_id                   = "vpc-0123456789abcdef0"
  egress_https_cidr_blocks = ["10.0.0.0/8"]
}

run "launch_template_carries_the_fixed_security_settings" {
  command = apply

  assert {
    condition = (
      aws_launch_template.runner.metadata_options[0].http_tokens == "required" &&
      aws_launch_template.runner.metadata_options[0].http_put_response_hop_limit == 1 &&
      aws_launch_template.runner.metadata_options[0].instance_metadata_tags == "disabled" &&
      aws_launch_template.runner.instance_initiated_shutdown_behavior == "terminate"
    )
    error_message = "IMDSv2 required, hop limit 1, no instance metadata tags and terminate-on-shutdown are not optional."
  }

  assert {
    condition = (
      aws_launch_template.runner.block_device_mappings[0].ebs[0].encrypted == "true" &&
      aws_launch_template.runner.block_device_mappings[0].ebs[0].volume_type == "gp3" &&
      aws_launch_template.runner.block_device_mappings[0].ebs[0].delete_on_termination == "true"
    )
    error_message = "The root volume must be encrypted gp3 and deleted with the instance."
  }

  assert {
    condition = (
      aws_launch_template.runner.network_interfaces[0].associate_public_ip_address == "false" &&
      aws_launch_template.runner.network_interfaces[0].subnet_id == "subnet-0123456789abcdef0"
    )
    error_message = "The template must own the network interface and never assign a public address."
  }

  assert {
    condition     = aws_launch_template.runner.user_data == null
    error_message = "The template must never hold user data: the controller adds it, with the one-time JIT configuration, at launch."
  }
}

run "every_resource_type_carries_the_ownership_tag" {
  command = apply

  variables {
    request_id = "req-1"
    tags       = { Environment = "test" }
  }

  assert {
    condition = alltrue([
      for spec in aws_launch_template.runner.tag_specifications :
      spec.tags["leo-runners:managed-by"] == "leo-runners" && spec.tags["RequestId"] == "req-1" && spec.tags["Environment"] == "test"
    ])
    error_message = "Instances, volumes and network interfaces must all carry the ownership tag, the request ID and the caller's tags."
  }

  assert {
    condition     = toset([for spec in aws_launch_template.runner.tag_specifications : spec.resource_type]) == toset(["instance", "volume", "network-interface"])
    error_message = "The launch template must tag the instance, its volume and its network interface."
  }

  assert {
    condition     = output.cleanup_selector.required_tags["leo-runners:managed-by"] == "leo-runners" && output.cleanup_selector.required_tags["RequestId"] == "req-1"
    error_message = "The cleanup selector must be the ownership tag and the request ID."
  }
}

run "controller_runner_is_ready_to_pass_to_the_controller_module" {
  command = apply

  variables {
    request_id       = "req-1"
    cleanup_deadline = "2030-01-01T00:00:00Z"
    tags             = { Environment = "test" }
  }

  assert {
    condition = (
      output.controller_runner.enabled == true &&
      output.controller_runner.launch_template_id == "lt-0123456789abcdef0" &&
      output.controller_runner.launch_template_version == "3" &&
      output.controller_runner.role_arn == "arn:aws:iam::111122223333:role/leo-test-runner" &&
      output.controller_runner.instance_profile_arn == "arn:aws:iam::111122223333:instance-profile/leo-test-runner"
    )
    error_message = "controller_runner must carry the template, an exact version, the role and the profile."
  }

  assert {
    condition     = output.controller_runner.subnet_id == null && length(output.controller_runner.security_group_ids) == 0
    error_message = "The controller must not be given a subnet or security groups: EC2 refuses them next to a template network interface."
  }

  assert {
    condition     = output.controller_runner.max_runners == 1 && output.controller_runner.max_lifetime_minutes == 120
    error_message = "A new deployment starts with one runner and a two hour lifetime."
  }

  assert {
    condition = (
      length(output.controller_runner.tags) == 3 &&
      output.controller_runner.tags["Environment"] == "test" &&
      output.controller_runner.tags["RequestId"] == "req-1" &&
      output.controller_runner.tags["CleanupDeadline"] == "2030-01-01T00:00:00Z"
    )
    error_message = "The controller must be given the deployment's distinguishing tags, and only those, so its reaper backstop never touches another deployment's runners."
  }

  assert {
    condition     = length(split("{{GITHUB_JIT_CONFIG_B64}}", output.controller_runner.user_data)) == 2
    error_message = "The bundled bootstrap must contain the JIT placeholder exactly once."
  }
}

run "caller_supplied_user_data_is_used" {
  command = apply

  variables {
    user_data = "#!/bin/bash\nJIT={{GITHUB_JIT_CONFIG_B64}}\n"
  }

  assert {
    condition     = output.controller_runner.user_data == "#!/bin/bash\nJIT={{GITHUB_JIT_CONFIG_B64}}\n"
    error_message = "user_data must replace the bundled bootstrap."
  }
}

run "runner_role_trusts_only_ec2_and_starts_empty" {
  command = apply

  assert {
    condition     = jsondecode(aws_iam_role.runner.assume_role_policy).Statement[0].Principal.Service == "ec2.amazonaws.com"
    error_message = "Only EC2 may assume the runner role."
  }

  assert {
    condition     = length(aws_iam_role_policy_attachment.managed) == 0 && length(aws_iam_role_policy_attachment.ssm_core) == 0 && length(aws_iam_role_policy.workload) == 0
    error_message = "The runner role must start with no permissions."
  }
}

run "optional_permissions_are_attached_when_asked" {
  command = apply

  variables {
    enable_ssm          = true
    managed_policy_arns = ["arn:aws:iam::111122223333:policy/example-readonly"]
    role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = ["arn:aws:s3:::example-bucket/*"]
      }]
    })
  }

  assert {
    condition     = length(aws_iam_role_policy_attachment.managed) == 1 && length(aws_iam_role_policy_attachment.ssm_core) == 1 && length(aws_iam_role_policy.workload) == 1
    error_message = "SSM, managed policies and the inline policy must be attached when requested."
  }
}

run "security_group_has_no_ingress_and_only_requested_egress" {
  command = apply

  variables {
    egress_https_cidr_blocks     = ["10.0.0.0/8", "172.16.0.0/12"]
    egress_https_prefix_list_ids = ["pl-0123456789abcdef0"]
    egress_dns_cidr_blocks       = ["10.0.0.2/32"]
  }

  assert {
    condition = (
      length(aws_vpc_security_group_egress_rule.https_cidr) == 2 &&
      length(aws_vpc_security_group_egress_rule.https_prefix_list) == 1 &&
      length(aws_vpc_security_group_egress_rule.dns_udp) == 1 &&
      length(aws_vpc_security_group_egress_rule.dns_tcp) == 1
    )
    error_message = "Exactly the requested egress rules must be created."
  }

  assert {
    condition     = length(aws_security_group.runner[0].ingress) == 0
    error_message = "Runners must never accept inbound traffic."
  }
}

run "existing_security_groups_can_replace_the_created_one" {
  command = apply

  variables {
    create_security_group = false
    vpc_id                = null
    security_group_ids    = ["sg-0fedcba9876543210"]
  }

  assert {
    condition     = length(aws_security_group.runner) == 0 && output.security_group_id == null && length(output.security_group_ids) == 1 && output.security_group_ids[0] == "sg-0fedcba9876543210"
    error_message = "Only the supplied security groups must be attached."
  }
}

run "created_security_group_needs_a_vpc" {
  command = plan

  variables {
    vpc_id = null
  }

  expect_failures = [terraform_data.guard]
}

run "created_security_group_needs_egress" {
  command = plan

  variables {
    egress_https_cidr_blocks = []
  }

  expect_failures = [terraform_data.guard]
}

run "allow_all_actions_are_rejected" {
  command = plan

  variables {
    role_policy_json = jsonencode({
      Version   = "2012-10-17"
      Statement = [{ Effect = "Allow", Action = "*", Resource = "*" }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "not_action_allow_is_rejected" {
  command = plan

  variables {
    role_policy_json = jsonencode({
      Version   = "2012-10-17"
      Statement = [{ Effect = "Allow", NotAction = ["iam:*"], Resource = "*" }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "wildcard_passrole_is_rejected" {
  command = plan

  variables {
    role_policy_json = jsonencode({
      Version   = "2012-10-17"
      Statement = [{ Effect = "Allow", Action = ["iam:PassRole"], Resource = ["arn:aws:iam::111122223333:role/*"] }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "policy_without_a_version_is_rejected" {
  command = plan

  variables {
    role_policy_json = jsonencode({
      Statement = [{ Effect = "Allow", Action = ["s3:GetObject"], Resource = ["arn:aws:s3:::example-bucket/*"] }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "reserved_tag_prefix_is_rejected" {
  command = plan

  variables {
    tags = { "leo-runners:managed-by" = "x" }
  }

  expect_failures = [var.tags]
}

run "tags_that_would_break_the_controller_setting_are_rejected" {
  command = plan

  variables {
    tags = { Owner = "a,b" }
  }

  expect_failures = [var.tags]
}

run "user_data_needs_exactly_one_placeholder" {
  command = plan

  variables {
    user_data = "#!/bin/bash\necho hello\n"
  }

  expect_failures = [var.user_data]
}

run "bad_ami_is_rejected" {
  command = plan

  variables {
    ami_id = "not-an-ami"
  }

  expect_failures = [var.ami_id]
}

run "lifetime_is_bounded" {
  command = plan

  variables {
    max_lifetime_minutes = 1
  }

  expect_failures = [var.max_lifetime_minutes]
}
