# Runner Network Terraform Module

This reusable module supplies the network attachments for ephemeral AWS CI
runners. It does **not** create a VPC, subnet, route table, internet gateway,
or NAT gateway by default. Existing VPC and subnet ownership stays with the
caller.

## Modes

The smallest mode only passes through IDs already owned by the caller:

```hcl
module "runner_network" {
  source = "./terraform/modules/runner-network"

  name              = "leo-runners"
  vpc_id            = "vpc-0123456789abcdef0"
  runner_subnet_ids = ["subnet-0123456789abcdef0"]
  security_group_ids = ["sg-0123456789abcdef0"]
}
```

Set `create_runner_security_group = true` to create an ingress-free runner
security group. Supply `vpc_cidr_block` (or an explicit `dns_egress_cidr`) so
DNS egress is constrained to the VPC resolver path. The created group allows
only:

- TCP/443 to `https_egress_cidr` (default `0.0.0.0/0` for GitHub and registries);
- UDP/53 to the DNS destination; and
- TCP/53 to the DNS destination.

There are intentionally no inbound rules. SSH, RDP, and public runner service
ports are not opened.

## VPC endpoints

Endpoints are disabled by default. Enable them explicitly and provide the
corresponding subnet or route-table IDs:

```hcl
enable_vpc_endpoints      = true
interface_endpoint_services = ["ssm", "ssmmessages", "ec2messages"]
gateway_endpoint_services   = ["s3"]
endpoint_subnet_ids       = ["subnet-private-a", "subnet-private-b"]
endpoint_route_table_ids  = ["rtb-private-a", "rtb-private-b"]
```

Interface endpoints receive a dedicated security group. When the module also
creates the runner group, endpoint ingress is restricted to that group on
TCP/443. When using caller-supplied runner groups, provide
`endpoint_security_group_ingress_cidr`, preferably the private subnet/VPC CIDR,
so the module does not silently create unrestricted endpoint access.

Gateway endpoints update the supplied route tables. Review existing routes and
endpoint policies before applying.

## NAT and cost

NAT is disabled by default. `enable_nat_gateway = true` creates one public NAT
gateway and one EIP in the supplied existing public subnet, then adds a default
route to every supplied private route table. The module never creates the VPC,
public subnet, or route tables.

NAT gateways incur hourly charges and per-GB data-processing charges. A single
NAT is cheaper but creates an AZ dependency; one NAT per AZ improves resilience
but is intentionally outside this module's default behavior. Interface
endpoints also incur hourly per-AZ and data-processing charges. Gateway
endpoints have no hourly endpoint charge, but they only cover supported AWS
services. The `cost_warnings` output makes selected paid options visible in
plans and downstream tooling.

## Security and ownership

- Use private runner subnets without public IP assignment.
- Keep GitHub/JIT secrets out of Terraform variables, state, user data, and AMIs.
- Use VPC Flow Logs and CloudTrail according to the environment's security policy.
- Treat fork jobs as untrusted and place them in separately approved capacity.
- Review all supplied security groups, route tables, endpoint policies, and NAT
  routes before apply.

The endpoint security group necessarily has a narrowly scoped inbound TCP/443
rule because interface endpoints accept connections from runners. This is
separate from the runner security group, which remains ingress-free.

## Validation

From the repository root:

```bash
terraform fmt -check -recursive terraform/modules/runner-network
terraform -chdir=terraform/modules/runner-network init -backend=false
terraform -chdir=terraform/modules/runner-network validate
terraform/modules/runner-network/validate.sh
```

The validation script is offline and checks that no VPC resource or credential
material was added, that creation is opt-in, and that the required egress and
cost safeguards remain present.
