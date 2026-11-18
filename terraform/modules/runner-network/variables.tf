variable "name" {
  description = "Name prefix for runner-network resources."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9-]{0,62}$", var.name))
    error_message = "name must be 1-63 characters, start with a letter or number, and contain only letters, numbers, and hyphens."
  }
}

variable "vpc_id" {
  description = "Existing VPC ID. This module never creates a VPC. Required when any resource is created."
  type        = string
  default     = null
  nullable    = true
}

variable "vpc_cidr_block" {
  description = "CIDR used for DNS egress and endpoint security-group egress when a runner security group is created."
  type        = string
  default     = null
  nullable    = true
}

variable "runner_subnet_ids" {
  description = "Existing subnet IDs where ephemeral runners may launch."
  type        = list(string)
  default     = []
}

variable "security_group_ids" {
  description = "Existing security-group IDs to attach to runners."
  type        = list(string)
  default     = []
}

variable "create_runner_security_group" {
  description = "Explicitly create an ingress-free security group with HTTPS and DNS egress."
  type        = bool
  default     = false
}

variable "runner_security_group_name" {
  description = "Optional name for the created runner security group."
  type        = string
  default     = null
}

variable "https_egress_cidr" {
  description = "IPv4 destination allowed for runner TCP/443 egress. Prefer a controlled egress path or endpoint policy where possible."
  type        = string
  default     = "0.0.0.0/0"
}

variable "dns_egress_cidr" {
  description = "IPv4 destination allowed for runner TCP/UDP 53 egress. Defaults to vpc_cidr_block when supplied."
  type        = string
  default     = null
  nullable    = true
}

variable "enable_vpc_endpoints" {
  description = "Explicitly create the requested interface and gateway VPC endpoints."
  type        = bool
  default     = false
}

variable "interface_endpoint_services" {
  description = "AWS service suffixes for interface endpoints, for example ssm, ssmmessages, ec2messages, or logs."
  type        = set(string)
  default     = []
}

variable "gateway_endpoint_services" {
  description = "AWS service suffixes for gateway endpoints, normally s3 and optionally dynamodb."
  type        = set(string)
  default     = []
}

variable "endpoint_subnet_ids" {
  description = "Subnets for interface endpoint ENIs. Required when interface_endpoint_services is non-empty."
  type        = list(string)
  default     = []
}

variable "endpoint_route_table_ids" {
  description = "Route tables for gateway endpoints. Required when gateway_endpoint_services is non-empty."
  type        = list(string)
  default     = []
}

variable "enable_nat_gateway" {
  description = "Explicitly create one managed NAT gateway and route private route tables through it. NAT has hourly and data-processing charges."
  type        = bool
  default     = false
}

variable "nat_public_subnet_id" {
  description = "Existing public subnet ID for the single NAT gateway. The module does not create a VPC or public subnet."
  type        = string
  default     = null
  nullable    = true
}

variable "nat_private_route_table_ids" {
  description = "Existing private route table IDs that should receive a default route through the created NAT gateway."
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Additional tags applied to created security groups, endpoints, EIP, and NAT gateway."
  type        = map(string)
  default     = {}
}

variable "egress_mode" {
  description = "Declared egress topology for reachability evidence. Metadata only; it does not create or route resources."
  type        = string
  default     = "nat"

  validation {
    condition     = contains(["nat", "vpc-endpoint", "hybrid", "approved-proxy"], var.egress_mode)
    error_message = "egress_mode must be nat, vpc-endpoint, hybrid, or approved-proxy."
  }
}

variable "enable_dns_support" {
  description = "Documentation-only guard for the existing VPC DNS requirement. The module does not mutate the VPC."
  type        = bool
  default     = true

  validation {
    condition     = var.enable_dns_support
    error_message = "The existing VPC must provide DNS support for runner name resolution."
  }
}

variable "enable_dns_hostnames" {
  description = "Documentation-only guard for the existing VPC DNS hostname requirement. The module does not mutate the VPC."
  type        = bool
  default     = true

  validation {
    condition     = var.enable_dns_hostnames
    error_message = "The existing VPC must provide DNS hostnames for the runner network configuration."
  }
}

variable "endpoint_security_group_ingress_cidr" {
  description = "Optional IPv4 CIDR allowed to reach interface endpoints on TCP/443. Defaults to the VPC CIDR; prefer runner-only security-group ingress using endpoint_security_group_source_security_group_id."
  type        = string
  default     = null
  nullable    = true
}
