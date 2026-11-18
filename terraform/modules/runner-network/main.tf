locals {
  common_tags = merge(
    {
      "managed-by" = "terraform"
      "component"  = "ephemeral-ci-runner-network"
    },
    var.tags,
  )

  dns_cidr = coalesce(var.dns_egress_cidr, var.vpc_cidr_block, "0.0.0.0/0")

  interface_services = var.enable_vpc_endpoints ? var.interface_endpoint_services : toset([])
  gateway_services   = var.enable_vpc_endpoints ? var.gateway_endpoint_services : toset([])
}

resource "aws_security_group" "runner" {
  count = var.create_runner_security_group ? 1 : 0

  name        = coalesce(var.runner_security_group_name, "${var.name}-runner")
  description = "Ingress-free security group for ephemeral CI runners."
  vpc_id      = var.vpc_id

  revoke_rules_on_delete = true

  # No ingress blocks are intentional. Runners are reached through outbound
  # control-plane connections and do not expose SSH or an application port.
  egress {
    description = "HTTPS for GitHub, package registries, and approved APIs"
    protocol    = "tcp"
    from_port   = 443
    to_port     = 443
    cidr_blocks = [var.https_egress_cidr]
  }

  egress {
    description = "DNS over UDP to the VPC resolver"
    protocol    = "udp"
    from_port   = 53
    to_port     = 53
    cidr_blocks = [local.dns_cidr]
  }

  egress {
    description = "DNS over TCP to the VPC resolver"
    protocol    = "tcp"
    from_port   = 53
    to_port     = 53
    cidr_blocks = [local.dns_cidr]
  }

  tags = merge(local.common_tags, { Name = coalesce(var.runner_security_group_name, "${var.name}-runner") })

  lifecycle {
    precondition {
      condition     = var.vpc_id != null && trimspace(var.vpc_id) != ""
      error_message = "vpc_id is required when create_runner_security_group is true."
    }
    precondition {
      condition     = var.vpc_cidr_block != null || var.dns_egress_cidr != null
      error_message = "vpc_cidr_block or dns_egress_cidr is required when creating the runner security group."
    }
  }
}

resource "aws_security_group" "endpoint" {
  count = var.enable_vpc_endpoints && length(local.interface_services) > 0 ? 1 : 0

  name        = "${var.name}-endpoint"
  description = "Private interface endpoint access for ephemeral CI runners."
  vpc_id      = var.vpc_id

  revoke_rules_on_delete = true

  ingress {
    description     = "HTTPS from the runner security group or constrained endpoint CIDR"
    protocol        = "tcp"
    from_port       = 443
    to_port         = 443
    security_groups = var.create_runner_security_group ? [aws_security_group.runner[0].id] : []
    cidr_blocks     = var.create_runner_security_group ? [] : [coalesce(var.endpoint_security_group_ingress_cidr, var.vpc_cidr_block, "0.0.0.0/0")]
  }

  egress {
    description = "Endpoint response traffic inside the VPC"
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = [coalesce(var.vpc_cidr_block, "0.0.0.0/0")]
  }

  tags = merge(local.common_tags, { Name = "${var.name}-endpoint" })

  lifecycle {
    precondition {
      condition     = var.vpc_id != null && trimspace(var.vpc_id) != ""
      error_message = "vpc_id is required when VPC interface endpoints are enabled."
    }
    precondition {
      condition     = var.create_runner_security_group || (var.endpoint_security_group_ingress_cidr != null && trimspace(var.endpoint_security_group_ingress_cidr) != "") || (var.vpc_cidr_block != null && trimspace(var.vpc_cidr_block) != "")
      error_message = "Provide a created runner security group, endpoint_security_group_ingress_cidr, or vpc_cidr_block to constrain endpoint ingress."
    }
  }
}

resource "aws_vpc_endpoint" "interface" {
  for_each = local.interface_services

  vpc_id              = var.vpc_id
  service_name        = "com.amazonaws.${data.aws_region.current.region}.${each.key}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = var.endpoint_subnet_ids
  security_group_ids  = [aws_security_group.endpoint[0].id]
  private_dns_enabled = true

  tags = merge(local.common_tags, { Name = "${var.name}-${each.key}" })

  lifecycle {
    precondition {
      condition     = length(var.endpoint_subnet_ids) > 0
      error_message = "endpoint_subnet_ids is required for interface endpoints."
    }
  }
}

resource "aws_vpc_endpoint" "gateway" {
  for_each = local.gateway_services

  vpc_id            = var.vpc_id
  service_name      = "com.amazonaws.${data.aws_region.current.region}.${each.key}"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = var.endpoint_route_table_ids

  tags = merge(local.common_tags, { Name = "${var.name}-${each.key}" })

  lifecycle {
    precondition {
      condition     = length(var.endpoint_route_table_ids) > 0
      error_message = "endpoint_route_table_ids is required for gateway endpoints."
    }
  }
}

data "aws_region" "current" {}

resource "aws_eip" "nat" {
  count = var.enable_nat_gateway ? 1 : 0

  domain = "vpc"
  tags   = merge(local.common_tags, { Name = "${var.name}-nat-eip" })

  lifecycle {
    precondition {
      condition     = var.nat_public_subnet_id != null && trimspace(var.nat_public_subnet_id) != ""
      error_message = "nat_public_subnet_id is required when enable_nat_gateway is true."
    }
  }
}

resource "aws_nat_gateway" "runner" {
  count = var.enable_nat_gateway ? 1 : 0

  allocation_id = aws_eip.nat[0].id
  subnet_id     = var.nat_public_subnet_id

  connectivity_type = "public"
  tags              = merge(local.common_tags, { Name = "${var.name}-nat" })

  depends_on = [aws_eip.nat]

  lifecycle {
    precondition {
      condition     = length(var.nat_private_route_table_ids) > 0
      error_message = "nat_private_route_table_ids is required when enable_nat_gateway is true."
    }
  }
}

resource "aws_route" "nat_default" {
  for_each = var.enable_nat_gateway ? toset(var.nat_private_route_table_ids) : toset([])

  route_table_id         = each.key
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.runner[0].id
}
