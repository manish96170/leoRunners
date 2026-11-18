variable "alb_arn" {
  description = "ARN of the existing Application Load Balancer. The module adopts it by ARN and never creates, modifies, or replaces the load balancer."
  type        = string

  validation {
    condition     = can(regex("^arn:[^:]+:elasticloadbalancing:[^:]+:[0-9]{12}:loadbalancer/app/.+$", var.alb_arn))
    error_message = "alb_arn must be an Application Load Balancer ARN."
  }
}

variable "alb_security_group_id" {
  description = "Security group attached to the ALB. Required when manage_security_group_ingress is true."
  type        = string
  default     = null
}

variable "enable_https_listener" {
  description = "Create the HTTPS listener with a fixed default response. Disabled by default so a review-only plan changes nothing."
  type        = bool
  default     = false
}

variable "existing_https_listener_arn" {
  description = "Adopt an HTTPS listener that already exists instead of creating one. Mutually exclusive with enable_https_listener."
  type        = string
  default     = null

  validation {
    condition     = var.existing_https_listener_arn == null || can(regex("^arn:[^:]+:elasticloadbalancing:[^:]+:[0-9]{12}:listener/app/.+$", var.existing_https_listener_arn))
    error_message = "existing_https_listener_arn must be an Application Load Balancer listener ARN."
  }
}

variable "certificate_arn" {
  description = "ARN of the existing ACM (or IAM) certificate for the HTTPS listener. Required when enable_https_listener is true. The module never requests or validates certificates."
  type        = string
  default     = null

  validation {
    condition     = var.certificate_arn == null || can(regex("^arn:[^:]+:(acm|iam):.+$", var.certificate_arn))
    error_message = "certificate_arn must be an ACM or IAM server certificate ARN."
  }
}

variable "additional_certificate_arns" {
  description = "Additional certificates attached to the HTTPS listener for SNI."
  type        = set(string)
  default     = []
}

variable "https_port" {
  description = "HTTPS listener port."
  type        = number
  default     = 443

  validation {
    condition     = var.https_port >= 1 && var.https_port <= 65535
    error_message = "https_port must be a valid TCP port."
  }
}

variable "ssl_policy" {
  description = "TLS security policy for the HTTPS listener."
  type        = string
  default     = "ELBSecurityPolicy-TLS13-1-2-2021-06"
}

variable "default_response_status_code" {
  description = "Status code of the fixed default response served for any request that matches no listener rule."
  type        = string
  default     = "404"

  validation {
    condition     = can(regex("^[45][0-9]{2}$", var.default_response_status_code))
    error_message = "default_response_status_code must be a 4xx or 5xx status code."
  }
}

variable "default_response_message" {
  description = "Body of the fixed default response."
  type        = string
  default     = "route not configured"
}

variable "enable_http_redirect" {
  description = "Create an HTTP listener that permanently redirects to HTTPS."
  type        = bool
  default     = false
}

variable "http_port" {
  description = "HTTP listener port used for the redirect."
  type        = number
  default     = 80

  validation {
    condition     = var.http_port >= 1 && var.http_port <= 65535
    error_message = "http_port must be a valid TCP port."
  }
}

variable "manage_security_group_ingress" {
  description = "Manage the ALB security-group ingress rules for the listeners. Set false when another stack owns that security group."
  type        = bool
  default     = true
}

variable "https_ingress_cidr_blocks" {
  description = "IPv4 CIDRs allowed to reach the HTTPS listener. No default: choose explicitly. A public webhook receiver may use GitHub's published hook ranges or 0.0.0.0/0."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.https_ingress_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every https_ingress_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

variable "https_ingress_prefix_list_ids" {
  description = "Managed prefix list IDs allowed to reach the HTTPS listener."
  type        = list(string)
  default     = []
}

variable "http_ingress_cidr_blocks" {
  description = "IPv4 CIDRs allowed to reach the HTTP redirect listener. Defaults to the HTTPS CIDRs when empty."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.http_ingress_cidr_blocks : can(cidrnetmask(c))])
    error_message = "Every http_ingress_cidr_blocks entry must be a valid IPv4 CIDR."
  }
}

variable "listener_rule_priority_ranges" {
  description = <<-EOT
    Optional listener-rule priority convention shared by every service behind
    this gateway, keyed by service class, for example
    { controller = { start = 100, end = 199 } }. Ranges must be inside
    1-50000 and must not overlap. The module only validates and republishes
    them; consumers use them to pick listener_priority values.
  EOT
  type = map(object({
    start = number
    end   = number
  }))
  default = {}

  validation {
    condition     = alltrue([for k, r in var.listener_rule_priority_ranges : r.start >= 1 && r.end <= 50000 && r.start <= r.end])
    error_message = "Each priority range must satisfy 1 <= start <= end <= 50000."
  }

  validation {
    condition = alltrue([
      for a, ra in var.listener_rule_priority_ranges : alltrue([
        for b, rb in var.listener_rule_priority_ranges : a == b || ra.end < rb.start || rb.end < ra.start
      ])
    ])
    error_message = "Listener rule priority ranges must not overlap."
  }
}

variable "tags" {
  description = "Tags applied to every taggable resource."
  type        = map(string)
  default     = {}
}
