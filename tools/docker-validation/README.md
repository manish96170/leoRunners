# Docker Validation

This directory contains the Phase 12 validator for the repository's existing
root `Dockerfile`. It is intentionally separate from image construction and
does not modify the image or application source.

## Checks

`validate.sh` explicitly requires both the Docker CLI and a reachable Docker
daemon. If Docker is absent or the daemon is stopped, it exits with status `2`
and prints the exact prerequisite that failed.

The mandatory validation:

- builds the root `Dockerfile` using the repository root as context;
- verifies the runtime user is UID/GID `65532:65532`;
- verifies `8080/tcp` is exposed;
- verifies the controller entrypoint is `/usr/local/bin/runner-controller`;
- scans the image filesystem for common secret files, credentials, private
  keys, and JIT configuration files.

The image tag and generated container name are temporary by default. Cleanup
removes the container and image even when a check fails. Use `--keep-image` or
`DOCKER_VALIDATION_IMAGE` when inspecting a locally built image after a run.

## Usage

From the repository root:

```sh
tools/docker-validation/validate.sh
```

Run the optional container checks as well:

```sh
tools/docker-validation/validate.sh --run-smoke
```

The smoke mode starts the image with a disposable webhook secret, waits for
`/healthz`, checks an unsigned webhook returns `401`, and checks a signed
malformed webhook returns `400`. The container is removed by the exit trap.
This does not contact AWS, GCP, GitHub, or a model endpoint.

Run shell-only checks (works without Docker):

```sh
tools/docker-validation/test.sh
```

The validator is a local image-contract gate. It does not prove real cloud
provisioning, GitHub registration, or Kubernetes deployment behavior.
