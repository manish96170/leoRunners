variable "aws_region" {
  description = "AWS region for the isolated UAT foundation."
  type        = string
  default     = "us-west-2"
}

variable "name_prefix" {
  description = "Stable isolated resource prefix."
  type        = string
  default     = "leo-runners-uat"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{2,50}$", var.name_prefix))
    error_message = "name_prefix must contain lowercase letters, numbers, and hyphens."
  }
}

variable "owner" {
  description = "Accountable UAT owner tag."
  type        = string
  default     = "anchor-mani"
}

variable "repository" {
  description = "Repository admitted to this UAT foundation."
  type        = string
  default     = "anchor-os/anchor-dashboard"
}

variable "runner_group" {
  description = "Future GitHub runner-group identity. No GitHub runner group is created by this foundation."
  type        = string
  default     = "leo-runners-uat-dashboard"
}

variable "request_id" {
  description = "Approved UAT request identity used for exact cleanup selection."
  type        = string
  default     = "foundation-review"
}

variable "cleanup_deadline" {
  description = "UTC cleanup deadline tag for the reviewed foundation request."
  type        = string
  default     = "2099-01-01T00:00:00Z"
}

variable "controller_vpc_id" {
  description = "Existing two-AZ controller VPC. This composition does not create a VPC."
  type        = string
}

variable "controller_vpc_cidr" {
  description = "CIDR of the existing controller VPC, used only for DNS egress rules."
  type        = string
}

variable "runner_vpc_id" {
  description = "Existing UAT runner VPC. This composition does not create a VPC."
  type        = string
}

variable "runner_vpc_cidr" {
  description = "CIDR of the existing runner VPC."
  type        = string
}

variable "runner_subnet_ids" {
  description = "Existing private subnets approved for future ephemeral runners. No runner is launched by this foundation."
  type        = list(string)
}

variable "controller_generated_policy_json" {
  description = "Optional reviewed output from iam-policy-autopilot. Null creates the controller role without runtime permissions."
  type        = string
  default     = null
  sensitive   = true
}

variable "enable_point_in_time_recovery" {
  description = "Enable DynamoDB point-in-time recovery during UAT."
  type        = bool
  default     = true
}

variable "log_retention_days" {
  description = "Controller log retention in days."
  type        = number
  default     = 30

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365], var.log_retention_days)
    error_message = "log_retention_days must be a supported CloudWatch retention period."
  }
}

variable "tags" {
  description = "Additional non-secret tags for all foundation resources."
  type        = map(string)
  default     = {}
}
