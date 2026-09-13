# Managed Capacity

Managed capacity is infrastructure operated by the platform provider. The
platform owns the account/project, network, image pipeline, provider
credentials, runner cleanup, and capacity-pool configuration. A customer
still owns its GitHub organization/repository permissions and workload policy;
managed capacity does not grant the platform access to unrelated customer
accounts.

## Shared Control Plane

The managed deployment uses the same control-plane concepts as customer
capacity:

```text
GitHub webhook -> normalized job -> policy -> managed capacity pool
             -> JIT runner -> lease -> completion -> cleanup
```

The selected pool carries `ownership=managed`, a tenant identifier, provider
and region, security profile, allowed labels, and billing metadata. Provider
code creates exactly one ephemeral runner per provisioning attempt and the
reconciler removes expired or orphaned resources.

## Ownership Boundaries

The platform operator owns:

- managed cloud accounts/projects, networks, images, templates, and quotas;
- controller and provider credentials for those resources;
- runner bootstrap, registration, termination, and operational telemetry.

The customer owns:

- GitHub App installation and repository authorization;
- labels, workflow policy, secret exposure decisions, and allowed workload
  classes;
- approval of whether a repository may run on managed capacity.

Tenant identity is checked at webhook normalization and again at pool
selection. A managed pool must not be selected for a tenant or repository
outside its allowlist.

## Credentials And Isolation

Use short-lived, scoped credentials where the provider supports them. Keep
GitHub installation credentials in the control-plane secret store; deliver
only the one-time JIT configuration to the runner through the protected
bootstrap contract. Never put GitHub tokens, cloud keys, or JIT configuration
in images, Terraform variables, instance labels, logs, or ordinary state.

The controller identity may create, inspect, and terminate only managed
runner resources. The runner identity is workload-specific and cannot manage
the control plane, IAM, other tenants, or other runners. Network policy should
deny inbound administration by default and provide only approved outbound
access.

## Deployment Guardrail

Managed capacity is provisioned only in platform-owned accounts/projects that
have already been configured and approved by the operator. The controller
does not discover customer accounts, create roles, accept role assumptions, or
modify networks automatically. Customer onboarding supplies any external
trust, role, project, subnet, or GitHub authorization explicitly and outside
the controller's runtime.
