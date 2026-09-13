packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = ">= 1.3.0"
    }
  }
}

variable "aws_region" {
  type        = string
  description = "AWS Region in which to build and publish the AMI."

  validation {
    condition     = can(regex("^[a-z]{2}(?:-gov)?-[a-z0-9-]+-[0-9]+$", var.aws_region))
    error_message = "The AWS region must be formatted like us-east-1."
  }
}

variable "vpc_id" {
  type        = string
  description = "VPC used for the temporary Packer build instance."

  validation {
    condition     = can(regex("^vpc-[0-9a-f]+$", var.vpc_id))
    error_message = "The VPC ID must be an EC2 VPC ID."
  }
}

variable "subnet_id" {
  type        = string
  description = "Subnet used for the temporary Packer build instance."

  validation {
    condition     = can(regex("^subnet-[0-9a-f]+$", var.subnet_id))
    error_message = "The subnet ID must be an EC2 subnet ID."
  }
}

variable "source_ami" {
  type        = string
  description = "Pinned Amazon Linux 2023 x86_64 source AMI ID for this Region."

  validation {
    condition     = can(regex("^ami-[0-9a-f]+$", var.source_ami))
    error_message = "The source AMI must be an EC2 AMI ID; pin this value rather than resolving a moving alias in CI."
  }
}

variable "iam_instance_profile" {
  type        = string
  description = "Least-privilege instance profile used only by the temporary build instance."

  validation {
    condition     = length(trimspace(var.iam_instance_profile)) > 0
    error_message = "The IAM instance profile is required; use a dedicated least-privilege profile."
  }
}

variable "security_group_id" {
  type        = string
  description = "Build security group that permits SSH only from the Packer builder."

  validation {
    condition     = can(regex("^sg-[0-9a-f]+$", var.security_group_id))
    error_message = "The security group ID must be an EC2 security group ID."
  }
}

variable "instance_type" {
  type        = string
  description = "Temporary instance type used while baking the image."
  default     = "t3.small"
}

variable "ssh_username" {
  type        = string
  description = "SSH user supplied by the Amazon Linux source AMI."
  default     = "ec2-user"
}

variable "ami_name_prefix" {
  type        = string
  description = "Prefix for the immutable AMI name."
  default     = "leo-runners-amazon-linux-x86_64"

  validation {
    condition     = can(regex("^[A-Za-z0-9][A-Za-z0-9._-]{2,99}$", var.ami_name_prefix))
    error_message = "The AMI name prefix must be 3-100 characters using letters, numbers, dot, underscore, or hyphen."
  }
}

source "amazon-ebs" "amazon_linux_x86_64" {
  region               = var.aws_region
  source_ami           = var.source_ami
  instance_type        = var.instance_type
  ssh_username         = var.ssh_username
  vpc_id               = var.vpc_id
  subnet_id            = var.subnet_id
  security_group_id    = var.security_group_id
  iam_instance_profile = var.iam_instance_profile
  ssh_interface        = "private_ip"
  ssh_timeout          = "10m"

  # Enforce IMDSv2 on the temporary build host and on the resulting AMI.
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
    instance_metadata_tags      = "disabled"
  }
  imds_support = "v2.0"

  ami_name        = "${var.ami_name_prefix}-${formatdate("YYYYMMDDhhmmss", timestamp())}"
  ami_description = "Immutable Amazon Linux 2023 x86_64 base image for ephemeral CI runners."

  launch_block_device_mappings {
    device_name           = "/dev/xvda"
    volume_size           = 30
    volume_type           = "gp3"
    delete_on_termination = true
    encrypted             = true
  }

  tags = {
    "leo-runners:managed-by" = "packer"
    "leo-runners:image"      = "ephemeral-ci-runner"
    "leo-runners:os"         = "amazon-linux-2023"
    "leo-runners:arch"       = "x86_64"
    "leo-runners:phase"      = "5"
  }

  snapshot_tags = {
    "leo-runners:managed-by" = "packer"
    "leo-runners:image"      = "ephemeral-ci-runner"
  }
}

build {
  name    = "leo-runners-amazon-linux-x86_64"
  sources = ["source.amazon-ebs.amazon_linux_x86_64"]

  provisioner "shell" {
    script            = "${path.root}/scripts/install-base-tools.sh"
    execute_command   = "sudo bash '{{ .Path }}'"
    expect_disconnect = false
  }

  provisioner "shell" {
    script          = "${path.root}/scripts/validate-image.sh"
    execute_command = "sudo bash '{{ .Path }}'"
  }

  post-processor "manifest" {
    output = "${path.root}/manifest.json"
  }
}
