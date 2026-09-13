# Codex Handoff Prompt: Docker and Kubernetes Setup

You are continuing work in:

```text
/Users/Manish.Sharma/hornblower/leoRunners
```

The project is a Go-based ephemeral CI runner control plane. Do not commit or push anything. Preserve existing user changes and inspect the current worktree before editing.

## Objective

Set up and verify Docker on the development machine, build and run the controller container, and make the deployment Kubernetes-compatible without making Kubernetes a required local dependency.

The platform must remain useful without AI and without Kubernetes. Docker and Kubernetes are deployment/runtime options, not critical dependencies of the controller design.

## Current repository

Important paths include:

```text
controller/                         Go controller module
controller/cmd/runner-controller/   controller executable
controller/internal/config/         typed runtime configuration
controller/internal/providers/       fake, AWS, and GCP providers
controller/internal/state/          memory and file-backed state
controller/internal/lifecycle/       reconciliation and reaping
bootstrap/runner/                    ephemeral runner bootstrap
ami/                                 Packer image assets
workloads/                           workload manifests
tools/smoke/                         offline controller smoke test
tools/benchmark/                     startup benchmark harness
tools/benchmark-compare/             baseline/candidate comparison
intelligence/                        optional AI fixtures and policy
Dockerfile                           existing multi-stage container build
.dockerignore                        existing container exclusions
.github/workflows/ci.yml             existing CI checks
PLAN.md                              current execution plan
HANDOFF.md                           session handoff
TODO.md                              remaining work
```

Read the specification file and the current `PLAN.md`, `HANDOFF.md`, `TODO.md`, `Dockerfile`, controller configuration, and smoke harness before acting.

## Phase A: inspect the machine

Report the operating system, architecture, and available tools:

```bash
uname -a
uname -m
command -v docker || true
docker version || true
docker info || true
command -v kubectl || true
kubectl version --client || true
command -v kind || true
command -v k3d || true
command -v minikube || true
```

Do not silently install system software. If Docker Desktop, Docker Engine, `kubectl`, Kind, k3d, or Minikube is missing, explain exactly what is missing and ask for permission before installing it. Prefer the current stable releases and official installation sources.

## Phase B: Docker setup

If Docker is available, verify:

```bash
docker version
docker info
docker build --tag leo-runners/controller:local .
docker image inspect leo-runners/controller:local
```

The image must:

- build from the repository root;
- use the existing multi-stage Go build;
- run as non-root UID/GID `65532:65532`;
- expose port `8080`;
- use `/var/lib/leo-runners` for optional file-backed state;
- contain no source credentials, GitHub tokens, JIT configuration, AWS keys, GCP keys, SSH keys, or local state;
- keep AI disabled unless explicitly configured;
- use a read-only filesystem where practical, with a writable state volume and temporary directory supplied explicitly;
- terminate gracefully on `SIGTERM`.

Verify the image user:

```bash
docker image inspect leo-runners/controller:local --format '{{.Config.User}}'
```

Run a local container smoke test with fake provider mode and an isolated temporary state volume. Generate a valid HMAC-signed webhook only if the test needs to exercise event intake. Always remove the test container and temporary volume afterward. Do not use real AWS/GCP/GitHub credentials.

Use the existing smoke harness where possible:

```bash
./tools/smoke/test.sh
```

If Docker is unavailable, run all non-Docker checks and record Docker as an environment-dependent gap.

## Phase C: Kubernetes compatibility

Add Kubernetes manifests under a clearly named directory such as:

```text
deploy/kubernetes/
```

Use plain Kubernetes YAML unless an existing Helm or Kustomize convention is already present. Do not add an operator or Kubernetes controller.

The deployment must include:

- `Namespace` only if there is a clear reason and it is configurable;
- `ServiceAccount` with no unnecessary permissions;
- `Deployment` with configurable image and replica count;
- `Service` only if needed for the webhook endpoint;
- `ConfigMap` for non-secret configuration;
- `Secret` references for webhook and GitHub credentials, with no literal secret values;
- persistent storage for `STATE_PATH` only for single-replica/local compatibility;
- readiness and liveness probes for `/healthz`;
- resource requests and limits;
- security context with `runAsNonRoot: true`, UID/GID `65532`, dropped capabilities, and `allowPrivilegeEscalation: false`;
- `terminationGracePeriodSeconds` compatible with controller shutdown;
- explicit environment variables for provider, capacity, reconciliation, and state configuration;
- comments or documentation explaining that production multi-replica state requires a shared durable store and coordinated leases.

Do not give the controller Kubernetes API permissions unless the implementation actually needs them. The controller provisions EC2/GCE through cloud APIs; it does not provision runners as Kubernetes Pods in this phase.

## Phase D: validation

Run every applicable check:

```bash
cd controller
gofmt -w cmd internal
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

For Kubernetes manifests, use available validation tools:

```bash
kubectl apply --dry-run=client -f deploy/kubernetes/
```

If a local cluster is available, create a disposable cluster and verify:

1. manifests apply successfully;
2. the controller pod becomes ready;
3. `/healthz` succeeds through the Service;
4. the pod runs as the non-root configured UID;
5. `SIGTERM` causes graceful shutdown;
6. no cloud or GitHub call occurs in fake mode;
7. state volume behavior is understood and documented.

Delete only the disposable cluster/resources created for this validation. Do not delete existing user clusters, namespaces, images, volumes, or contexts.

## Security requirements

- Never print secret values.
- Never bake secrets into Docker layers, Kubernetes YAML, ConfigMaps, AMIs, labels, annotations, or logs.
- Do not commit generated kubeconfig files, Docker credentials, cloud credentials, state files, or reports containing secrets.
- Use Kubernetes Secrets or an external secret manager by reference; do not put real values in manifests.
- Keep the runner itself outside the Kubernetes control plane unless a later phase explicitly adds a Kubernetes provider.
- Do not enable privileged containers, host networking, host PID, host mounts, Docker socket access, or broad Kubernetes RBAC without a separate security decision.

## Deliverables

Produce:

```text
Docker setup result and version information
Docker image build/run result
deploy/kubernetes/ manifests or a clear explanation of why a requested manifest is blocked
Kubernetes validation result
updated docs/production-readiness.md or a new docs/docker-kubernetes.md
a concise update to PLAN.md, HANDOFF.md, and TODO.md
```

The final report must separate:

- completed local changes;
- commands that passed;
- commands blocked by missing machine tools;
- cloud/GitHub validation that was intentionally not performed;
- security risks and remaining work;
- exact files changed.

Do not claim Kubernetes or Docker validation passed unless the command actually ran successfully. Do not commit or push.
