#!/usr/bin/env bash
# Creates the local AWS resources in floci: the controller-state table (from the
# repository's own Terraform module), a launch template for runner instances,
# and a Secrets Manager secret for the GitHub App private key.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
need docker; need terraform; need openssl
resolve_arch
require_floci

work="$REHEARSAL_HOME/tf"
mkdir -p "$work"
cat > "$work/main.tf" <<TF
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0, < 7.0"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "000000000000"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  endpoints {
    dynamodb = "$AWS_ENDPOINT_URL"
  }
}

module "state" {
  source                        = "$REPO_ROOT/terraform/modules/controller-state"
  table_name                    = "$TABLE_NAME"
  enable_point_in_time_recovery = false
  enable_deletion_protection    = false
  enable_kms_encryption         = false
}
TF
(
  cd "$work"
  terraform init -backend=false -input=false >/dev/null
  # Re-running setup must be safe: adopt a table that exists in floci but not yet
  # in this directory's Terraform state.
  if ! terraform state list 2>/dev/null | grep -q 'aws_dynamodb_table.controller_state' \
    && aws dynamodb describe-table --table-name "$TABLE_NAME" >/dev/null 2>&1; then
    terraform import -input=false module.state.aws_dynamodb_table.controller_state "$TABLE_NAME" >/dev/null
  fi
  terraform apply -auto-approve -input=false >/dev/null
)
log "state table $TABLE_NAME ready (module: terraform/modules/controller-state)"

aws ec2 describe-launch-templates --launch-template-names "$LAUNCH_TEMPLATE_NAME" >/dev/null 2>&1 \
  && aws ec2 delete-launch-template --launch-template-name "$LAUNCH_TEMPLATE_NAME" >/dev/null
lt_id=$(aws ec2 create-launch-template --launch-template-name "$LAUNCH_TEMPLATE_NAME" \
  --launch-template-data "{\"ImageId\":\"$RUNNER_AMI\",\"InstanceType\":\"$RUNNER_INSTANCE_TYPE\",\"MetadataOptions\":{\"HttpTokens\":\"required\",\"HttpEndpoint\":\"enabled\"}}" \
  --query 'LaunchTemplate.LaunchTemplateId' --output text)
log "launch template $lt_id ($RUNNER_AMI, $RUNNER_INSTANCE_TYPE, IMDSv2 required)"

secret_name=leo-rehearsal/github-app-private-key
aws secretsmanager describe-secret --secret-id "$secret_name" >/dev/null 2>&1 \
  || aws secretsmanager create-secret --name "$secret_name" --description "GitHub App private key (local rehearsal)" >/dev/null
log "secret container $secret_name ready (store the key with store-app-key.sh)"

cat > "$REHEARSAL_HOME/state.env" <<ENV
LAUNCH_TEMPLATE_ID=$lt_id
RUNNER_ARCH_LABEL=$RUNNER_ARCH_LABEL
SECRET_NAME=$secret_name
ENV
log "setup complete"
