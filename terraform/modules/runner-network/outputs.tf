output "vpc_id" {
  description = "Existing VPC ID supplied to the module."
  value       = var.vpc_id
}

output "runner_subnet_ids" {
  description = "Existing runner subnet IDs supplied to the module."
  value       = var.runner_subnet_ids
}

output "security_group_ids" {
  description = "Existing and/or module-created security groups for runner attachment."
  value       = concat(var.security_group_ids, var.create_runner_security_group ? [aws_security_group.runner[0].id] : [])
}

output "runner_security_group_id" {
  description = "Created ingress-free runner security-group ID, or null when creation is disabled."
  value       = var.create_runner_security_group ? aws_security_group.runner[0].id : null
}

output "interface_endpoint_ids" {
  description = "Created interface endpoint IDs keyed by AWS service suffix."
  value       = { for service, endpoint in aws_vpc_endpoint.interface : service => endpoint.id }
}

output "gateway_endpoint_ids" {
  description = "Created gateway endpoint IDs keyed by AWS service suffix."
  value       = { for service, endpoint in aws_vpc_endpoint.gateway : service => endpoint.id }
}

output "endpoint_security_group_id" {
  description = "Created endpoint security-group ID, or null when no interface endpoints are created."
  value       = length(aws_security_group.endpoint) > 0 ? aws_security_group.endpoint[0].id : null
}

output "nat_gateway_id" {
  description = "Created NAT gateway ID, or null when NAT is disabled."
  value       = var.enable_nat_gateway ? aws_nat_gateway.runner[0].id : null
}

output "nat_eip_public_ip" {
  description = "Public IP allocated to the NAT gateway, or null when NAT is disabled."
  value       = var.enable_nat_gateway ? aws_eip.nat[0].public_ip : null
}

output "cost_warnings" {
  description = "Operational cost warnings for the selected network options."
  value = concat(
    var.enable_nat_gateway ? ["NAT gateway enabled: expect hourly NAT gateway and per-GB data-processing charges; consider a shared or endpoint-based egress design."] : [],
    var.enable_vpc_endpoints && length(local.interface_services) > 0 ? ["Interface endpoints enabled: expect per-endpoint hourly and data-processing charges in each endpoint AZ."] : [],
    var.enable_vpc_endpoints && length(local.gateway_services) > 0 ? ["Gateway endpoints enabled: no hourly endpoint charge, but route-table scope and endpoint policies must be reviewed."] : [],
  )
}

output "reachability_labels" {
  description = "Stable labels for offline reachability and billing reconciliation evidence."
  value = {
    egress_mode  = var.egress_mode
    subnet_class = "private-preferred"
    dns_mode     = var.enable_dns_support && var.enable_dns_hostnames ? "cloud-resolver-required" : "invalid"
    isolation    = "inbound-deny-imdsv2"
    cost_labels = concat(
      contains(["nat", "hybrid"], var.egress_mode) ? ["nat-hourly", "nat-processing-per-gb"] : [],
      contains(["vpc-endpoint", "hybrid"], var.egress_mode) ? ["endpoint-hourly-or-route-scope", "endpoint-processing-per-gb"] : [],
      var.egress_mode == "approved-proxy" ? ["proxy-service-cost", "proxy-processing-cost"] : [],
    )
  }
}
