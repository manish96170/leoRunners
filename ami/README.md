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

## Tagging a build

`extra_tags` is applied to the temporary build instance, its volume, the AMI and
its snapshots. Put a request ID and a cleanup deadline there, and a build (even a
failed or interrupted one) can be found and removed by the same tag intersection
as everything else for that request. Packer removes its own temporary instance,
key pair and security group when it finishes; after a build, check that nothing
tagged with the request ID except the AMI and its snapshots remains.

## Adding workload tools

The base image carries only what a runner needs to start and run a job. A
workload's own tools (a browser, Docker, a language runtime, system libraries)
belong in an **extension** kept outside this repository, with the versions it
pins, so that customer choices never enter the generic code:

```hcl
extension_script      = "/path/to/your/extension.sh"
extension_environment = { SOME_TOOL_VERSION = "1.2.3" }
```

The script runs as root after the base tools and before validation. Because
validation runs afterwards, an extension cannot leave behind a runner
installation, credentials or SSH keys: the build fails if it does. Pin every
version and verify every download by checksum inside the script. Record the
extension's name and version next to the AMI ID, so an image can be traced to
what is in it.

Tools that `actions/setup-node` or a workflow installs itself at job time (Node,
Terraform) do not need baking; baking them saves job time and makes the job
independent of the download, which is the usual reason to.

## Image contents

The installer uses Amazon Linux 2023's `dnf` package manager and installs:

`ca-certificates`, `curl-minimal`, `git`, `gzip`, `jq`, `libatomic` (Node.js
builds need it), `make`, `openssl`, `tar`, `unzip`, and `which`.

The GitHub Actions runner is deliberately not baked: `bootstrap/runner/user-data.sh`
downloads a pinned, checksum-verified runner at launch. That costs launch time and
needs egress to GitHub's download hosts; baking it would need a change to
`validate-image.sh`, which currently refuses an image that contains one.

`validate-image.sh` runs inside the temporary build host and fails if the
runner installation or common credential paths are present. It does not call
GitHub, contact the controller, or retrieve instance metadata.
