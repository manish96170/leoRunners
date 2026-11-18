# AMI Profiles

This directory contains immutable runner-image profile declarations. The
controller schedules by profile name; the AWS provider resolves an approved
profile publication to a region-specific AMI ID.

## Profile rules

- Every profile has a unique `name`, immutable semantic `version`, parent, and
  x86_64/ARM architecture declaration.
- `base-linux-x64` is the minimal parent. Derived profiles declare exactly one
  parent and may add tools, never weaken security or change the bootstrap
  contract.
- Pin tool versions and verify downloaded artifacts with checksums or trusted
  signatures. Do not install development environments in EC2 user-data.
- One concern per extension is preferred. Add Node, browsers, Rust, Go,
  Docker, Terraform, or Serverless only when workflow evidence supports it.
- A new profile version is required for any tool, base OS, bootstrap, kernel,
  repository, or security change. Published versions and AMI IDs are never
  mutated.

## Required build metadata

The image pipeline must retain the profile file, parent version, source
revision, build ID, content digest, region, AMI ID, complete tool manifest,
artifact checksums, builder identity, validation result, and publication time.
The AMI ID is provider metadata and is not part of the profile's stable name.

## Approval gates

Before an AMI is enabled, validate schema and reproducibility, scan for
credentials, verify non-root execution and IMDSv2 assumptions, smoke-test every
declared tool, launch the exact AMI through the Launch Template, run the JIT
registration/bootstrap contract, execute a disposable test job, confirm
termination, and capture cold-start measurements. Destroy validation
instances and remove temporary credentials afterward.

## Secrets and rollback

Never store GitHub tokens, JIT configuration, cloud keys, SSH keys, customer
data, or mutable caches in an image or its metadata. JIT data is delivered only
through the private one-time bootstrap channel and is excluded from logs and
measurement records.

Keep the prior approved profile publication while rolling out a new one. A
rollback disables the candidate mapping and points new work to the previous
profile version; it never modifies an existing AMI. See
`docs/runner-images.md` for the full contract and measurement schema.
