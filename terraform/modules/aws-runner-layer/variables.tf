variable "name_prefix" {
  description = "Prefix for the names of everything this module creates."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,30}[a-z0-9]$", var.name_prefix))
    error_message = "name_prefix must be 3-32 characters of lowercase letters, digits, and hyphens, starting with a letter."
  }
}

variable "ami_id" {
  description = "Immutable runner AMI ID (for example from the ami/ Packer pipeline). The module does not choose an image."
  type        = string

  validation {
    condition     = can(regex("^ami-[0-9a-f]{8}([0-9a-f]{9})?$", var.ami_id))
    error_message = "ami_id must be an AMI ID such as ami-0123456789abcdef0."
  }
}

variable "instance_type" {
  description = "EC2 instance type for runners. Size it for the workload (browser tests and Docker need more than a build-only job)."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]+\\.[a-z0-9]+$", var.instance_type))
    error_message = "instance_type must be an instance type such as c7i.xlarge."
  }
}

###############################################################################
# Network (all supplied by the caller; nothing is assumed)
###############################################################################

variable "subnet_id" {
  description = "Subnet runners launch into. The launch template defines the network interface, so the controller must not also be given a subnet or security groups."
  type        = string

  validation {
    condition     = can(regex("^subnet-[0-9a-f]{8}([0-9a-f]{9})?$", var.subnet_id))
    error_message = "subnet_id must be a subnet ID such as subnet-0123456789abcdef0."
  }
}

variable "create_security_group" {
  description = "Create an ingress-free runner security group in vpc_id. Set false to use only security_group_ids."
  type        = bool
  default     = true
}

variable "vpc_id" {
  description = "VPC of subnet_id. Required when create_security_group is true."
  type        = string
  default     = null

  validation {
    condition     = var.vpc_id == null || can(regex("^vpc-[0-9a-f]{8}([0-9a-f]{9})?$", var.vpc_id))
    error_message = "vpc_id must be a VPC ID such as vpc-0123456789abcdef0."
  }
}

variable "security_group_ids" {
  description = "Existing security groups to attach in addition to (or instead of) the created one. They should allow no inbound traffic."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for s in var.security_group_ids : can(regex("^sg-[0-9a-f]{8}([0-9a-f]{9})?$", s))])
    error_message = "Every security_group_ids entry must be a security group ID."
  }
}

variable "egress_https_cidr_blocks" {
  description = "IPv4 CIDRs the created security group lets runners reach on TCP/443 (GitHub, package registries, AWS APIs). No default: choose explicitly, for example your NAT or proxy ranges. Runners that cannot reach GitHub cannot register."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.egress_https_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every egress_https_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

variable "egress_https_prefix_list_ids" {
  description = "Managed prefix list IDs the created security group lets runners reach on TCP/443 (for example S3 gateway endpoints)."
  type        = list(string)
  default     = []
}

variable "egress_dns_cidr_blocks" {
  description = "IPv4 CIDRs the created security group lets runners reach on TCP/UDP 53. Needed only for custom resolvers; the Amazon-provided VPC resolver is not filtered."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.egress_dns_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every egress_dns_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

###############################################################################
# Runner identity
###############################################################################

variable "role_name" {
  description = "Name of the runner IAM role and instance profile. Null uses \"<name_prefix>-runner\"."
  type        = string
  default     = null
}

variable "iam_path" {
  description = "IAM path for the role and instance profile."
  type        = string
  default     = "/"
}

variable "permissions_boundary_arn" {
  description = "Optional permissions boundary for the runner role."
  type        = string
  default     = null
}

variable "managed_policy_arns" {
  description = "Managed policies to attach to the runner role. The role starts with no permissions; grant only what workflows need."
  type        = list(string)
  default     = []
}

variable "role_policy_json" {
  description = "Optional inline policy for what workflows legitimately need in the account (for example access to temporary integration resources). Rejected if it allows Action \"*\", uses Allow with NotAction, or allows iam:PassRole on a wildcard. Review it as you would any role that runs untrusted pull-request code."
  type        = string
  default     = null
}

variable "enable_ssm" {
  description = "Attach AmazonSSMManagedInstanceCore so operators can use Session Manager. Off by default."
  type        = bool
  default     = false
}

variable "controller_role_arn" {
  description = "ARN of the controller task role. Optional; when set the module also checks it differs from the runner role and shares its account, and publishes the pass-role constraint with this principal."
  type        = string
  default     = null
}

###############################################################################
# Instance
###############################################################################

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

variable "user_data" {
  description = "Runner bootstrap script containing the {{GITHUB_JIT_CONFIG_B64}} placeholder exactly once. Null uses the bundled bootstrap/runner/user-data.sh, which downloads and verifies a pinned runner at every launch; production images should pre-bake the runner and pass a shorter script. The launch template itself never holds user data: the controller adds it, with the one-time JIT configuration, at launch."
  type        = string
  default     = null

  validation {
    condition     = var.user_data == null || length(split("{{GITHUB_JIT_CONFIG_B64}}", var.user_data)) == 2
    error_message = "user_data must contain the {{GITHUB_JIT_CONFIG_B64}} placeholder exactly once."
  }
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

###############################################################################
# Ownership and cleanup
###############################################################################

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
  description = "Extra tags for everything this module creates and for every runner. They are also passed to the controller (see controller_runner.tags), so keys and values must contain no comma, keys no equals sign, and nothing may use the reserved leo-runners: or aws: prefixes."
  type        = map(string)
  default     = {}

  validation {
    condition = alltrue([
      for k, v in var.tags :
      !startswith(lower(k), "leo-runners:") && !startswith(lower(k), "aws:") && length(k) >= 1 && length(k) <= 128 && length(v) >= 1 && length(v) <= 256 && !strcontains(k, ",") && !strcontains(k, "=") && !strcontains(v, ",")
    ])
    error_message = "tags must be non-empty, have no comma in keys or values and no equals sign in keys, and not use the leo-runners: or aws: prefixes."
  }
}
