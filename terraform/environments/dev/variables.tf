variable "aws_region" {
  description = "AWS region for the development composition. No networking is created here."
  type        = string

  validation {
    condition     = can(regex("^[a-z]{2}(-gov)?-[a-z0-9-]+-[0-9]+$", var.aws_region))
    error_message = "aws_region must be an AWS region name such as us-east-1."
  }
}

variable "state_table_name" {
  description = "DynamoDB table name for shared controller state."
  type        = string
}

variable "controller_role_name" {
  description = "Name of the controller runtime role."
  type        = string
}

variable "runner_role_name" {
  description = "Name of the ephemeral runner role and instance profile."
  type        = string
}

variable "controller_trusted_service_principals" {
  description = "Explicit AWS service principals allowed to assume the controller role."
  type        = set(string)
  nullable    = false

  validation {
    condition     = length(var.controller_trusted_service_principals) > 0 && alltrue([for principal in var.controller_trusted_service_principals : principal != "*"])
    error_message = "Provide at least one explicit non-wildcard controller service principal."
  }
}

variable "controller_trusted_principal_arns" {
  description = "Optional explicit AWS principal ARNs allowed to assume the controller role."
  type        = set(string)
  default     = []
}

variable "controller_oidc_provider_arn" {
  description = "Optional OIDC provider ARN for controller web-identity access."
  type        = string
  default     = null
}

variable "controller_oidc_condition_prefix" {
  description = "Optional OIDC issuer host used in exact trust conditions."
  type        = string
  default     = null
}

variable "controller_oidc_subjects" {
  description = "Optional exact OIDC subjects allowed to assume the controller role."
  type        = set(string)
  default     = []
}

variable "controller_permissions_boundary_arn" {
  description = "Optional permissions boundary ARN for the controller role."
  type        = string
  default     = null
}

variable "controller_generated_policy_json" {
  description = "Required reviewed IAM policy JSON generated externally by iam-policy-autopilot and approved before apply."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = length(trimspace(var.controller_generated_policy_json)) > 0 && can(jsondecode(var.controller_generated_policy_json))
    error_message = "controller_generated_policy_json must be a non-empty, valid JSON policy supplied from a reviewed external artifact."
  }
}

variable "state_kms_key_arn" {
  description = "Optional customer-managed KMS key ARN for the DynamoDB table."
  type        = string
  default     = null
}

variable "enable_point_in_time_recovery" {
  description = "Enable DynamoDB point-in-time recovery."
  type        = bool
  default     = true
}

variable "enable_deletion_protection" {
  description = "Protect the development state table from accidental deletion."
  type        = bool
  default     = true
}

variable "enable_runner_ssm" {
  description = "Attach AmazonSSMManagedInstanceCore to the runner role. Keep false unless explicitly reviewed."
  type        = bool
  default     = false
}

variable "runner_permissions_boundary_arn" {
  description = "Optional permissions boundary ARN for the runner role."
  type        = string
  default     = null
}

variable "tags" {
  description = "Additional tags for composed resources."
  type        = map(string)
  default     = {}
}
