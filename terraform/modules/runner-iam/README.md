# Runner IAM Module

Create separate roles and policies for the controller and runner instances.

The controller role is limited to the selected Launch Template, required `RunInstances`/`DescribeInstances`/`TerminateInstances` operations, and tag-on-create permissions. The runner role is workload-specific and must not grant arbitrary pull requests privileged account access.

Require IMDSv2, avoid embedded credentials, prefer GitHub OIDC for workload cloud access, and use conditions that constrain Launch Template usage, region, tags, and instance profile. Exact policy statements require the target account, region, network, and workload permissions.
