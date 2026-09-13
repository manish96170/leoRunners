# Phase 5 Plan

Phase 5 prepares immutable runner images and measures the cold-start path. The repository contains offline-validated Packer assets, a minimal `base-linux-x64` profile, and a standalone benchmark tool with fake mode by default.

## Real AWS sequence

1. Pin an Amazon Linux 2023 x86_64 source AMI per region.
2. Review the Packer builder role, private build subnet, build security group, and cleanup permissions.
3. Run `ami/test.sh`, then Packer format, init, and validate with private variables.
4. Build the AMI and inspect the manifest, image profile marker, permissions, and forbidden-secret scan.
5. Launch a disposable instance from the exact AMI and Launch Template.
6. Deliver a test JIT configuration through the private bootstrap channel.
7. Run one controlled GitHub job, collect benchmark timings, and terminate the instance.
8. Approve or roll back the immutable profile publication based on security, lifecycle, and timing gates.

The offline benchmark mode never calls AWS. Real mode remains guarded until account, region, cost, and cleanup controls are reviewed.
