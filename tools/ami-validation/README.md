# AMI Validation

`validate.sh` is the offline contract gate for the runner AMI. By default it
does not invoke Packer, contact AWS, initialize plugins, or create resources.
It checks the pinned source AMI, x86_64 architecture, non-root runtime policy,
secret exclusion, package provenance, immutable digest/version metadata, and
rollback metadata across the AMI contract, manifest, Packer template, and
profile.

Run the offline tests with:

```sh
./tools/ami-validation/test.sh
```

## Guarded Packer path

Packer is never called by the default command. A real validation requires an
existing private variables file and an exact confirmation token:

```sh
./tools/ami-validation/validate.sh \
  --real-packer \
  --confirm I_UNDERSTAND_PACKER_VALIDATION \
  --var-file /private/leo-runners.pkrvars.hcl
```

Building an AMI requires a separate explicit flag and token:

```sh
./tools/ami-validation/validate.sh \
  --real-packer --build \
  --confirm I_UNDERSTAND_PACKER_BUILD \
  --var-file /private/leo-runners.pkrvars.hcl
```

The example variables file is documentation-only and must not be used for a
live build. Promotion and rollback remain external operator actions that must
record the immutable image ID and digest.
