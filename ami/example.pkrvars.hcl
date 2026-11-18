# Copy this file to a private, ignored file and replace every placeholder.
# The source AMI must be a pinned Amazon Linux 2023 x86_64 AMI in aws_region.
aws_region           = "us-east-1"
vpc_id               = "vpc-0123456789abcdef0"
subnet_id            = "subnet-0123456789abcdef0"
source_ami           = "ami-0123456789abcdef0"
iam_instance_profile = "leo-runners-ami-builder"
security_group_id    = "sg-0123456789abcdef0"

instance_type = "t3.small"
ssh_username  = "ec2-user"
