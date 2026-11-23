variable "region" {
  description = "AWS region of the target account."
  type        = string
}

variable "name_prefix" {
  description = "Prefix for all controller resources."
  type        = string
  default     = "leo-runners"
}

variable "tags" {
  description = "Ownership and request tags applied to everything."
  type        = map(string)
  default     = {}
}

# Existing network and load balancer, adopted by ARN or ID.
variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

variable "alb_arn" {
  type = string
}

variable "alb_security_group_id" {
  type = string
}

variable "certificate_arn" {
  type = string
}

variable "https_ingress_cidr_blocks" {
  type = list(string)
}

variable "egress_https_cidr_blocks" {
  type = list(string)
}

variable "webhook_path" {
  description = "Path the GitHub webhook calls."
  type        = string
  default     = "/webhooks/github"
}

# Phase flags. All default to the inert review-only state.
variable "enable_gateway" {
  type    = bool
  default = false
}

variable "create_foundation" {
  type    = bool
  default = false
}

variable "enable_listener_rule" {
  type    = bool
  default = false
}

variable "enable_service" {
  type    = bool
  default = false
}

# Phase 2 inputs.
variable "image_digest" {
  type    = string
  default = null
}

variable "secrets_ready" {
  type    = bool
  default = false
}

variable "task_role_policy_json" {
  type    = string
  default = null
}

variable "github" {
  type = object({
    jit_enabled           = optional(bool, false)
    runner_group_id       = optional(number)
    allowed_organizations = optional(list(string), [])
    allowed_repositories  = optional(list(string), [])
    allowed_labels        = optional(list(string), [])
    allow_forks           = optional(bool, false)
    api_base_url          = optional(string)
    runner_scope          = optional(string, "repository")
    app = optional(object({
      app_id            = string
      installation_id   = number
      token_permissions = optional(map(string), {})
    }))
  })
  default = {}
}

variable "runner_layer" {
  description = "Optional runner layer. Null (the default) creates no runner infrastructure and keeps the runner lifecycle off. Requires github.jit_enabled and a task_role_policy_json; the PassRole statement for the runner role is added for you."
  type = object({
    ami_id                   = string
    instance_type            = string
    vpc_id                   = string
    subnet_id                = string
    egress_https_cidr_blocks = list(string)
    labels                   = list(string)
    request_id               = optional(string)
    cleanup_deadline         = optional(string)
  })
  default = null
}
