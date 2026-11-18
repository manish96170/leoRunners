# Non-secret example inputs for the aws-ephemeral-runner contract. All values
# are placeholders.

name_prefix = "leo-runners"
ami_id      = "ami-0123456789abcdef0"

instance_types     = ["c7i.xlarge", "c6i.xlarge"]
subnet_ids         = ["subnet-0123456789abcdef0", "subnet-0fedcba9876543210"]
security_group_ids = ["sg-0123456789abcdef0"]

runner_role_arn             = "arn:aws:iam::111122223333:role/example-leo-runner"
runner_instance_profile_arn = "arn:aws:iam::111122223333:instance-profile/example-leo-runner"
controller_role_arn         = "arn:aws:iam::111122223333:role/example-leo-controller"

root_volume_size_gb  = 100
max_lifetime_minutes = 120

request_id       = "example-request-1"
cleanup_deadline = "2030-01-01T00:00:00Z"

tags = {
  Environment = "example"
  Owner       = "example-team"
}
