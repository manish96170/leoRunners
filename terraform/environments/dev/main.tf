module "controller_state" {
  source = "../../modules/controller-state"

  table_name                    = var.state_table_name
  enable_point_in_time_recovery = var.enable_point_in_time_recovery
  enable_deletion_protection    = var.enable_deletion_protection
  kms_key_arn                   = var.state_kms_key_arn
  tags                          = local.common_tags
}

module "controller_iam" {
  source = "../../modules/controller-iam"

  role_name                  = var.controller_role_name
  trusted_service_principals = var.controller_trusted_service_principals
  trusted_principal_arns     = var.controller_trusted_principal_arns
  oidc_provider_arn          = var.controller_oidc_provider_arn
  oidc_condition_prefix      = var.controller_oidc_condition_prefix
  oidc_subjects              = var.controller_oidc_subjects
  permissions_boundary_arn   = var.controller_permissions_boundary_arn

  # This value must come from an independently generated and reviewed policy.
  generated_policy_json = var.controller_generated_policy_json
  tags                  = local.common_tags
}

module "runner_runtime" {
  source = "../../modules/runner-runtime"

  name                     = var.runner_role_name
  enable_ssm               = var.enable_runner_ssm
  permissions_boundary_arn = var.runner_permissions_boundary_arn
  tags                     = local.common_tags
}

module "observability" {
  count  = var.enable_observability ? 1 : 0
  source = "../../modules/observability"

  name               = var.observability_name
  namespace          = var.observability_namespace
  log_retention_days = var.observability_log_retention_days
  kms_key_id         = var.observability_kms_key_id
  skip_destroy       = var.observability_skip_destroy
  metric_dimensions  = var.observability_metric_dimensions
  alarm_actions      = var.observability_alarm_actions
  tags               = local.common_tags
}

locals {
  common_tags = merge(
    {
      environment = "dev"
      managed-by  = "terraform"
      project     = "leo-runners"
    },
    var.tags,
  )
}
