variable "name" {
  description = "Stable name for the ephemeral runner IAM role and instance profile."
  type        = string
}

variable "path" {
  description = "IAM path for the runner role and instance profile."
  type        = string
  default     = "/leo-runners/"
}

variable "enable_ssm" {
  description = "Attach the AWS-managed SSM core policy to the runner role. Keep false unless operators explicitly need SSM."
  type        = bool
  default     = false
}

variable "permissions_boundary_arn" {
  description = "Optional permissions boundary ARN for the runner role."
  type        = string
  default     = null
}

variable "tags" {
  description = "Tags applied to the runner role and instance profile."
  type        = map(string)
  default     = {}
}
