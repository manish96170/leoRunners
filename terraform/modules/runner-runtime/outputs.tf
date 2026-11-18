output "role_arn" {
  description = "ARN of the ephemeral runner IAM role."
  value       = aws_iam_role.runner.arn
}

output "role_name" {
  description = "Name of the ephemeral runner IAM role."
  value       = aws_iam_role.runner.name
}

output "instance_profile_arn" {
  description = "ARN of the EC2 instance profile for the runner."
  value       = aws_iam_instance_profile.runner.arn
}

output "instance_profile_name" {
  description = "Name of the EC2 instance profile for the runner."
  value       = aws_iam_instance_profile.runner.name
}
