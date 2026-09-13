# Artifact Security Validation

This Phase 52 validator performs an offline, read-only security scan of a
built-image metadata contract, Dockerfile, and Kubernetes manifests. It does
not build, run, pull, push, apply, delete, or mutate artifacts or cluster
resources.

It checks digest pinning, Linux/amd64 metadata, non-root execution, read-only
root compatibility, embedded secrets, Dockerfile secret copies, Kubernetes
privileged and host access, service-account token automount, writable roots,
privilege escalation, wildcard or cluster-admin RBAC, literal Secret data, and
AI-enabled defaults or credentials.

Results are `PASS`, `WARN`, or `FAIL`. Warnings return zero, policy failures
return one, and invalid inputs return two. JSON reports are redacted, written
atomically, and use owner-only `0600` permissions.

The default command scans the safe offline fixtures:

```sh
tools/artifact-security-validation/validate.sh
```

Scan real artifacts with explicit paths:

```sh
tools/artifact-security-validation/validate.sh \
  --dockerfile Dockerfile \
  --manifest-dir deploy/kubernetes \
  --metadata path/to/built-image-metadata.json \
  --report /tmp/artifact-security.json
```

An existing local image may be inspected with read-only `docker image inspect`
when `--image` is supplied. A Kustomize directory may be rendered with
read-only `kubectl kustomize` when `--render-kustomize` is supplied. Missing
Docker or kubectl produces a warning and never blocks static checks. No Docker
build/run/registry operation or Kubernetes apply/delete operation is used.

Run the offline suite with:

```sh
tools/artifact-security-validation/test.sh
```
