# Kubernetes Deployment Contract

This directory contains the reviewed Kubernetes contract and plain manifests
for a single-replica deployment. Render and review them for the target cluster
before applying them; do not rely on undocumented defaults.

## Required workload

Deploy one controller `Deployment` and, when needed, an internal `Service` for
port `8080`. Add a `ServiceAccount` without Kubernetes API permissions; the
controller talks to AWS, GCP, and GitHub APIs, not to the Kubernetes API. Use a
`ConfigMap` for non-secret settings and Secret references for sensitive values.

The workload must:

- use the repository `Dockerfile` image by immutable digest. Replace the
  `replace-with-real-digest` placeholder before applying;
- expose container port `8080`;
- run as non-root UID/GID `65532:65532`;
- set `allowPrivilegeEscalation: false` and drop all capabilities;
- use the `RuntimeDefault` seccomp profile;
- provide resource requests and limits;
- mount `/tmp` as writable when using a read-only root filesystem;
- mount encrypted durable storage at `/var/lib/leo-runners` when using file
  state;
- set a termination grace period of at least 30 seconds;
- configure liveness and readiness HTTP probes for `/healthz`.

The current binary exposes `/healthz`, not `/readyz`. The endpoint proves that
the HTTP server is serving, but does not prove provider, GitHub, or state-store
health. Use a startup grace period and monitor dependency failures separately;
do not probe `/webhooks/github`.

## Configuration

Non-secret environment values should be supplied by a ConfigMap or equivalent
deployment configuration:

```text
LISTEN_ADDR=:8080
STATE_PATH=/var/lib/leo-runners/state.json
CAPACITY_PROVIDER=fake
CAPACITY_POOL_PROVIDER=fake
CAPACITY_MODE=fallback
CAPACITY_POOL_ID=local
CAPACITY_OWNERSHIP=managed
CAPACITY_SECURITY_PROFILE=isolated
RECONCILE_INTERVAL=30s
```

Production provider configuration replaces the fake values with the reviewed
AWS or GCP settings. Keep the selected provider and capacity-pool provider
consistent. Keep AI disabled unless its endpoint, model, consent, and privacy
review are complete.

Reference these values from Kubernetes Secrets, never as literals in a
checked-in manifest:

```text
GITHUB_WEBHOOK_SECRET
GITHUB_APP_INSTALLATION_TOKEN
AI_API_KEY (only when AI is explicitly enabled)
```

Create or synchronize those Secrets through the cluster's approved secret
manager. Do not place credentials in ConfigMaps, image layers, annotations,
labels, command-line arguments, or repository files. The JIT value generated
for one runner is transient and must go directly to the runner bootstrap; it
must not be persisted in this deployment's state or Kubernetes objects.

## Cloud identity boundaries

Use AWS IRSA/pod identity or GCP Workload Identity, depending on the selected
provider. Grant the controller only the cloud actions required to provision,
describe, and terminate its owned runner resources. Do not mount static cloud
credential files. The controller identity must be distinct from the identity
used by runner VMs for workload access.

The controller does not need Kubernetes RBAC. Do not grant access to Pods,
Secrets, Nodes, or cluster-wide resources merely because it runs in Kubernetes.

## State and scaling

The file repository is single-replica compatible, not highly available. For a
local or controlled deployment, mount a PVC at `/var/lib/leo-runners` and keep
`replicas: 1`. The volume must be encrypted, access-controlled, and writable by
UID `65532`.

Do not increase replicas while using the file repository. Multiple active
controllers need a shared durable store, coordinated leases, and a worker model
designed for concurrent reconciliation. A PVC does not provide those semantics.
Provider ownership tags and reconciliation remain necessary for cleanup after
pod restarts.

## Fake mode

Use fake mode for local Kubernetes smoke tests:

```text
CAPACITY_PROVIDER=fake
CAPACITY_POOL_PROVIDER=fake
GITHUB_JIT_ENABLED=false
GITHUB_WEBHOOK_SECRET=<test-only-secret>
```

Fake mode must not receive AWS or GCP credentials, GitHub installation tokens,
AI credentials, or production state. It should produce no cloud or model calls.
Use a disposable namespace, PVC, and Secret for the test and remove only those
resources after verification.

## Apply and validation

The checked-in files are a fake-mode-compatible single-replica baseline, not a
production-ready configuration. Replace the image, storage class, identity
annotations, and secret integration as required by the target cluster. Review
the rendered result before applying it. The validator checks the checked-in
contract offline, then runs rendered-manifest checks when `kubectl` is
available; skipped rendering is reported explicitly:

```bash
kubectl apply --dry-run=client -k deploy/kubernetes
kubectl diff -k deploy/kubernetes
./deploy/kubernetes/validate.sh
```

Run the focused manifest test, which works without Docker or a cluster:

```bash
./deploy/kubernetes/test.sh
```

With a disposable cluster, verify:

```bash
kubectl get pods -n <namespace>
kubectl describe pod -n <namespace> <pod-name>
kubectl port-forward -n <namespace> svc/<service-name> 8080:8080
curl --fail http://127.0.0.1:8080/healthz
```

Also verify the pod security context reports UID `65532`, the state directory
is writable by that UID, probes behave as documented, and `SIGTERM` produces a
bounded graceful shutdown. Do not apply these commands to an existing cluster
until the namespace and rendered resources have been explicitly identified.

For the complete local Docker and operational checklist, see
[`docs/docker-kubernetes.md`](../../docs/docker-kubernetes.md).
