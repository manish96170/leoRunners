variable "aws_region" {
  description = "AWS region for the foundation."
  type        = string
  default     = "us-east-1"
}

variable "name_prefix" {
  description = "Stable prefix for every resource this composition creates."
  type        = string
  default     = "leo-runners"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{2,50}$", var.name_prefix))
    error_message = "name_prefix must contain lowercase letters, numbers, and hyphens."
  }
}

variable "environment" {
  description = "Environment name used in the Environment tag (for example dev, staging, prod)."
  type        = string
}

variable "owner" {
  description = "Accountable owner, used in the Owner tag."
  type        = string
}

variable "repository" {
  description = "GitHub repository (owner/name) this foundation is admitted for. Used only as a tag."
  type        = string
}

variable "runner_group" {
  description = "Future GitHub runner-group name, used only as a tag. No runner group is created by this composition."
  type        = string
}

variable "request_id" {
  description = "Approved change-request identity, used for exact cleanup selection. The default is a review-only placeholder."
  type        = string
  default     = "foundation-review"
}

variable "cleanup_deadline" {
  description = "UTC cleanup deadline tag for the reviewed request. The default is a review-only placeholder."
  type        = string
  default     = "2099-01-01T00:00:00Z"
}

variable "controller_vpc_id" {
  description = "Existing VPC for the controller. This composition does not create a VPC."
  type        = string
}

variable "controller_vpc_cidr" {
  description = "CIDR of the controller VPC, used only for DNS egress rules."
  type        = string
}

variable "controller_https_egress_cidr_blocks" {
  description = "IPv4 CIDRs the controller may reach on TCP/443 (GitHub API, AWS APIs, image pulls). The default is open because GitHub's address ranges are broad; restrict it when you route through a NAT gateway or VPC endpoints."
  type        = list(string)
  default     = ["0.0.0.0/0"]

  validation {
    condition     = alltrue([for c in var.controller_https_egress_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every controller_https_egress_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

variable "runner_vpc_id" {
  description = "Existing VPC for future ephemeral runners. This composition does not create a VPC."
  type        = string
}

variable "runner_vpc_cidr" {
  description = "CIDR of the runner VPC."
  type        = string
}

variable "runner_subnet_ids" {
  description = "Existing private subnets approved for future ephemeral runners. No runner is launched by this composition."
  type        = list(string)
}

variable "enable_https_gateway" {
  description = "Adopt an existing ALB and certificate and add only a fixed-response HTTPS listener plus the ALB security-group ingress rules."
  type        = bool
  default     = false
}

variable "gateway_alb_arn" {
  description = "Existing gateway ALB ARN. Required only when enable_https_gateway is true."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.gateway_alb_arn == null || can(regex("^arn:[^:]+:elasticloadbalancing:[^:]+:[0-9]{12}:loadbalancer/app/.+$", var.gateway_alb_arn))
    error_message = "gateway_alb_arn must be an Application Load Balancer ARN or null."
  }
}

variable "gateway_alb_security_group_id" {
  description = "Security group already attached to the gateway ALB. Required only when enable_https_gateway is true."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.gateway_alb_security_group_id == null || can(regex("^sg-[0-9a-f]+$", var.gateway_alb_security_group_id))
    error_message = "gateway_alb_security_group_id must be an EC2 security group ID or null."
  }
}

variable "gateway_certificate_arn" {
  description = "Issued ACM certificate ARN in the ALB's region. Required only when enable_https_gateway is true."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.gateway_certificate_arn == null || can(regex("^arn:[^:]+:acm:[^:]+:[0-9]{12}:certificate/.+$", var.gateway_certificate_arn))
    error_message = "gateway_certificate_arn must be an ACM certificate ARN or null."
  }
}

variable "gateway_https_ingress_cidr_blocks" {
  description = "IPv4 CIDRs allowed to reach the gateway HTTPS listener. No default: choose explicitly. Required when enable_https_gateway is true."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.gateway_https_ingress_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every gateway_https_ingress_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

variable "controller_generated_policy_json" {
  description = "Optional reviewed output from iam-policy-autopilot. Null creates the controller role without runtime permissions."
  type        = string
  default     = null
  sensitive   = true
}

variable "enable_point_in_time_recovery" {
  description = "Enable DynamoDB point-in-time recovery on the state table."
  type        = bool
  default     = true
}

variable "enable_state_deletion_protection" {
  description = "Enable DynamoDB deletion protection on the state table. Turn it off only for disposable environments."
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
