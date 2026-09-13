output "table_name" {
  description = "DynamoDB controller-state table name."
  value       = aws_dynamodb_table.controller_state.name
}

output "table_arn" {
  description = "DynamoDB controller-state table ARN."
  value       = aws_dynamodb_table.controller_state.arn
}

output "table_id" {
  description = "DynamoDB controller-state table ID."
  value       = aws_dynamodb_table.controller_state.id
}

output "state_index_name" {
  description = "Sparse state/event GSI name."
  value       = var.state_index_name
}

output "expiry_index_name" {
  description = "Sparse expiry/reaper GSI name."
  value       = var.expiry_index_name
}

output "state_index_arn" {
  description = "Sparse state/event GSI ARN for scoped data-plane access."
  value       = "${aws_dynamodb_table.controller_state.arn}/index/${var.state_index_name}"
}

output "expiry_index_arn" {
  description = "Sparse expiry/reaper GSI ARN for scoped data-plane access."
  value       = "${aws_dynamodb_table.controller_state.arn}/index/${var.expiry_index_name}"
}
