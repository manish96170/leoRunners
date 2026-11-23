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
  name_prefix              = "leo-test"
  vpc_id                   = "vpc-0123456789abcdef0"
  alb_security_group_id    = "sg-0123456789abcdef0"
  egress_https_cidr_blocks = ["10.0.0.0/8"]
}

run "disabled_by_default_creates_nothing" {
  command = plan

  assert {
    condition     = output.phase == "disabled"
    error_message = "With no phase flag set the module must be inert."
  }

  assert {
    condition     = length(aws_iam_role.task) == 0 && length(aws_security_group.task) == 0 && length(aws_ecs_service.controller) == 0
    error_message = "Nothing may be created before create_foundation is true."
  }
}

run "foundation_never_starts_the_controller" {
  command = plan

  variables {
    create_foundation = true
    create_cluster    = true
  }

  assert {
    condition     = output.phase == "foundation"
    error_message = "Phase must be foundation."
  }

  assert {
    condition = (
      length(aws_ecr_repository.controller) == 1 &&
      length(aws_cloudwatch_log_group.controller) == 1 &&
      length(module.state) == 1 &&
      length(aws_secretsmanager_secret.webhook) == 1 &&
      length(aws_iam_role.execution) == 1 &&
      length(aws_iam_role.task) == 1 &&
      length(aws_security_group.task) == 1 &&
      length(aws_lb_target_group.controller) == 1
    )
    error_message = "Foundation must create the registry, logs, state, secret container, roles, security group, and target group."
  }

  assert {
    condition     = length(aws_ecs_service.controller) == 0 && length(aws_ecs_task_definition.controller) == 0 && length(aws_lb_listener_rule.controller) == 0
    error_message = "Foundation must not create the service, task definition, or listener rule."
  }

  assert {
    condition     = length(aws_secretsmanager_secret.github_token) == 0
    error_message = "The installation-token container is only created when GitHub JIT is enabled."
  }
}

run "foundation_with_jit_creates_token_container" {
  command = plan

  variables {
    create_foundation = true
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
    }
  }

  assert {
    condition     = length(aws_secretsmanager_secret.github_token) == 1
    error_message = "JIT requires the installation-token secret container."
  }

  assert {
    condition     = output.container_environment["GITHUB_RUNNER_GROUP_ID"] == "1" && output.container_environment["GITHUB_ALLOWED_ORGANIZATIONS"] == "example-org"
    error_message = "JIT settings must reach the container environment."
  }
}

run "listener_rule_requires_inputs" {
  command = plan

  variables {
    create_foundation    = true
    enable_listener_rule = true
  }

  expect_failures = [terraform_data.guard]
}

run "listener_rule_without_service" {
  command = plan

  variables {
    create_foundation      = true
    enable_listener_rule   = true
    listener_arn           = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    listener_path_patterns = ["/hooks/example"]
    listener_priority      = 150
  }

  assert {
    condition     = length(aws_lb_listener_rule.controller) == 1 && length(aws_ecs_service.controller) == 0
    error_message = "The route can be enabled before the service starts."
  }
}

run "service_requires_listener_rule" {
  command = plan

  variables {
    create_foundation     = true
    enable_service        = true
    create_cluster        = true
    image_digest          = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    secrets_ready         = true
    subnet_ids            = ["subnet-0123456789abcdef0"]
    task_role_policy_arns = ["arn:aws:iam::111122223333:policy/reviewed-controller"]
  }

  expect_failures = [terraform_data.guard]
}

run "service_requires_digest" {
  command = plan

  variables {
    create_foundation      = true
    enable_listener_rule   = true
    enable_service         = true
    create_cluster         = true
    secrets_ready          = true
    subnet_ids             = ["subnet-0123456789abcdef0"]
    listener_arn           = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    listener_path_patterns = ["/hooks/example"]
    listener_priority      = 150
    task_role_policy_arns  = ["arn:aws:iam::111122223333:policy/reviewed-controller"]
  }

  expect_failures = [terraform_data.guard]
}

run "service_requires_secrets_ready" {
  command = plan

  variables {
    create_foundation      = true
    enable_listener_rule   = true
    enable_service         = true
    create_cluster         = true
    image_digest           = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    subnet_ids             = ["subnet-0123456789abcdef0"]
    listener_arn           = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    listener_path_patterns = ["/hooks/example"]
    listener_priority      = 150
    task_role_policy_arns  = ["arn:aws:iam::111122223333:policy/reviewed-controller"]
  }

  expect_failures = [terraform_data.guard]
}

run "service_requires_runtime_policy" {
  command = plan

  variables {
    create_foundation      = true
    enable_listener_rule   = true
    enable_service         = true
    create_cluster         = true
    image_digest           = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    secrets_ready          = true
    subnet_ids             = ["subnet-0123456789abcdef0"]
    listener_arn           = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    listener_path_patterns = ["/hooks/example"]
    listener_priority      = 150
  }

  expect_failures = [terraform_data.guard]
}

run "full_service_phase" {
  command = plan

  variables {
    create_foundation      = true
    enable_listener_rule   = true
    enable_service         = true
    create_cluster         = true
    image_digest           = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
    secrets_ready          = true
    subnet_ids             = ["subnet-0123456789abcdef0"]
    listener_arn           = "arn:aws:elasticloadbalancing:us-east-1:111122223333:listener/app/example/0123456789abcdef/0123456789abcdef"
    listener_path_patterns = ["/hooks/example"]
    listener_priority      = 150
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem", "dynamodb:PutItem"]
        Resource = ["arn:aws:dynamodb:us-east-1:111122223333:table/leo-test-state"]
      }]
    })
  }

  assert {
    condition     = output.phase == "service"
    error_message = "Phase must be service."
  }

  assert {
    condition     = length(aws_ecs_service.controller) == 1 && length(aws_ecs_task_definition.controller) == 1 && length(aws_lb_listener_rule.controller) == 1
    error_message = "The service phase must create the task definition, service, and route."
  }

  assert {
    condition     = output.container_environment["DYNAMODB_TABLE_NAME"] == "leo-test-state" && output.container_environment["AI_ENABLED"] == "false"
    error_message = "The container must use the DynamoDB state backend with AI disabled."
  }

  assert {
    condition     = !contains(keys(output.container_environment), "GITHUB_WEBHOOK_SECRET") && !contains(keys(output.container_environment), "GITHUB_APP_INSTALLATION_TOKEN")
    error_message = "Secrets must never appear in the plain environment."
  }
}

run "mutable_tag_is_rejected" {
  command = plan

  variables {
    image_digest = "latest"
  }

  expect_failures = [var.image_digest]
}

run "credential_like_environment_key_is_rejected" {
  command = plan

  variables {
    extra_environment = { SOME_API_TOKEN = "x" }
  }

  expect_failures = [var.extra_environment]
}

run "multi_replica_is_rejected" {
  command = plan

  variables {
    desired_count = 2
  }

  expect_failures = [var.desired_count]
}

run "wildcard_passrole_is_rejected" {
  command = plan

  variables {
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = "iam:PassRole"
        Resource = "*"
      }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "partial_wildcard_passrole_is_rejected" {
  command = plan

  variables {
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = ["iam:PassRole"]
        Resource = ["arn:aws:iam::111122223333:role/runner-*"]
      }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "allow_all_actions_is_rejected" {
  command = plan

  variables {
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = "*"
        Resource = ["arn:aws:dynamodb:us-east-1:111122223333:table/leo-test-state"]
      }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "allow_not_action_is_rejected" {
  command = plan

  variables {
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect    = "Allow"
        NotAction = ["iam:DeleteRole"]
        Resource  = ["arn:aws:dynamodb:us-east-1:111122223333:table/leo-test-state"]
      }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "single_statement_object_policy_is_accepted" {
  command = plan

  variables {
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = {
        Effect   = "Allow"
        Action   = "dynamodb:GetItem"
        Resource = "arn:aws:dynamodb:us-east-1:111122223333:table/leo-test-state"
      }
    })
  }

  assert {
    condition     = output.phase == "disabled"
    error_message = "A valid single-statement policy must be accepted."
  }
}

run "garbage_policy_is_rejected" {
  command = plan

  variables {
    task_role_policy_json = "not json"
  }

  expect_failures = [terraform_data.guard]
}

run "jit_requires_scope" {
  command = plan

  variables {
    github = {
      jit_enabled     = true
      runner_group_id = 1
    }
  }

  expect_failures = [terraform_data.guard]
}

run "runner_group_without_jit_is_rejected" {
  command = plan

  variables {
    github = {
      runner_group_id = 1
    }
  }

  expect_failures = [terraform_data.guard]
}

run "runner_lifecycle_requires_scoped_passrole" {
  command = plan

  variables {
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
    }
    runner = {
      enabled              = true
      launch_template_id   = "lt-0123456789abcdef0"
      role_arn             = "arn:aws:iam::111122223333:role/example-runner"
      instance_profile_arn = "arn:aws:iam::111122223333:instance-profile/example-runner"
    }
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = ["ec2:DescribeInstances"]
        Resource = ["*"]
      }]
    })
  }

  expect_failures = [terraform_data.guard]
}

run "runner_lifecycle_with_scoped_passrole" {
  command = plan

  variables {
    create_foundation = true
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
    }
    runner = {
      enabled              = true
      launch_template_id   = "lt-0123456789abcdef0"
      role_arn             = "arn:aws:iam::111122223333:role/example-runner"
      instance_profile_arn = "arn:aws:iam::111122223333:instance-profile/example-runner"
    }
    task_role_policy_json = jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect    = "Allow"
        Action    = ["iam:PassRole"]
        Resource  = ["arn:aws:iam::111122223333:role/example-runner"]
        Condition = { StringEquals = { "iam:PassedToService" = "ec2.amazonaws.com" } }
      }]
    })
  }

  assert {
    condition     = output.container_environment["AWS_LAUNCH_TEMPLATE_ID"] == "lt-0123456789abcdef0" && output.container_environment["CAPACITY_PROVIDER"] == "aws"
    error_message = "Runner settings must reach the container when the lifecycle is enabled."
  }
}

run "token_container_can_be_kept_while_jit_is_off" {
  command = plan

  variables {
    create_foundation          = true
    create_github_token_secret = true
  }

  assert {
    condition     = length(aws_secretsmanager_secret.github_token) == 1
    error_message = "The token container must be creatable ahead of enabling JIT."
  }

  assert {
    condition     = !contains(keys(output.container_environment), "GITHUB_RUNNER_GROUP_ID")
    error_message = "JIT settings must stay out of the environment while JIT is off."
  }
}

run "role_names_can_be_overridden" {
  command = plan

  variables {
    create_foundation   = true
    execution_role_name = "custom-execution"
    task_role_name      = "custom-task"
  }

  assert {
    condition     = aws_iam_role.execution[0].name == "custom-execution" && aws_iam_role.task[0].name == "custom-task"
    error_message = "Role name overrides must be honored."
  }

  assert {
    condition     = aws_iam_role_policy.execution_secrets[0].name == "custom-execution-secrets"
    error_message = "The inline secrets policy name must follow the execution role name."
  }
}

run "github_app_mode_uses_a_private_key_not_a_token" {
  command = plan

  variables {
    create_foundation = true
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
      app = {
        app_id            = "12345"
        installation_id   = 678
        token_permissions = { administration = "write", actions = "read" }
      }
    }
  }

  assert {
    condition     = length(aws_secretsmanager_secret.github_app_key) == 1 && length(aws_secretsmanager_secret.github_token) == 0
    error_message = "App mode must create the private-key container and no installation-token container."
  }

  assert {
    condition     = contains(output.container_secret_names, "GITHUB_APP_PRIVATE_KEY") && !contains(output.container_secret_names, "GITHUB_APP_INSTALLATION_TOKEN") && contains(output.container_secret_names, "GITHUB_WEBHOOK_SECRET")
    error_message = "The container must receive the private key and never the static token in App mode."
  }

  assert {
    condition = (
      output.container_environment["GITHUB_APP_ID"] == "12345" &&
      output.container_environment["GITHUB_APP_INSTALLATION_ID"] == "678" &&
      output.container_environment["GITHUB_APP_TOKEN_PERMISSIONS"] == "actions=read,administration=write"
    )
    error_message = "App identifiers must reach the container as plain environment, with permissions sorted."
  }

  assert {
    condition     = !contains(keys(output.container_environment), "GITHUB_APP_PRIVATE_KEY")
    error_message = "The private key must never appear in the plain environment."
  }
}

run "static_token_mode_is_unchanged_without_app" {
  command = plan

  variables {
    create_foundation = true
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
    }
  }

  assert {
    condition     = length(aws_secretsmanager_secret.github_token) == 1 && length(aws_secretsmanager_secret.github_app_key) == 0
    error_message = "Without an App the static token container is used."
  }

  assert {
    condition     = contains(output.container_secret_names, "GITHUB_APP_INSTALLATION_TOKEN") && !contains(output.container_secret_names, "GITHUB_APP_PRIVATE_KEY") && !contains(keys(output.container_environment), "GITHUB_APP_ID")
    error_message = "Static-token mode must not receive App settings."
  }
}

run "github_app_requires_jit" {
  command = plan

  variables {
    github = {
      app = { app_id = "12345", installation_id = 678 }
    }
  }

  expect_failures = [var.github]
}

run "github_app_rejects_bad_permission_level" {
  command = plan

  variables {
    github = {
      jit_enabled = true
      app = {
        app_id            = "12345"
        installation_id   = 678
        token_permissions = { administration = "owner" }
      }
    }
  }

  expect_failures = [var.github]
}

run "github_app_rejects_bad_ids" {
  command = plan

  variables {
    github = {
      jit_enabled = true
      app         = { app_id = "bad id", installation_id = 0 }
    }
  }

  expect_failures = [var.github]
}

run "github_app_requires_a_key_secret_when_not_creating_containers" {
  command = plan

  variables {
    create_foundation           = true
    create_secret_containers    = false
    existing_webhook_secret_arn = "arn:aws:secretsmanager:us-east-1:111122223333:secret:example-webhook-AbCdEf"
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
      app                   = { app_id = "12345", installation_id = 678 }
    }
  }

  expect_failures = [terraform_data.guard]
}

run "github_app_accepts_an_existing_key_secret" {
  command = plan

  variables {
    create_foundation                          = true
    create_secret_containers                   = false
    existing_webhook_secret_arn                = "arn:aws:secretsmanager:us-east-1:111122223333:secret:example-webhook-AbCdEf"
    existing_github_app_private_key_secret_arn = "arn:aws:secretsmanager:us-east-1:111122223333:secret:example-app-key-AbCdEf"
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
      app                   = { app_id = "12345", installation_id = 678 }
    }
  }

  assert {
    condition     = output.github_app_private_key_secret_arn == "arn:aws:secretsmanager:us-east-1:111122223333:secret:example-app-key-AbCdEf"
    error_message = "The adopted key secret ARN must be published."
  }
}

run "organization_runner_scope_reaches_the_container" {
  command = plan

  variables {
    create_foundation = true
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      runner_scope          = "organization"
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
      app = {
        app_id            = "12345"
        installation_id   = 678
        token_permissions = { organization_self_hosted_runners = "write" }
      }
    }
  }

  assert {
    condition = (
      output.container_environment["GITHUB_RUNNER_SCOPE"] == "organization" &&
      output.container_environment["GITHUB_APP_TOKEN_PERMISSIONS"] == "organization_self_hosted_runners=write"
    )
    error_message = "The organization scope and its narrower token permission must reach the container."
  }
}

run "repository_runner_scope_is_the_default" {
  command = plan

  variables {
    create_foundation = true
    github = {
      jit_enabled           = true
      runner_group_id       = 1
      allowed_organizations = ["example-org"]
      allowed_labels        = ["self-hosted"]
    }
  }

  assert {
    condition     = output.container_environment["GITHUB_RUNNER_SCOPE"] == "repository"
    error_message = "Existing deployments must keep repository registration unless they opt in."
  }
}

run "unknown_runner_scope_is_rejected" {
  command = plan

  variables {
    github = {
      jit_enabled  = true
      runner_scope = "enterprise"
    }
  }

  expect_failures = [var.github]
}

run "organization_scope_requires_jit" {
  command = plan

  variables {
    github = {
      runner_scope = "organization"
    }
  }

  expect_failures = [var.github]
}
