# Workload Manifests

This directory contains versioned, repository-derived workload profiles. A
manifest describes capabilities observed in a real repository and the evidence
behind each observation. It is input to image selection and scheduling, not a
package-installation recipe.

## Design rules

- Keep `enabled: false` and `required: false` until a workflow, lockfile,
  source tree, build script, or measured job proves the capability is needed.
- Record evidence with a repository-relative `source`, a precise `locator`, and
  an observation. Do not put tokens, environment values, customer data, or
  complete dependency files into the manifest.
- A capability may be enabled without being required when it is useful but a
  workflow can still run without it. Make it required only when absence should
  prevent scheduling onto an image.
- The platform consumes generic capability IDs and runner requirements. It does
  not assume Node.js, JavaScript, or any particular package manager.
- Cache entries describe measurements and policy, not implementation details of
  a specific CI vendor. Start with `observe-only` until hit rate and restore
  cost justify enabling a cache.
- Bump `metadata.version` when the manifest meaningfully changes. Preserve
  the previous file for reproducibility of benchmark results.

## Deriving a manifest from a real repository

1. Inspect workflow files and scripts for commands, action inputs, services,
   container builds, and explicit runtime versions.
2. Inspect lockfiles and configuration by filename and structure, without
   copying dependency contents or secrets into the manifest.
3. Map each observation to the closest generic capability ID. Use a new
   capability ID only when the existing vocabulary cannot express the need.
4. Set `evidence.status` to `observed` only when the source is present and
   reproducible. Use `inferred` for a strong indirect signal and `absent`
   when the repository was checked and no evidence was found.
5. Set `enabled` and `required` from the observed workflow contract, then run
   `./workloads/validate.sh`.
6. Execute the workload on the selected image and update `validation` and
   `caches` with measured results. A failed command is evidence to investigate,
   not permission to silently add every tool to the image.

The same process works for a Go, Rust, Python, JVM, or mixed repository. The
manifest format is intentionally runtime-neutral; Node and frontend tooling
are just capability entries alongside infrastructure, testing, and systems
tooling.

## Updating safely

Review changes as a diff against the prior version. Require a source locator
for every newly enabled capability, keep secrets out of evidence, and record
the repository revision and inspection date. Publish a new image/profile only
after the workload validation gates pass; do not mutate an already-used
manifest version.
