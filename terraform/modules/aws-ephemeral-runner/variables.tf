variable "name_prefix" {
  description = "Prefix for the future launch template and related names."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,30}[a-z0-9]$", var.name_prefix))
    error_message = "name_prefix must be 3-32 characters of lowercase letters, digits, and hyphens, starting with a letter."
  }
}

variable "ami_id" {
  description = "Immutable runner AMI ID produced by the ami/ Packer pipeline."
  type        = string

  validation {
    condition     = can(regex("^ami-[0-9a-f]{8}([0-9a-f]{9})?$", var.ami_id))
    error_message = "ami_id must be an AMI ID such as ami-0123456789abcdef0."
  }
}

variable "instance_types" {
  description = "Allowed EC2 instance types, in preference order."
  type        = list(string)

  validation {
    condition     = length(var.instance_types) > 0 && alltrue([for t in var.instance_types : can(regex("^[a-z0-9]+\\.[a-z0-9]+$", t))])
    error_message = "instance_types must contain at least one valid instance type such as c7i.xlarge."
  }
}

variable "subnet_ids" {
  description = "Subnets runners may launch into. Supplied by the customer; the module assumes no names or topology."
  type        = list(string)

  validation {
    condition     = length(var.subnet_ids) > 0 && alltrue([for s in var.subnet_ids : can(regex("^subnet-[0-9a-f]{8}([0-9a-f]{9})?$", s))])
    error_message = "subnet_ids must contain at least one subnet ID."
  }
}

variable "security_group_ids" {
  description = "Security groups attached to runners. Expected to allow no inbound traffic and only the outbound access the workloads need."
  type        = list(string)

  validation {
    condition     = length(var.security_group_ids) > 0 && alltrue([for s in var.security_group_ids : can(regex("^sg-[0-9a-f]{8}([0-9a-f]{9})?$", s))])
    error_message = "security_group_ids must contain at least one security group ID."
  }
}

variable "runner_role_arn" {
  description = "ARN of the dedicated runner IAM role. It must not be the controller role or a customer workload role."
  type        = string

  validation {
    condition     = can(regex("^arn:[^:]+:iam::[0-9]{12}:role/.+$", var.runner_role_arn)) && !strcontains(var.runner_role_arn, "*")
    error_message = "runner_role_arn must be an exact IAM role ARN with no wildcard."
  }
}

variable "runner_instance_profile_arn" {
  description = "ARN of the instance profile that wraps runner_role_arn."
  type        = string

  validation {
    condition     = can(regex("^arn:[^:]+:iam::[0-9]{12}:instance-profile/.+$", var.runner_instance_profile_arn)) && !strcontains(var.runner_instance_profile_arn, "*")
    error_message = "runner_instance_profile_arn must be an exact IAM instance profile ARN with no wildcard."
  }
}

variable "controller_role_arn" {
  description = "ARN of the controller task role that is allowed to pass runner_role_arn to EC2."
  type        = string

  validation {
    condition     = can(regex("^arn:[^:]+:iam::[0-9]{12}:role/.+$", var.controller_role_arn)) && !strcontains(var.controller_role_arn, "*")
    error_message = "controller_role_arn must be an exact IAM role ARN with no wildcard."
  }
}

variable "root_device_name" {
  description = "Root device name of the runner AMI."
  type        = string
  default     = "/dev/xvda"
}

variable "root_volume_size_gb" {
  description = "Size of the encrypted gp3 root volume."
  type        = number
  default     = 100

  validation {
    condition     = var.root_volume_size_gb >= 20 && var.root_volume_size_gb <= 16384
    error_message = "root_volume_size_gb must be between 20 and 16384."
  }
}

variable "root_volume_iops" {
  description = "Provisioned IOPS for the gp3 root volume."
  type        = number
  default     = 3000

  validation {
    condition     = var.root_volume_iops >= 3000 && var.root_volume_iops <= 16000
    error_message = "root_volume_iops must be between 3000 and 16000 for gp3."
  }
}

variable "root_volume_throughput_mibps" {
  description = "Provisioned throughput for the gp3 root volume."
  type        = number
  default     = 125

  validation {
    condition     = var.root_volume_throughput_mibps >= 125 && var.root_volume_throughput_mibps <= 1000
    error_message = "root_volume_throughput_mibps must be between 125 and 1000 for gp3."
  }
}

variable "root_volume_kms_key_arn" {
  description = "Optional customer-managed KMS key for root volume encryption. Null uses the account's EBS default key. Encryption itself is never optional."
  type        = string
  default     = null
}

variable "max_lifetime_minutes" {
  description = "Hard maximum runner lifetime. The controller stamps ExpiresAt = CreatedAt + this value and the reaper terminates anything past it."
  type        = number
  default     = 120

  validation {
    condition     = var.max_lifetime_minutes >= 5 && var.max_lifetime_minutes <= 1440
    error_message = "max_lifetime_minutes must be between 5 and 1440."
  }
}

variable "request_id" {
  description = "Optional deployment or change-request identifier. When set it is stamped on every runner and becomes part of the request-scoped cleanup selector."
  type        = string
  default     = null

  validation {
    condition     = var.request_id == null || can(regex("^[A-Za-z0-9._:/@=+-]{1,128}$", var.request_id))
    error_message = "request_id must be 1-128 characters valid in an EC2 tag value."
  }
}

variable "cleanup_deadline" {
  description = "Optional RFC 3339 UTC deadline after which request-scoped resources should be gone."
  type        = string
  default     = null

  validation {
    condition     = var.cleanup_deadline == null || can(regex("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$", var.cleanup_deadline))
    error_message = "cleanup_deadline must be an RFC 3339 UTC timestamp such as 2030-01-01T00:00:00Z."
  }
}

variable "tags" {
  description = "Additional ownership tags. Platform and ManagedBy are fixed by the contract and cannot be overridden."
  type        = map(string)
  default     = {}
}
