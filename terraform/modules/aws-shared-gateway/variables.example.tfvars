alb_arn               = "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/app/example/0123456789abcdef"
alb_security_group_id = "sg-0123456789abcdef0"
certificate_arn       = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"

# Review-only default: nothing is created until this is true.
enable_https_listener = false

# Choose explicitly who may reach the listener. 0.0.0.0/0 is a deliberate
# choice for a public webhook receiver; narrower ranges are better when known.
https_ingress_cidr_blocks = ["192.0.2.0/24"]

# enable_http_redirect = true

listener_rule_priority_ranges = {
  controller = { start = 100, end = 199 }
  other      = { start = 200, end = 299 }
}

tags = {
  Environment = "example"
  Owner       = "example-team"
}
