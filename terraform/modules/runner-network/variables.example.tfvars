name               = "leo-runners"
vpc_id             = "vpc-0123456789abcdef0"
vpc_cidr_block     = "10.20.0.0/16"
runner_subnet_ids  = ["subnet-0123456789abcdef0"]
security_group_ids = ["sg-0123456789abcdef0"]

# Keep false when using existing security groups.
create_runner_security_group = false

# Enable only after reviewing endpoint pricing and route-table ownership.
enable_vpc_endpoints        = false
interface_endpoint_services = []
gateway_endpoint_services   = []
endpoint_subnet_ids         = []
endpoint_route_table_ids    = []

# NAT is a paid option and is disabled by default.
enable_nat_gateway          = false
nat_public_subnet_id        = null
nat_private_route_table_ids = []

tags = {
  environment = "dev"
}
