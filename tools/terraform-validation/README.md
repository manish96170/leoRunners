# Terraform Provider Validation

This is an offline, review-only contract gate for the Terraform tree. It
checks provider source and version contracts, Terraform version floors,
committed lockfiles, local module source paths, root module composition, root
outputs, and basic variable/resource wiring without requiring a provider
plugin.

The contract is stored at `terraform/provider-contract.v1.json`. A missing
optional lockfile is reported as `WARN`; a missing required lockfile, malformed
lockfile, source mismatch, out-of-range locked version, broken local module
path, or missing required output is a failure.

Provider status is deliberately explicit. `UNAVAILABLE` means the offline gate
did not find initialized provider binaries. This is not a validation failure;
an approved environment must run `terraform init -backend=false` followed by
`terraform validate` before any plan or apply. This tool never runs init,
provider schema discovery, plan, apply, IAM upload, or external cloud calls.

```sh
./tools/terraform-validation/test.sh
./tools/terraform-validation/validate.sh
./tools/terraform-validation/validate.sh --json
```

The environment variable guards `TERRAFORM_VALIDATION_INIT=true`,
`TERRAFORM_VALIDATION_APPLY=true`, and `TERRAFORM_VALIDATION_UPLOAD=true`
fail closed so this gate cannot be repurposed for a mutating workflow.
