# Documentation-only example values. Replace placeholders in an ignored local
# tfvars file after the deployable module contract has been reviewed.

table_name                    = "leo-runners-controller-state"
billing_mode                  = "PAY_PER_REQUEST"
ttl_attribute                 = "expires_at"
enable_point_in_time_recovery = true
enable_deletion_protection    = true
enable_kms_encryption         = true
kms_key_arn                   = "arn:aws:kms:us-east-1:123456789012:key/REPLACE_ME"
state_index_name              = "gsi1"
expiry_index_name             = "gsi2"

tags = {
  owner       = "platform-team"
  environment = "staging"
}
