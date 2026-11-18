# Phase 6 Plan

Phase 6 turns the platform into a repeatable workload-validation experiment. A workload manifest records observed capabilities and evidence without copying secrets or dependency contents. A preflight check reports whether a selected runner image can satisfy those requirements. The validation harness records command results, durations, platform timings, and redacted output.

## Real repository sequence

1. Select a repository, commit, workflow, and test branch with explicit owner approval.
2. Derive `workloads/workload-manifest.v1.yaml` from workflow files, lockfiles, scripts, and infrastructure declarations.
3. Run the local preflight checker against the candidate image and review every warning/failure.
4. Run the exact workload command in `tools/workload-validation` with a fixed checkout and image profile.
5. Run the same workload on the existing CI baseline and record both reports.
6. Compare startup, dependency, build, test, browser, Docker, cleanup, and total timings.
7. Update the manifest only with observed evidence and publish a new image/profile version when justified.

The harness does not infer workflow commands, install missing tools, contact AWS, or silently modify a repository. Real AWS/GitHub execution remains a controlled external validation step.
