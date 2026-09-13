# Docker and Kubernetes Operations

This guide describes local container use and the Kubernetes deployment boundary
for the Go controller. The controller provisions ephemeral GitHub Actions
runners through cloud APIs; it does not create runner Pods in Kubernetes.
Kubernetes is optional for local development and fake-provider testing.

## Local Docker setup

Run these checks from the repository root:

```bash
docker version
docker info
docker build --tag leo-runners/controller:local .
docker image inspect leo-runners/controller:local
docker image inspect leo-runners/controller:local --format '{{.Config.User}}'
```

The checked-in `Dockerfile` uses a multi-stage Go build. The final process runs
as UID/GID `65532:65532`, listens on port `8080`, receives `SIGTERM`, and uses
`/var/lib/leo-runners/state.json` when `STATE_PATH` is not overridden. The image
contains neither credentials nor runner JIT configuration.

For a local fake-provider run, use a disposable state volume and a read-only
root filesystem. `/tmp` is supplied explicitly:

```bash
docker volume create leo-runners-state-local
docker run --rm --name leo-runners-local \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --mount source=leo-runners-state-local,target=/var/lib/leo-runners \
  --publish 8080:8080 \
  --env LISTEN_ADDR=:8080 \
  --env STATE_PATH=/var/lib/leo-runners/state.json \
  --env CAPACITY_PROVIDER=fake \
  --env CAPACITY_POOL_PROVIDER=fake \
  --env GITHUB_JIT_ENABLED=false \
  --env GITHUB_WEBHOOK_SECRET=local-development-secret \
  leo-runners/controller:local
```

In another terminal, verify the process:

```bash
curl --fail http://127.0.0.1:8080/healthz
```

Use the repository smoke harness for signed webhook checks without cloud or
model calls:

```bash
./tools/smoke/test.sh
```

Stop the container with `docker stop leo-runners-local`. Remove only the
disposable local volume when its state is no longer useful:

```bash
docker volume rm leo-runners-state-local
```

Do not pass production secrets through `docker build`, build arguments, image
layers, shell history, labels, or command-line arguments. Supply runtime
secrets through a protected local mechanism only for development.

## Kubernetes deployment shape

The directory `deploy/kubernetes/` contains plain manifests and a Kustomization
for a single-replica fake/provider-compatible deployment. They are deployment
templates, not universal production defaults. Review the target namespace,
storage class, image digest, secret integration, and cloud identity before
applying them.

The controller needs no Kubernetes API permissions. Its ServiceAccount should
therefore have no Role or RoleBinding. The Service is an internal/private HTTP
path for `/webhooks/github` and `/healthz`; expose it publicly only through an
approved ingress or gateway with TLS and network policy.

At minimum, a rendered workload should set:

- image by immutable digest, with port `8080`;
- `runAsNonRoot: true`, `runAsUser: 65532`, and `runAsGroup: 65532`;
- `allowPrivilegeEscalation: false`, dropped Linux capabilities, and a
  `RuntimeDefault` seccomp profile;
- resource requests and limits;
- `terminationGracePeriodSeconds` of at least 30 seconds and longer than any
  configured provider operation timeout;
- a writable `/tmp` mount when the root filesystem is read-only;
- `GET /healthz` probes on port `8080`.

The current executable exposes `/healthz` only. It returns `200` while the HTTP
process is serving and does not verify GitHub, cloud credentials, provider
capacity, or durable-state health. Until a dependency-aware readiness endpoint
is implemented, liveness and readiness may both use `/healthz`, with a startup
grace period sized for image startup. Do not use the webhook route as a probe.

Example probe settings for a rendered workload:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Configuration and secrets

Use a `ConfigMap` for non-secret values such as:

```text
LISTEN_ADDR=:8080
STATE_PATH=/var/lib/leo-runners/state.json
CAPACITY_PROVIDER=aws|gcp|fake
CAPACITY_MODE=customer-first|managed-first|customer-only|managed-only|fallback
CAPACITY_POOL_ID=...
CAPACITY_POOL_PROVIDER=...
CAPACITY_OWNERSHIP=managed|customer
CAPACITY_SECURITY_PROFILE=isolated
RECONCILE_INTERVAL=30s
```

Reference secrets with `env[].valueFrom.secretKeyRef` or an approved external
secret integration. The Secret object must be created out of band or rendered
by a secret-management tool; do not put literal values in checked-in YAML.
Required sensitive values include:

- `GITHUB_WEBHOOK_SECRET`;
- `GITHUB_APP_INSTALLATION_TOKEN` when JIT mode is enabled;
- optional AI API credentials, only when AI is explicitly enabled and consented.

For AWS, prefer pod identity/IRSA or an equivalent workload-identity binding.
For GCP, prefer Workload Identity and Application Default Credentials. Do not
mount cloud access-key files or put them in ConfigMaps. The runner VM's cloud
identity and the controller's cloud identity are separate boundaries.

The JIT configuration is single-use sensitive data. The controller must deliver
it through the runner bootstrap contract and must not place it in Kubernetes
Secrets, ConfigMaps, labels, annotations, durable state, or logs.

## State and replicas

The file-backed repository is suitable for a single controller replica with a
persistent, durable, encrypted volume mounted at `/var/lib/leo-runners`. It uses atomic
writes and a backup snapshot, but it is not a distributed database. A
ReadWriteOnce PVC is a compatibility option for one replica, not high
availability.

Do not scale this deployment beyond one active controller while it uses the
file repository. Multiple replicas require a shared durable store, coordinated
leases, idempotent workers, and a deliberate migration plan. A restart can
recover file-backed records, but it cannot discover every cloud orphan unless
provider discovery and ownership-tag reaping are configured.

## Shutdown and security

On rollout or eviction, Kubernetes sends `SIGTERM`. The controller stops its
HTTP server with a bounded 30-second shutdown context; durable state and the
next reconciliation pass must keep unfinished work recoverable. Set the pod
grace period longer than the longest bounded provider operation. Do not use
`kill -9` as normal rollout behavior.

Keep the pod non-root and unprivileged. Do not enable privileged mode, host
networking, host PID, hostPath mounts, Docker socket access, or broad
Kubernetes RBAC. Runner workloads remain outside the Kubernetes control plane
unless a future phase explicitly adds a Kubernetes provider.

## Validation

Run repository checks from the root:

```bash
cd controller
gofmt -d cmd internal
go test ./...
go test -race ./...
go vet ./...
go build ./...
cd ..
./tools/smoke/test.sh
./intelligence/validate.sh
./workloads/validate.sh
./ami/test.sh
git diff --check
```

When Docker is available, build and inspect the image, run the fake-mode
container, check `/healthz`, and verify the configured user is `65532:65532`.
When a Kubernetes client is available, validate the Kustomization with
`kubectl apply --dry-run=client -k deploy/kubernetes`; use a disposable cluster
for live apply. The repository also provides `deploy/kubernetes/validate.sh`
for offline structural and credential checks.
Confirm pod readiness, non-root execution, Service health, SIGTERM behavior,
state-volume permissions, and absence of cloud/model calls in fake mode. Do
not modify or delete an existing cluster, namespace, context, volume, or image.
