# Runner EC2 Module

This module will own the customer-configurable Launch Template and the network/security-group inputs required by the AWS provider.

Required inputs include AMI/profile, instance type policy, subnet and security group IDs, encrypted root volume settings, and a runner bootstrap reference. The template must require IMDSv2 and must not contain GitHub or cloud credentials.

The controller launches one instance with `RunInstances` and launch-time ownership tags. This module does not create an Auto Scaling Group, EC2 Fleet, warm pool, or persistent runner capacity in Phase 2.
