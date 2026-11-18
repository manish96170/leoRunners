variable "role_name" {
  description = "Stable name for the controller runtime role."
  type        = string

  validation {
    condition     = length(trimspace(var.role_name)) > 0
    error_message = "role_name must not be empty."
  }
}

variable "role_path" {
  description = "IAM path for the controller runtime role."
  type        = string
  default     = "/service-role/"

  validation {
    condition     = startswith(var.role_path, "/") && endswith(var.role_path, "/")
    error_message = "role_path must start and end with /."
  }
}

variable "description" {
  description = "Description stored on the controller runtime role."
  type        = string
  default     = "Ephemeral CI controller runtime role"
}

variable "trusted_service_principals" {
  description = "Explicit AWS service principals allowed to assume the role, such as ec2.amazonaws.com or eks.amazonaws.com."
  type        = set(string)
  default     = []

  validation {
    condition     = alltrue([for principal in var.trusted_service_principals : length(trimspace(principal)) > 0 && principal != "*"])
    error_message = "trusted_service_principals must contain non-empty, explicit service principals and cannot contain *."
  }
}

variable "trusted_principal_arns" {
  description = "Explicit AWS principal ARNs allowed to assume the role. Keep this list limited to the controller runtime identity."
  type        = set(string)
  default     = []

  validation {
    condition     = alltrue([for principal in var.trusted_principal_arns : startswith(principal, "arn:") && !can(regex("\\*", principal))])
    error_message = "trusted_principal_arns must contain explicit, non-wildcard ARNs."
  }
}

variable "oidc_provider_arn" {
  description = "Optional OIDC provider ARN for a controller running with web identity, for example an EKS service account."
  type        = string
  default     = null
}

variable "oidc_condition_prefix" {
  description = "OIDC issuer host used in trust conditions, without https://, for example oidc.eks.us-east-1.amazonaws.com/id/EXAMPLE."
  type        = string
  default     = null
}

variable "oidc_subjects" {
  description = "Exact OIDC subject claims allowed to assume the role. Wildcard subjects are intentionally not accepted by this module."
  type        = set(string)
  default     = []
}

variable "oidc_audience" {
  description = "Exact OIDC audience claim required by the trust policy."
  type        = string
  default     = "sts.amazonaws.com"
}

variable "permissions_boundary_arn" {
  description = "Optional permissions boundary ARN for the controller role."
  type        = string
  default     = null
}

variable "generated_policy_json" {
  description = "Optional least-privilege permissions policy generated and reviewed by iam-policy-autopilot from the controller runtime source. This module does not derive actions."
  type        = string
  default     = null
  sensitive   = true
}

variable "generated_policy_arns" {
  description = "Optional ARNs of externally managed, reviewed least-privilege policies generated from controller runtime behavior."
  type        = set(string)
  default     = []
}

variable "tags" {
  description = "Tags applied to the controller IAM role."
  type        = map(string)
  default     = {}
}

variable "max_session_duration" {
  description = "Maximum role session duration in seconds."
  type        = number
  default     = 3600

  validation {
    condition     = var.max_session_duration >= 3600 && var.max_session_duration <= 43200
    error_message = "max_session_duration must be between 3600 and 43200 seconds."
  }
}
