output "alb_arn" {
  description = "The adopted ALB ARN, echoed for composition."
  value       = var.alb_arn
}

output "https_listener_arn" {
  description = "ARN of the HTTPS listener to attach service rules to (created or adopted), or null when none is configured."
  value       = var.enable_https_listener ? aws_lb_listener.https[0].arn : var.existing_https_listener_arn
}

output "http_redirect_listener_arn" {
  description = "ARN of the HTTP-to-HTTPS redirect listener, or null when disabled."
  value       = try(aws_lb_listener.http_redirect[0].arn, null)
}

output "listener_rule_priority_ranges" {
  description = "The validated, non-overlapping listener-rule priority convention."
  value       = var.listener_rule_priority_ranges
}
