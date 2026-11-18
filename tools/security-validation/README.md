# Local Security Validation

This scanner performs a local, read-only security gate for the ephemeral CI
platform. It scans the repository's Dockerfile, Kubernetes YAML, AMI and
bootstrap assets, Terraform files, and versioned cache, network, workload, and
intelligence fixtures.

It reports `pass`, `warn`, or `fail` and exits with:

- `0` when no failure is found (warnings are allowed);
- `1` when a policy failure is found;
- `2` for invalid invocation or an unreadable scan root.

## Usage

From this directory:

```sh
go run . --root ../.. --format text
go run . --root ../.. --format json --report /tmp/security-validation.json
```

The report never includes source excerpts or matched values. JSON reports are
written atomically with restrictive permissions.

## Policies

The scanner fails on credential-shaped values and private keys, literal
secret-like assignments, privileged or root containers, privilege escalation,
host networking/PID/IPC, host mounts and Docker socket access, and wildcard or
administrator Kubernetes RBAC. It also fails when the default/runtime
configuration enables AI. Explicitly enabled example profiles produce a
warning so they require a separate consent and privacy review.

Secret references such as Kubernetes `secretKeyRef`, environment expansion,
placeholders, and empty secret manifests are accepted because they do not bake
credential material into repository assets.

Run the focused validation suite with:

```sh
./test.sh
```

This tool makes no cloud, cluster, Docker daemon, GitHub, or model calls.
