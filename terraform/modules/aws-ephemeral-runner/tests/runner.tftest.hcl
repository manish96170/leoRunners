variables {
  name_prefix                 = "leo-test"
  ami_id                      = "ami-0123456789abcdef0"
  instance_types              = ["c7i.xlarge"]
  subnet_ids                  = ["subnet-0123456789abcdef0"]
  security_group_ids          = ["sg-0123456789abcdef0"]
  runner_role_arn             = "arn:aws:iam::111122223333:role/example-runner"
  runner_instance_profile_arn = "arn:aws:iam::111122223333:instance-profile/example-runner"
  controller_role_arn         = "arn:aws:iam::111122223333:role/example-controller"
}

run "contract_fixes_the_security_settings" {
  command = plan

  assert {
    condition = (
      output.launch_template_spec.metadata_options.http_tokens == "required" &&
      output.launch_template_spec.metadata_options.http_put_response_hop_limit == 1 &&
      output.launch_template_spec.root_block_device.encrypted == true &&
      output.launch_template_spec.root_block_device.volume_type == "gp3" &&
      output.launch_template_spec.associate_public_ip_address == false &&
      output.launch_template_spec.instance_initiated_shutdown_behavior == "terminate"
    )
    error_message = "IMDSv2, encrypted gp3, no public IP, and terminate-on-shutdown are not optional."
  }
}

run "passrole_is_scoped_to_the_exact_runner_role" {
  command = plan

  assert {
    condition = (
      length(output.controller_policy_constraints.pass_role.resources) == 1 &&
      output.controller_policy_constraints.pass_role.resources[0] == "arn:aws:iam::111122223333:role/example-runner" &&
      output.controller_policy_constraints.pass_role.conditions["iam:PassedToService"] == "ec2.amazonaws.com"
    )
    error_message = "PassRole must name exactly the runner role and be limited to EC2."
  }
}

run "request_scoped_cleanup_selector" {
  command = plan

  variables {
    request_id       = "change-1234"
    cleanup_deadline = "2030-01-01T00:00:00Z"
  }

  assert {
    condition     = output.cleanup_selector.required_tags.RequestId == "change-1234" && output.ownership_tags.CleanupDeadline == "2030-01-01T00:00:00Z"
    error_message = "The cleanup selector must be the ownership tag and RequestId intersection."
  }
}

run "no_request_id_means_no_cleanup_selector" {
  command = plan

  assert {
    condition     = output.cleanup_selector == null
    error_message = "Without a request ID there must be no selector to act on."
  }
}

run "runner_must_differ_from_controller" {
  command = plan

  variables {
    runner_role_arn = "arn:aws:iam::111122223333:role/example-controller"
  }

  expect_failures = [output.launch_template_spec]
}

run "roles_must_share_an_account" {
  command = plan

  variables {
    controller_role_arn = "arn:aws:iam::444455556666:role/example-controller"
  }

  expect_failures = [output.launch_template_spec]
}

run "wildcard_runner_role_is_rejected" {
  command = plan

  variables {
    runner_role_arn = "arn:aws:iam::111122223333:role/example-*"
  }

  expect_failures = [var.runner_role_arn]
}

run "reserved_tag_prefix_cannot_be_used" {
  command = plan

  variables {
    tags = { "leo-runners:managed-by" = "something-else" }
  }

  expect_failures = [output.ownership_tags]
}

run "ownership_tag_matches_the_provider" {
  command = plan

  assert {
    condition     = output.ownership_tags["leo-runners:managed-by"] == "leo-runners" && output.controller_policy_constraints.run_instances.required_request_tags["leo-runners:managed-by"] == "leo-runners"
    error_message = "The contract must use the tag key the controller's AWS provider actually stamps."
  }
}

run "controller_role_is_optional" {
  command = plan

  variables {
    controller_role_arn = null
  }

  assert {
    condition     = output.controller_policy_constraints.pass_role.principal == null && output.controller_policy_constraints.pass_role.resources[0] == var.runner_role_arn
    error_message = "Without a controller role the constraint must still pin the runner role."
  }
}

run "bad_ami_is_rejected" {
  command = plan

  variables {
    ami_id = "latest"
  }

  expect_failures = [var.ami_id]
}
