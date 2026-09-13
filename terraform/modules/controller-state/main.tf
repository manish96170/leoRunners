locals {
  common_tags = merge(
    {
      managed_by = "terraform"
      component  = "controller-state"
      data_class = "controller-state"
    },
    var.tags,
  )
}

resource "aws_dynamodb_table" "controller_state" {
  name         = var.table_name
  billing_mode = var.billing_mode
  hash_key     = "pk"
  range_key    = "sk"

  # DynamoDB requires every table/index key to be declared here. The
  # controller writes the GSI attributes only on records that need an index,
  # making both indexes sparse.
  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  attribute {
    name = "gsi1pk"
    type = "S"
  }

  attribute {
    name = "gsi1sk"
    type = "S"
  }

  attribute {
    name = "gsi2pk"
    type = "S"
  }

  attribute {
    name = "gsi2sk"
    type = "S"
  }

  global_secondary_index {
    name            = var.state_index_name
    hash_key        = "gsi1pk"
    range_key       = "gsi1sk"
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = var.expiry_index_name
    hash_key        = "gsi2pk"
    range_key       = "gsi2sk"
    projection_type = "ALL"
  }

  ttl {
    attribute_name = var.ttl_attribute
    enabled        = true
  }

  point_in_time_recovery {
    enabled = var.enable_point_in_time_recovery
  }

  server_side_encryption {
    enabled     = var.enable_kms_encryption
    kms_key_arn = var.enable_kms_encryption ? var.kms_key_arn : null
  }

  deletion_protection_enabled = var.enable_deletion_protection
  tags                        = local.common_tags
}
