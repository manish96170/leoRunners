# Observability Module

This module creates the CloudWatch operational surface for the Leo Runners
controller:

- one encrypted-when-configured CloudWatch log group with explicit retention;
- notification-only metric alarms at a fixed 60-second period with explicit
  M-of-N and missing-data behavior; and
- one dashboard with alarm status and low-cardinality metric widgets.
- an optional single subscription filter for routing redacted logs to an
  explicitly supplied Lambda, Kinesis, or Firehose destination.

The default dimensions are limited to `Environment`, `Provider`, and `Region`.
Do not add job IDs, runner IDs, repository names, request IDs, credentials, or
payload values. Those belong in redacted structured logs and events, not metric
labels. The module has no IAM policy resources and no lifecycle, scaling,
termination, or remediation actions.

Production routing is disabled unless `log_subscription` is set. The
destination and delivery role are supplied by the caller; this module does
not create or grant access to either resource. Lambda destinations do not use
`role_arn`; Kinesis and Firehose destinations require the caller's delivery
role ARN. Only one subscription filter is created so the log-group filter
limit remains explicit.

## Usage

```hcl
module "observability" {
  source = "../../modules/observability"

  name            = "leo-runners-controller"
  namespace       = "LeoRunners/Controller"
  kms_key_id      = var.observability_kms_key_id
  metric_dimensions = {
    Environment = "production"
    Provider    = "aws"
    Region      = "us-east-1"
  }
  alarm_actions = [var.operations_sns_topic_arn]
  require_encryption         = true
  minimum_log_retention_days = 30
  log_subscription = {
    destination_arn = var.redacted_log_delivery_arn
    role_arn        = var.redacted_log_delivery_role_arn
    filter_pattern  = "{ $.sensitive = false }"
    distribution    = "ByLogStream"
  }
  tags = {
    owner = "platform-team"
  }
}
```

`alarm_actions` is intentionally an external notification input. Supplying an
SNS topic does not grant this module IAM permissions or create an SNS topic.
Review alarm thresholds against observed baselines before applying. The
duration alarm uses p99 rather than Average so tail latency is visible.
`require_encryption` and `minimum_log_retention_days` make production policy
explicit at plan time. Keep subscription filter patterns aligned with the
controller's redaction contract; this module cannot inspect payload contents.

## Validation

```text
terraform fmt -check -recursive
terraform init -backend=false
terraform validate
./validate.sh
```

Inspect the plan for one log group, the expected alarms, at most one explicit
subscription filter, no compute actions, and no IAM resources. Keep plans,
state, tfvars, and KMS identifiers private.
