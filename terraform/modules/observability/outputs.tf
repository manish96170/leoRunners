output "log_group_name" {
  description = "CloudWatch Logs group receiving controller telemetry."
  value       = aws_cloudwatch_log_group.controller.name
}

output "log_group_arn" {
  description = "ARN of the controller telemetry log group."
  value       = aws_cloudwatch_log_group.controller.arn
}

output "log_subscription_filter_name" {
  description = "Optional subscription filter name, or null when production log routing is disabled."
  value       = try(aws_cloudwatch_log_subscription_filter.controller["configured"].name, null)
}

output "alarm_arns" {
  description = "Notification-only operational alarm ARNs keyed by configured alarm name."
  value       = { for key, alarm in aws_cloudwatch_metric_alarm.operational : key => alarm.arn }
}

output "dashboard_name" {
  description = "CloudWatch dashboard name."
  value       = aws_cloudwatch_dashboard.controller.dashboard_name
}

output "dashboard_arn" {
  description = "CloudWatch dashboard ARN."
  value       = aws_cloudwatch_dashboard.controller.dashboard_arn
}
