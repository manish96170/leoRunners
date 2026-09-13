data "aws_partition" "current" {}

data "aws_iam_policy" "ssm_core" {
  count = var.enable_ssm ? 1 : 0
  name  = "AmazonSSMManagedInstanceCore"
}

locals {
  common_tags = merge(
    {
      "managed-by" = "terraform"
      "component"  = "ephemeral-ci-runner"
    },
    var.tags,
  )
}

resource "aws_iam_role" "runner" {
  name                 = var.name
  path                 = var.path
  permissions_boundary = var.permissions_boundary_arn
  description          = "Minimal runtime role for an ephemeral CI runner."

  # EC2 is the only principal allowed to assume this role. Workload access
  # belongs in a separate, explicitly approved identity or OIDC trust.
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "Ec2Only"
      Effect = "Allow"
      Principal = {
        Service = "ec2.${data.aws_partition.current.dns_suffix}"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "ssm_core" {
  count      = var.enable_ssm ? 1 : 0
  role       = aws_iam_role.runner.name
  policy_arn = data.aws_iam_policy.ssm_core[0].arn
}

resource "aws_iam_instance_profile" "runner" {
  name = var.name
  path = var.path
  role = aws_iam_role.runner.name
  tags = local.common_tags
}
