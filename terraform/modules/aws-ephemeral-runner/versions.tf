terraform {
  required_version = ">= 1.5.0"

  # Declared for consistency with the other aws-* modules, which this contract
  # is consumed alongside. The module itself creates no AWS resources.
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0, < 7.0"
    }
  }
}
