locals {
  log_group_name = coalesce(var.log_group_name, "/leo-runners/${var.name}")
  common_tags = merge({
    managed_by = "terraform"
    component  = "controller-observability"
    data_class = "operational-telemetry"
  }, var.tags)

  alarm_metrics = [for alarm in values(var.alarms) : concat(
    [var.namespace, alarm.metric_name],
    flatten([for key in sort(keys(var.metric_dimensions)) : [key, var.metric_dimensions[key]]]),
  )]
}

resource "aws_cloudwatch_log_group" "controller" {
  name              = local.log_group_name
  retention_in_days = var.log_retention_days
  kms_key_id        = var.kms_key_id
  skip_destroy      = var.skip_destroy
  tags              = local.common_tags
}

resource "aws_cloudwatch_log_subscription_filter" "controller" {
  for_each = var.log_subscription == null ? {} : { configured = var.log_subscription }

  name            = "${var.name}-log-routing"
  log_group_name  = aws_cloudwatch_log_group.controller.name
  destination_arn = each.value.destination_arn
  filter_pattern  = each.value.filter_pattern
  distribution    = each.value.distribution
  role_arn        = each.value.role_arn == "" ? null : each.value.role_arn
}

resource "aws_cloudwatch_metric_alarm" "operational" {
  for_each = var.alarms

  alarm_name          = "${var.name}-${each.key}"
  alarm_description   = each.value.description
  namespace           = var.namespace
  metric_name         = each.value.metric_name
  dimensions          = var.metric_dimensions
  period              = var.alarm_period_seconds
  statistic           = each.value.extended_statistic == "" ? each.value.statistic : null
  extended_statistic  = each.value.extended_statistic == "" ? null : each.value.extended_statistic
  unit                = each.value.unit
  threshold           = each.value.threshold
  comparison_operator = each.value.comparison_operator
  evaluation_periods  = each.value.evaluation_periods
  datapoints_to_alarm = each.value.datapoints_to_alarm
  treat_missing_data  = each.value.treat_missing_data

  # These are deliberately notification-only. Termination, scaling, and
  # remediation require a separate reviewed automation boundary.
  alarm_actions = var.alarm_actions
}

resource "aws_cloudwatch_dashboard" "controller" {
  dashboard_name = "${var.name}-observability"
  dashboard_body = jsonencode({
    start          = "-PT8H"
    periodOverride = "INHERIT"
    widgets = [
      {
        type   = "text"
        width  = 24
        height = 2
        x      = 0
        y      = 0
        properties = {
          markdown = "# ${var.name} controller observability\n\nNamespace: `${var.namespace}` | Log group: `${local.log_group_name}`"
        }
      },
      {
        type   = "alarm"
        width  = 24
        height = 6
        x      = 0
        y      = 2
        properties = {
          title  = "Operational alarms"
          alarms = [for alarm in aws_cloudwatch_metric_alarm.operational : alarm.arn]
        }
      },
      {
        type   = "metric"
        width  = 24
        height = 6
        x      = 0
        y      = 8
        properties = {
          title   = "Controller metrics"
          view    = "timeSeries"
          stacked = false
          region  = data.aws_region.current.name
          period  = 60
          stat    = "Sum"
          metrics = local.alarm_metrics
        }
      }
    ]
  })
}

data "aws_region" "current" {}
