variable "table_name" {
  description = "DynamoDB table name for controller state."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]{3,255}$", var.table_name))
    error_message = "Table name must be 3-255 characters of letters, numbers, _, ., or -."
  }
}

variable "billing_mode" {
  description = "DynamoDB billing mode. The module is designed for on-demand billing."
  type        = string
  default     = "PAY_PER_REQUEST"

  validation {
    condition     = var.billing_mode == "PAY_PER_REQUEST"
    error_message = "Billing mode must be PAY_PER_REQUEST for this module."
  }
}

variable "ttl_attribute" {
  description = "Numeric Unix epoch-seconds attribute used for asynchronous DynamoDB TTL cleanup."
  type        = string
  default     = "expires_at"

  validation {
    condition     = can(regex("^[A-Za-z][A-Za-z0-9_]{0,254}$", var.ttl_attribute))
    error_message = "TTL attribute must be a valid DynamoDB attribute name."
  }
}

variable "enable_point_in_time_recovery" {
  description = "Enable DynamoDB point-in-time recovery."
  type        = bool
  default     = true
}

variable "enable_deletion_protection" {
  description = "Prevent accidental table deletion through the DynamoDB service setting."
  type        = bool
  default     = true
}

variable "enable_kms_encryption" {
  description = "Use server-side encryption for the table."
  type        = bool
  default     = true
}

variable "kms_key_arn" {
  description = "Optional customer-managed KMS key ARN. Null uses the AWS-owned DynamoDB key."
  type        = string
  default     = null

  validation {
    condition     = var.kms_key_arn == null || can(regex("^arn:[^:]+:kms:[^:]*:[0-9]{12}:key/.+$", var.kms_key_arn))
    error_message = "KMS key ARN must be a valid customer-managed KMS key ARN or null."
  }
}

variable "state_index_name" {
  description = "Name of the sparse state/event GSI."
  type        = string
  default     = "gsi1"

  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]{3,255}$", var.state_index_name))
    error_message = "State index name must be 3-255 valid DynamoDB index-name characters."
  }
}

variable "expiry_index_name" {
  description = "Name of the sparse expiry/reaper GSI."
  type        = string
  default     = "gsi2"

  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]{3,255}$", var.expiry_index_name))
    error_message = "Expiry index name must be 3-255 valid DynamoDB index-name characters."
  }
}

variable "tags" {
  description = "Tags applied to the DynamoDB table."
  type        = map(string)
  default     = {}
}
