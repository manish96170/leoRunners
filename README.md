# leoRunners

Self-hosted, ephemeral GitHub Actions runners on your own AWS account.

When a workflow job is queued, the controller launches one EC2 instance, registers it
with GitHub as a just-in-time (JIT) runner, lets it run that single job, then terminates
it. A reaper and reconciliation loop remove anything that is orphaned or expired.
Everything runs in your account: no job code, secrets, or logs leave it.

> **Status: pre-release.** The controller, AWS provider, state, IAM and observability
> pieces are implemented and tested with mocks. A full end-to-end run against a real AWS
> account and GitHub org has not been completed yet, and the one-step installer is still
> being built. See [Status](#status) before depending on it.

## How it works

```
GitHub workflow_job (queued) -> webhook -> controller -> EC2 RunInstances (launch template)
                                                 |                  |
                                           state (DynamoDB)   runner boots, registers via JIT,
                                                 |            runs one job, then is terminated
                                      reaper / reconciliation
```

- **Ephemeral:** one instance per job, never reused.
- **No long-lived runner credentials:** JIT configuration is single-use and is not baked
  into the AMI or persisted in controller state.
- **Hardened by default:** IMDSv2 required, ownership tags on every instance, separate
  controller and runner IAM roles, no inbound SSH.
- **Bounded:** a maximum runner lifetime and a capacity limit are always enforced.

## Get started (AWS)

Reusable Terraform modules for the controller, the HTTPS front door, and the runner contract are described in [`docs/aws-modules.md`](docs/aws-modules.md); a worked composition is in `terraform/examples/aws-customer`. The step-by-step quickstart (`docs/aws-quickstart.md`) is in progress, tracked in [`TODO.md`](TODO.md). In short:

1. Create a GitHub App for your organization and install it.
2. Prepare AWS: network, runner AMI, launch template.
3. Apply the Terraform for state and IAM, then deploy the controller.
4. Point the GitHub App webhook at the controller.
5. Run a test workflow on your runner label.

## Repository layout

| Path | Purpose |
|---|---|
| `controller/` | Go controller: webhook intake, scheduler, lifecycle, AWS and GCP providers |
| `bootstrap/runner/` | Script that registers the runner with its JIT config at boot |
| `ami/` | Packer template and scripts for the runner image |
| `terraform/` | Modules and examples: generic AWS deployment modules (`aws-*`), DynamoDB state, IAM, network, observability, dev composition |
| `deploy/kubernetes/` | Kubernetes manifests for the controller |
| `docs/` | Architecture, provider, security and operations documentation |
| `tools/` | Validation, benchmark and smoke-test tooling |

Maintainer phase plans and approval-gated validation runbooks also live under `docs/`.

## Status

Works and is covered by tests (mocked cloud and GitHub):
- Webhook signature verification, deduplication, lifecycle state machine, reaper.
- AWS provider (Launch Template + `RunInstances`, tags, IMDSv2, readiness, termination).
- GitHub JIT client and runner bootstrap contract.
- DynamoDB shared state and coordination, Terraform modules, CloudWatch observability.

Not done yet:
- Real AWS and GitHub end-to-end validation.
- GitHub App authentication in the controller (it currently takes a static installation token).
- A single AWS install that creates the network, launch template and controller runtime.
- Packer image bake and cold-start measurements on real infrastructure.

## License

Licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE). You may use,
modify and distribute it for any noncommercial purpose. Commercial use requires a
separate license from the copyright holder.
