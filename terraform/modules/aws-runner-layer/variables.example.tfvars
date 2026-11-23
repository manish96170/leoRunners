# Non-secret example inputs for the aws-runner-layer module. Every value is a
# placeholder; supply your own IDs, ranges and tags.

name_prefix   = "leo-runners"
ami_id        = "ami-0123456789abcdef0"
instance_type = "c7i.xlarge"

vpc_id    = "vpc-0123456789abcdef0"
subnet_id = "subnet-0123456789abcdef0"

# The created security group has no inbound rules. Egress is exactly this.
egress_https_cidr_blocks = ["10.0.0.0/8"]

root_volume_size_gb  = 100
max_lifetime_minutes = 120

request_id       = "example-request-1"
cleanup_deadline = "2030-01-01T00:00:00Z"

tags = {
  Environment = "example"
}
