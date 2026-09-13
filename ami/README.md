# Immutable Runner AMI

This directory builds a pinned Amazon Linux 2023 x86_64 AMI for ephemeral CI
runner hosts. The image contains only base execution and archive tooling. It
does not contain the GitHub Actions runner, a JIT configuration, a PAT, cloud
credentials, SSH keys, or workload-specific dependencies.

The controller supplies the one-time GitHub JIT configuration after launch;
registration remains a runtime concern. The AMI therefore stays reusable and
does not create a long-lived GitHub runner identity.

## Inputs

`runner.pkr.hcl` requires explicit values for:

- `aws_region`
- `vpc_id`
- `subnet_id`
- `source_ami`
- `iam_instance_profile`
- `security_group_id`

The source AMI should be a pinned Amazon Linux 2023 x86_64 image in the same
Region. The build instance profile is only for the temporary Packer host and
must be least privilege. Do not put AWS access keys in a variables file;
Packer uses the normal AWS SDK credential chain (for example, an SSO profile or
an assumed role).

The build security group must permit SSH from the Packer builder and should
not be reused as the runtime runner security group. The template uses the
private IP of the build instance, encrypted gp3 root storage, and IMDSv2 with
`HttpTokens=required`. It does not assign a public IP.

## Validate and build

Run offline checks first:

```sh
./test.sh
```

Copy and edit the example variables file outside source control, then validate
the Packer configuration:

```sh
cp example.pkrvars.hcl /tmp/leo-runners.pkrvars.hcl
# edit /tmp/leo-runners.pkrvars.hcl with real, pinned IDs
packer init .
packer validate -var-file=/tmp/leo-runners.pkrvars.hcl runner.pkr.hcl
```

The convenience script validates formatting, initializes the Amazon plugin,
and validates using the checked-in fixture values:

```sh
./validate.sh
```

Build only after reviewing the change and confirming the AWS account, Region,
subnet, profile, and cleanup permissions:

```sh
packer build -var-file=/tmp/leo-runners.pkrvars.hcl runner.pkr.hcl
```

The manifest is generated only by a successful build and is ignored by source
control. Promote an AMI by its immutable ID and record that ID in the Launch
Template; do not resolve a moving AMI alias during runner provisioning.

## Image contents

The installer uses Amazon Linux 2023's `dnf` package manager and installs:

`ca-certificates`, `curl-minimal`, `git`, `gzip`, `jq`, `make`, `openssl`,
`tar`, `unzip`, and `which`.

`validate-image.sh` runs inside the temporary build host and fails if the
runner installation or common credential paths are present. It does not call
GitHub, contact the controller, or retrieve instance metadata.
