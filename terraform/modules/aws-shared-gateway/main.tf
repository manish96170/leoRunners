locals {
  common_tags = merge(
    {
      "managed-by" = "terraform"
      "component"  = "leo-runners-shared-gateway"
    },
    var.tags,
  )

  https_source_count = length(var.https_ingress_cidr_blocks) + length(var.https_ingress_prefix_list_ids)
  has_https_listener = var.enable_https_listener || var.existing_https_listener_arn != null
  manage_https_rules = var.manage_security_group_ingress && local.has_https_listener && var.alb_security_group_id != null
  http_cidr_blocks   = length(var.http_ingress_cidr_blocks) > 0 ? var.http_ingress_cidr_blocks : var.https_ingress_cidr_blocks
}

resource "terraform_data" "guard" {
  input = {
    https_listener = local.has_https_listener
  }

  lifecycle {
    precondition {
      condition     = !(var.enable_https_listener && var.existing_https_listener_arn != null)
      error_message = "enable_https_listener and existing_https_listener_arn are mutually exclusive."
    }

    precondition {
      condition     = !var.enable_https_listener || var.certificate_arn != null
      error_message = "enable_https_listener requires certificate_arn."
    }

    precondition {
      condition     = var.enable_https_listener || length(var.additional_certificate_arns) == 0
      error_message = "additional_certificate_arns requires enable_https_listener = true."
    }

    precondition {
      condition     = !var.enable_http_redirect || local.has_https_listener
      error_message = "enable_http_redirect requires an HTTPS listener (enable_https_listener or existing_https_listener_arn)."
    }

    precondition {
      condition     = !(var.manage_security_group_ingress && local.has_https_listener) || var.alb_security_group_id != null
      error_message = "manage_security_group_ingress requires alb_security_group_id."
    }

    precondition {
      condition     = !(var.manage_security_group_ingress && local.has_https_listener) || local.https_source_count > 0
      error_message = "manage_security_group_ingress requires https_ingress_cidr_blocks or https_ingress_prefix_list_ids. There is no default; choose who may reach the listener."
    }
  }
}

resource "aws_lb_listener" "https" {
  count = var.enable_https_listener ? 1 : 0

  load_balancer_arn = var.alb_arn
  port              = var.https_port
  protocol          = "HTTPS"
  ssl_policy        = var.ssl_policy
  certificate_arn   = var.certificate_arn
  tags              = merge(local.common_tags, { "component" = "gateway-https-listener" })

  default_action {
    type = "fixed-response"

    fixed_response {
      content_type = "text/plain"
      message_body = var.default_response_message
      status_code  = var.default_response_status_code
    }
  }
}

resource "aws_lb_listener_certificate" "additional" {
  for_each = var.enable_https_listener ? var.additional_certificate_arns : toset([])

  listener_arn    = aws_lb_listener.https[0].arn
  certificate_arn = each.value
}

resource "aws_lb_listener" "http_redirect" {
  count = var.enable_http_redirect ? 1 : 0

  load_balancer_arn = var.alb_arn
  port              = var.http_port
  protocol          = "HTTP"
  tags              = merge(local.common_tags, { "component" = "gateway-http-redirect-listener" })

  default_action {
    type = "redirect"

    redirect {
      port        = tostring(var.https_port)
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_vpc_security_group_ingress_rule" "https_cidr" {
  for_each = local.manage_https_rules ? toset(var.https_ingress_cidr_blocks) : toset([])

  security_group_id = var.alb_security_group_id
  cidr_ipv4         = each.value
  description       = "HTTPS to the shared gateway from ${each.value}."
  from_port         = var.https_port
  to_port           = var.https_port
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_ingress_rule" "https_prefix_list" {
  for_each = local.manage_https_rules ? toset(var.https_ingress_prefix_list_ids) : toset([])

  security_group_id = var.alb_security_group_id
  prefix_list_id    = each.value
  description       = "HTTPS to the shared gateway from prefix list ${each.value}."
  from_port         = var.https_port
  to_port           = var.https_port
  ip_protocol       = "tcp"
  tags              = local.common_tags
}

resource "aws_vpc_security_group_ingress_rule" "http_redirect" {
  for_each = var.manage_security_group_ingress && var.enable_http_redirect && var.alb_security_group_id != null ? toset(local.http_cidr_blocks) : toset([])

  security_group_id = var.alb_security_group_id
  cidr_ipv4         = each.value
  description       = "HTTP redirect listener from ${each.value}."
  from_port         = var.http_port
  to_port           = var.http_port
  ip_protocol       = "tcp"
  tags              = local.common_tags
}
