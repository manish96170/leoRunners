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

variable "extra_tags" {
  type        = map(string)
  default     = {}
  description = "Extra tags (for example a request ID and a cleanup deadline) applied to the temporary build instance, its volumes, the AMI and its snapshots, so everything this build creates can be found and removed by the same tag intersection. Keys under the leo-runners: prefix are reserved."

  validation {
    condition     = alltrue([for k, v in var.extra_tags : !startswith(lower(k), "leo-runners:") && !startswith(lower(k), "aws:") && length(k) >= 1 && length(k) <= 128 && length(v) <= 256])
    error_message = "The extra_tags keys must not use the reserved leo-runners: or aws: prefixes."
  }
}

variable "extension_script" {
  type        = string
  default     = ""
  description = "Optional path to a script that adds workload tools (a browser, Docker, system libraries) to the image. It runs as root after the base tools and before validation, so it cannot leave a runner, credentials or SSH keys behind. Empty runs nothing."
}

variable "extension_environment" {
  type        = map(string)
  default     = {}
  description = "Environment variables passed to the extension script, for example pinned tool versions. Names must be upper case letters, digits and underscores."

  validation {
    condition     = alltrue([for k, v in var.extension_environment : can(regex("^[A-Z_][A-Z0-9_]*$", k)) && !strcontains(v, "\n")])
    error_message = "The extension_environment names must match ^[A-Z_][A-Z0-9_]*$ and every value must be a single line."
  }
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

locals {
  extension_script = var.extension_script != "" ? var.extension_script : "${path.root}/scripts/extension-none.sh"
  build_tags       = merge(var.extra_tags, { "leo-runners:managed-by" = "packer" })
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

  tags = merge(var.extra_tags, {
    "leo-runners:managed-by" = "packer"
    "leo-runners:image"      = "ephemeral-ci-runner"
    "leo-runners:os"         = "amazon-linux-2023"
    "leo-runners:arch"       = "x86_64"
    "leo-runners:phase"      = "5"
  })

  snapshot_tags = merge(var.extra_tags, {
    "leo-runners:managed-by" = "packer"
    "leo-runners:image"      = "ephemeral-ci-runner"
  })

  # The temporary build instance and its volume carry the same request tags, so
  # a failed or interrupted build can be cleaned up by tag, not by guessing.
  run_tags        = merge(var.extra_tags, { "leo-runners:managed-by" = "packer", "leo-runners:role" = "image-builder" })
  run_volume_tags = merge(var.extra_tags, { "leo-runners:managed-by" = "packer", "leo-runners:role" = "image-builder" })
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
    script            = local.extension_script
    environment_vars  = [for k, v in var.extension_environment : "${k}=${v}"]
    execute_command   = "sudo -E bash '{{ .Path }}'"
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
