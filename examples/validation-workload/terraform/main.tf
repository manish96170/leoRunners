# A temporary "integration environment" that needs no cloud account: the legacy
# Terraform version, a local state file, two old providers, apply, and destroy.
# It exercises the same steps a real temporary environment goes through
# (init, apply, health check, teardown) without creating anything remote.

terraform {
  required_version = ">= 0.12.0, < 0.13.0"
}

provider "null" {
  version = "~> 2.1"
}

provider "local" {
  version = "~> 1.4"
}

variable "run_id" {
  type = string
}

variable "out_dir" {
  type = string
}

resource "null_resource" "environment" {
  triggers = {
    run_id = var.run_id
  }
}

resource "local_file" "marker" {
  filename = "${var.out_dir}/environment.json"
  content  = jsonencode({ status = "up", run_id = var.run_id })

  depends_on = [null_resource.environment]
}

output "marker" {
  value = local_file.marker.filename
}
