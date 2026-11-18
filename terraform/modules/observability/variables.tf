variable "name" {
  description = "Stable service name used in CloudWatch resource names."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9_-]{1,62}$", var.name))
    error_message = "Name must be 2-63 characters and contain only letters, numbers, underscores, or hyphens."
  }
}

variable "namespace" {
  description = "Custom CloudWatch namespace emitted by the controller's EMF exporter."
  type        = string
  default     = "LeoRunners/Controller"

  validation {
    condition     = length(trimspace(var.namespace)) > 0 && !startswith(var.namespace, "AWS/")
    error_message = "Namespace must be non-empty and must not start with AWS/."
  }
}

variable "log_group_name" {
  description = "CloudWatch Logs group receiving controller JSON/EMF output."
  type        = string
  default     = null
}

variable "log_retention_days" {
  description = "Retention for controller logs."
  type        = number
  default     = 30

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653], var.log_retention_days) && var.log_retention_days >= var.minimum_log_retention_days
    error_message = "Log retention days must be CloudWatch-supported and at least minimum_log_retention_days."
  }
}

variable "kms_key_id" {
  description = "Optional customer-managed KMS key for the log group."
  type        = string
  default     = null

  validation {
    condition     = var.kms_key_id == null || length(trimspace(var.kms_key_id)) > 0
    error_message = "kms_key_id must be null or a non-empty KMS key ARN or ID."
  }
}

variable "require_encryption" {
  description = "Require a customer-managed KMS key for the log group. Enable for production deployments."
  type        = bool
  default     = false

  validation {
    condition     = !var.require_encryption || (var.kms_key_id != null && length(trimspace(var.kms_key_id)) > 0)
    error_message = "require_encryption=true requires a non-empty kms_key_id."
  }
}

variable "minimum_log_retention_days" {
  description = "Minimum allowed log retention. Set this to the approved production retention floor."
  type        = number
  default     = 1

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653], var.minimum_log_retention_days)
    error_message = "minimum_log_retention_days must be a CloudWatch-supported retention period."
  }
}

variable "log_subscription" {
  description = "Optional single CloudWatch Logs subscription destination. Null keeps log routing disabled."
  type = object({
    destination_arn = string
    role_arn        = string
    filter_pattern  = string
    distribution    = string
  })
  default = null

  validation {
    condition = var.log_subscription == null || (
      can(regex("^arn:[^:]+:[^:]+:", var.log_subscription.destination_arn)) &&
      length(trimspace(var.log_subscription.filter_pattern)) > 0 &&
      (length(trimspace(var.log_subscription.role_arn)) == 0 || can(regex("^arn:[^:]+:iam::[0-9]{12}:role/", var.log_subscription.role_arn))) &&
      (length(trimspace(var.log_subscription.role_arn)) > 0 || strcontains(var.log_subscription.destination_arn, ":lambda:")) &&
      contains(["Random", "ByLogStream"], var.log_subscription.distribution)
    )
    error_message = "Configured log_subscription requires an ARN destination, a non-empty filter pattern, a role ARN for non-Lambda destinations, and distribution Random or ByLogStream."
  }
}

variable "skip_destroy" {
  description = "Keep the log group when Terraform destroys the module. Keep true for production evidence retention."
  type        = bool
  default     = true
}

variable "metric_dimensions" {
  description = "Fixed low-cardinality CloudWatch dimensions. Do not include job, run, runner, request, repository, or secret identifiers."
  type        = map(string)
  default = {
    Environment = "dev"
  }

  validation {
    condition = (
      length(var.metric_dimensions) <= 3 &&
      alltrue([for key in keys(var.metric_dimensions) : contains(["Environment", "Provider", "Region"], key)]) &&
      alltrue([for value in values(var.metric_dimensions) : length(trimspace(value)) > 0 && length(value) <= 64])
    )
    error_message = "Metric dimensions may contain at most Environment, Provider, and Region, with non-empty values no longer than 64 characters."
  }
}

variable "alarm_period_seconds" {
  description = "Metric alarm period. This module intentionally uses one-minute evaluation granularity."
  type        = number
  default     = 60

  validation {
    condition     = var.alarm_period_seconds == 60
    error_message = "Alarm period seconds must remain 60 for the Phase 22 operational alarms."
  }
}

variable "alarms" {
  description = "Notification-only metric alarms. Each entry must use explicit M-of-N and missing-data settings."
  type = map(object({
    metric_name         = string
    statistic           = string
    threshold           = number
    comparison_operator = string
    evaluation_periods  = number
    datapoints_to_alarm = number
    treat_missing_data  = string
    unit                = string
    extended_statistic  = string
    description         = string
  }))
  default = {
    lifecycle-events = {
      metric_name         = "LifecycleEvents"
      statistic           = "Sum"
      threshold           = 1000
      comparison_operator = "GreaterThanThreshold"
      evaluation_periods  = 3
      datapoints_to_alarm = 2
      treat_missing_data  = "notBreaching"
      unit                = "Count"
      extended_statistic  = ""
      description         = "Unexpectedly high lifecycle event volume."
    }
    active-runners = {
      metric_name         = "RunnerCount"
      statistic           = "Maximum"
      threshold           = 1000
      comparison_operator = "GreaterThanThreshold"
      evaluation_periods  = 5
      datapoints_to_alarm = 3
      treat_missing_data  = "notBreaching"
      unit                = "Count"
      extended_statistic  = ""
      description         = "Unexpectedly high active runner count."
    }
    lifecycle-duration-p99 = {
      metric_name         = "LifecycleDuration"
      statistic           = ""
      threshold           = 300000
      comparison_operator = "GreaterThanThreshold"
      evaluation_periods  = 3
      datapoints_to_alarm = 2
      treat_missing_data  = "notBreaching"
      unit                = "Milliseconds"
      extended_statistic  = "p99"
      description         = "High p99 lifecycle duration in milliseconds."
    }
  }

  validation {
    condition = alltrue([
      for alarm in values(var.alarms) : (
        length(trimspace(alarm.metric_name)) > 0 &&
        alarm.evaluation_periods >= 2 &&
        alarm.datapoints_to_alarm >= 1 &&
        alarm.datapoints_to_alarm <= alarm.evaluation_periods &&
        contains(["missing", "notBreaching", "breaching", "ignore"], alarm.treat_missing_data) &&
        contains(["GreaterThanThreshold", "GreaterThanOrEqualToThreshold", "LessThanThreshold", "LessThanOrEqualToThreshold"], alarm.comparison_operator) &&
        (alarm.extended_statistic == "" || alarm.statistic == "")
      )
    ])
    error_message = "Each alarm needs valid M-of-N settings, explicit missing-data treatment, and must use either statistic or extended_statistic."
  }
}

variable "alarm_actions" {
  description = "Optional SNS or other notification ARNs. No lifecycle or compute actions are configured by this module."
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Additional tags applied to the log group."
  type        = map(string)
  default     = {}
}
