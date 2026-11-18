# Security Artifact Fixtures

These fixtures exercise Phase 52's built-image and rendered-manifest security
contract without Docker, kubectl, a registry, or a cluster.

- `safe/` must return `PASS`.
- `warn/` contains reviewable omissions and must return `WARN`.
- `fail/` contains privileged access, secret exposure, unpinned artifacts,
  wildcard RBAC, and AI-enabled defaults and must return `FAIL`.

Metadata files are assertions about a built image, not credentials or registry
access. Digests are deterministic test values.
