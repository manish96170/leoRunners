locals {
  service_trust_statements = [
    for principal in sort(tolist(var.trusted_service_principals)) : {
      Effect    = "Allow"
      Principal = { Service = principal }
      Action    = "sts:AssumeRole"
    }
  ]

  arn_trust_statements = [
    for principal in sort(tolist(var.trusted_principal_arns)) : {
      Effect    = "Allow"
      Principal = { AWS = principal }
      Action    = "sts:AssumeRole"
    }
  ]

  oidc_trust_statements = var.oidc_provider_arn == null ? [] : [{
    Effect    = "Allow"
    Principal = { Federated = var.oidc_provider_arn }
    Action    = "sts:AssumeRoleWithWebIdentity"
    Condition = {
      StringEquals = {
        "${var.oidc_condition_prefix}:aud" = var.oidc_audience
        "${var.oidc_condition_prefix}:sub" = sort(tolist(var.oidc_subjects))
      }
    }
  }]

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      local.service_trust_statements,
      local.arn_trust_statements,
      local.oidc_trust_statements,
    )
  })
}

resource "aws_iam_role" "controller" {
  name                 = var.role_name
  path                 = var.role_path
  description          = var.description
  assume_role_policy   = local.assume_role_policy
  max_session_duration = var.max_session_duration
  permissions_boundary = var.permissions_boundary_arn
  tags                 = var.tags

  lifecycle {
    precondition {
      condition = (
        length(var.trusted_service_principals) > 0 ||
        length(var.trusted_principal_arns) > 0 ||
        var.oidc_provider_arn != null
      )
      error_message = "At least one explicit service, AWS ARN, or OIDC trust principal is required."
    }

    precondition {
      condition = (
        var.oidc_provider_arn == null && var.oidc_condition_prefix == null && length(var.oidc_subjects) == 0
        ) || (
        var.oidc_provider_arn != null &&
        var.oidc_condition_prefix != null &&
        length(trimspace(var.oidc_condition_prefix)) > 0 &&
        length(var.oidc_subjects) > 0 &&
        !contains(tolist(var.oidc_subjects), "*")
      )
      error_message = "OIDC trust requires a provider, issuer condition prefix, and at least one exact subject; wildcard subjects are not allowed."
    }
  }
}

# This policy is intentionally supplied from outside the module. Generate it
# from controller runtime source with iam-policy-autopilot, review it, then
# pass the result through generated_policy_json or generated_policy_arns.
resource "aws_iam_role_policy" "generated_runtime" {
  count = var.generated_policy_json == null ? 0 : 1

  name   = "${var.role_name}-runtime-generated"
  role   = aws_iam_role.controller.name
  policy = var.generated_policy_json

  lifecycle {
    precondition {
      condition     = try(jsondecode(var.generated_policy_json).Version == "2012-10-17", false)
      error_message = "generated_policy_json must be a valid IAM policy document with Version 2012-10-17."
    }
  }
}

resource "aws_iam_role_policy_attachment" "generated_runtime" {
  for_each = var.generated_policy_arns

  role       = aws_iam_role.controller.name
  policy_arn = each.value
}
