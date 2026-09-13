output "role_name" {
  description = "Name of the controller runtime role."
  value       = aws_iam_role.controller.name
}

output "role_id" {
  description = "ID of the controller runtime role."
  value       = aws_iam_role.controller.id
}

output "role_arn" {
  description = "ARN of the controller runtime role."
  value       = aws_iam_role.controller.arn
}

output "role_unique_id" {
  description = "Stable unique ID of the controller runtime role."
  value       = aws_iam_role.controller.unique_id
}

output "assume_role_policy_json" {
  description = "Rendered trust policy for review and audit."
  value       = local.assume_role_policy
  sensitive   = true
}

output "generated_inline_policy_id" {
  description = "ID of the externally generated inline runtime policy, or null when no policy was supplied."
  value       = try(aws_iam_role_policy.generated_runtime[0].id, null)
}

output "generated_policy_attachment_arns" {
  description = "ARNs of externally managed generated policies attached by this module."
  value       = sort(tolist(var.generated_policy_arns))
}
