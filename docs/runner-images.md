# Runner Images

Phase 5 defines the immutable image contract for ephemeral runners. An image is
selected by profile, architecture, operating system, and security policy. The
controller supplies only job-specific, short-lived bootstrap data at launch;
it must not turn user-data into an operating-system installation script.

## Goals

- Make the boot-to-job path predictable and measurable.
- Keep the base image small enough to update and replicate quickly.
- Move stable tools into immutable image layers.
- Keep profile selection independent of AWS-specific AMI IDs.
- Allow a bad image to be disabled and rolled back without changing jobs.

The initial profile is `base-linux-x64`. It is a general-purpose Linux x86_64
image, not a promise that every workflow tool is installed. Node, browser,
Rust, Go, Docker, Terraform, and other larger toolchains belong in explicitly
versioned extension profiles after workflow evidence justifies them.

## Profile contract

Each profile is a reviewable YAML document under `ami/profiles/`. A profile
must declare:

- stable `name`, `version`, and `architecture`;
- an immutable parent profile or base operating-system identity;
- required tools and exact versions where reproducibility matters;
- security and bootstrap requirements;
- validation gates and measurement names;
- the AMI publication metadata, including region-specific AMI IDs once built.

The profile name is the scheduler-facing identity. The AMI ID is provider data
and must not be embedded in GitHub webhook handling or workflow semantics.

## Minimal base profile

`base-linux-x64` contains only the operating-system baseline and tools required
to start the ephemeral runner safely:

- Git, for checkout and GitHub Actions setup;
- `curl`, CA certificates, and DNS/network utilities for bootstrap;
- `jq`, for inspecting structured bootstrap and diagnostic data;
- Bash, core POSIX utilities, archive tools, and a non-root runner account;
- the pinned GitHub Actions runner distribution and its checksum;
- the bootstrap entrypoint and its supported shell contract.

The image does not include repository dependencies, customer secrets, GitHub
PATs, JIT configurations, cloud access keys, SSH keys, or a mutable cache.
Package indexes are cleaned before publication. The runner process and jobs
must execute as the dedicated non-root user unless a separately reviewed
profile explicitly declares a stronger isolation policy.

## Extensions and composition

An extension profile is a derived, immutable image with exactly one declared
parent. Extensions may add tools and validation, but may not weaken the
parent's security requirements or replace its bootstrap contract.

Rules:

1. Use a new profile version for every change to an installed tool, base image,
   bootstrap binary, kernel, package repository, or security setting.
2. Pin package repositories, package versions, and downloaded artifact
   checksums. Do not install latest packages during instance boot.
3. Keep one concern per profile where practical: for example,
   `node-linux-x64` or `node-browser-linux-x64`, rather than one universal
   development image.
4. Record the parent profile version and the complete tool manifest in the
   published artifact metadata.
5. Add a tool only when a real workflow requires it or measurements show that
   baking it materially improves job start time. Prefer job-level installation
   for rarely used tools.
6. Docker or privileged tooling requires an explicit security profile,
   runner IAM review, and workload isolation decision.

## Versioning, rollout, and rollback

Profile versions use `MAJOR.MINOR.PATCH` and are immutable after publication.
The resulting AMI is also immutable. A publication record maps:

```text
profile -> profile version -> build ID/digest -> region -> AMI ID
```

The scheduler/provider resolves only an enabled, approved publication. Roll
out by publishing a new version, validating it, and shifting a controlled
percentage or capacity pool to it. Keep the previous approved version until
the new version has passed a representative workload window.

Rollback means disabling the new publication and restoring the previous
profile-version mapping. Do not mutate an AMI in place or silently reuse an
AMI ID for a different profile. Existing runners remain disposable and are
terminated by normal lifecycle policy; new jobs use the restored mapping.

## Security constraints

- Never bake GitHub credentials, JIT configuration, repository tokens, cloud
  access keys, secrets, or customer data into an image, snapshot, user-data
  template, or AMI tags.
- Use IMDSv2 and the least-privilege runner instance role. Do not grant the
  image a controller role.
- Require encrypted root storage and a restricted security group at launch.
- Verify all image-build inputs and downloaded artifacts by checksum or a
  trusted signature; preserve the build manifest.
- Remove build credentials, package-manager caches containing credentials,
  shell history, temporary files, and diagnostic dumps before sealing.
- Run image validation as an isolated disposable test and destroy test
  instances after collection.
- Treat workflow code as untrusted. Privileged Docker, host mounts, and extra
  IAM permissions are opt-in and require separate review.

## Validation gates

An image cannot become `approved` unless every gate passes:

1. **Schema:** profile YAML, version, architecture, parent, tool manifest,
   and publication metadata validate.
2. **Reproducibility:** the build records source revisions, package versions,
   artifact checksums, builder identity, and a content digest.
3. **Security:** no credential or forbidden secret pattern is present; file
   permissions, runner user, IMDSv2 assumptions, encryption, and bootstrap
   contract are verified.
4. **Tool smoke test:** every declared tool reports the declared version and
   the runner distribution passes its checksum and startup check.
5. **Ephemeral launch:** a disposable EC2 instance launches from the candidate
   AMI with the required launch-template settings and reaches healthy state.
6. **Registration:** a test JIT configuration is delivered through the private
   bootstrap channel; the runner registers with the expected labels and is
   online without exposing the configuration in logs or durable state.
7. **Lifecycle:** a test job can start, complete or cancel, and the instance
   terminates; repeated cleanup is harmless.
8. **Performance:** timing fields are captured and compared with the approved
   version. A regression requires an explicit review before rollout.

Validation must test the exact published AMI and launch-template combination,
not merely the image-builder output or a hand-configured instance.

## Measurement record

Capture one structured record per validation run and per production attempt.
Durations are monotonic elapsed milliseconds; timestamps are UTC. The record
must contain at least:

```text
measurement_id
profile_name
profile_version
build_id
build_digest
region
ami_id
launch_template_id
instance_type
architecture
job_id
runner_id
attempt
queued_at
ec2_launch_requested_at
ec2_running_at
boot_started_at
bootstrap_started_at
registration_started_at
registered_at
ready_at
job_started_at
job_completed_at
terminated_at
launch_to_running_ms
running_to_bootstrap_ms
bootstrap_to_registered_ms
registered_to_ready_ms
ready_to_job_ms
job_duration_ms
total_lifecycle_ms
image_size_bytes
root_volume_size_bytes
cache_hit
failure_stage
failure_reason
```

Do not record JIT configuration values or credentials in this record. Compare
the candidate and approved profile using the same region, instance type,
workflow, and network conditions. Optimize only after these measurements
identify a material cold-start bottleneck.
