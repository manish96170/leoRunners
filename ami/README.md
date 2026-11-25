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

## Profiles: a base image and derived images

| Profile | What it adds | Built with |
|---|---|---|
| `base-linux-x64` | the base tools above | no extension |
| `frontend-browser-linux-x64` | a pinned Chrome, the libraries Cypress needs to drive it, Docker, optionally a pinned Terraform | `extensions/frontend-browser.sh` |

A derived profile has exactly one parent, may only add tools, and never weakens
the parent's security settings (`tools/ami-validation/validate.sh` fails
otherwise). Add a profile only when a real workload needs it; group repositories
by the kind of workload, not one image per repository, so there are few images to
patch. Each image is rebuilt for OS patches and runner updates, so every profile
is a recurring cost in effort even though a stored AMI costs very little.

Build the front-end image like this (the values come from the infra repository,
not from here):

```hcl
extension_script      = "../ami/extensions/frontend-browser.sh"
extension_environment = {
  CHROME_RPM_URL    = "https://<your storage>/google-chrome-stable-<version>-1.x86_64.rpm"
  CHROME_RPM_SHA256 = "<sha-256 of that file>"
  INSTALL_DOCKER    = "true"
  # optional, both together:
  # TERRAFORM_VERSION            = "0.12.23"
  # TERRAFORM_SHA256_LINUX_AMD64 = "<sha-256 of the release archive>"
}
ami_name_prefix = "leo-runners-frontend-browser-x86_64"
```

The extension was run on Amazon Linux 2023 x86_64 (under emulation) with a pinned
Chrome and Terraform and the distribution's Docker; the build log ends with the
record of what was installed. Chrome cannot start under CPU emulation, so that
run set `CHROME_LAUNCH_CHECK=false`; on a real x86 builder the extension starts
Chrome headless once and fails the build if it cannot (that check has not run
yet). The image records what it carries in `/etc/leo-runners/extension-frontend-browser`.
Instances launched from an image that has Docker put the runner user in the
`docker` group (the bootstrap does this only when the group exists). That is
root-equivalent on the host, which is acceptable only because each instance is
single-use, private and holds no credentials beyond its one-time registration.

A runner image is chosen through the launch template, so a repository that needs
the browser image runs on a runner pool whose launch template points at it.
Today one controller deployment has one launch template; routing several labels
to several images is not built yet.

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
