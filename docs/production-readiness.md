# Production Readiness

Phase 11 packages the controller as a non-root container and validates the Go
module in CI. The image is a deployment artifact, not a replacement for the
operational controls below.

## Image and runtime

- Build only from the repository root with the checked-in `Dockerfile`.
- Use the multi-stage build output, and deploy the final runtime stage only.
- Run as UID/GID `65532`; do not override it to root.
- Mount durable storage at `/var/lib/leo-runners` when `STATE_PATH` uses the
  image default. The state file must be on encrypted, access-controlled storage.
- Keep the container port reachable only through the intended private network
  path. The controller does not need inbound SSH access.
- Pin the image digest in deployment configuration and scan the resulting image
  before promotion.
- The checked-in Kubernetes deployment at `deploy/kubernetes/controller.yaml`
  runs one hardened replica with a state PVC, no Kubernetes RBAC, and a
  cluster-internal webhook Service. See `deploy/kubernetes/README.md` before
  applying it.

## Configuration

Configuration is supplied at runtime through an approved secret/configuration
manager, never through the Dockerfile, build arguments, source tree, or image
layers. At minimum, review:

- `LISTEN_ADDR`
- `STATE_PATH`
- `RECONCILE_INTERVAL`
- `GITHUB_WEBHOOK_SECRET`
- `GITHUB_APP_INSTALLATION_TOKEN`
- `GITHUB_RUNNER_GROUP_ID`
- AWS or GCP provider settings and capacity-pool settings

Use explicit production values for region, launch template or instance
template, network, security profile, state location, lifecycle timeouts, and
capacity limits. Fail closed when a required provider or GitHub setting is
missing. Do not place tokens in command-line arguments, logs, labels, tags,
state records, benchmark output, or telemetry.

## Secrets

Use a workload identity or instance/task role for cloud API access. GitHub
credentials must be short-lived and scoped to the installation and repository
operations required by the controller. Store webhook secrets and token sources
in the deployment platform's secret manager, rotate them without rebuilding
the image, and audit access to them.

The JIT configuration is single-use sensitive data. Deliver it to the runner
bootstrap through the protected one-time file or file-descriptor contract;
never persist it in durable state or generic provider metadata. Verify that
termination and failed provisioning clean up the instance and any temporary
secret material.

## Health and rollout

`GET /healthz` is the liveness endpoint and must return HTTP 200 only while the
process is serving requests. Configure a startup grace period long enough for
the image to start, and configure a readiness check that also verifies the
controller has usable durable state, provider configuration, and required
GitHub configuration before accepting workload traffic.

Expose health checks through the private service path. Do not use the webhook
endpoint as a health probe. Alert on failed probes, reconciliation errors,
provider API errors, webhook authentication failures, state-write failures,
stale leases, and unexpected runner age.

Roll out one instance first, exercise a signed synthetic webhook, verify
idempotent delivery handling and cleanup, then expand capacity. A shared
durable store and coordinated lease/worker model are required before running
multiple controller replicas.

Kubernetes file-backed state is single-replica compatibility only. Do not
increase the deployment replica count until the controller uses shared durable
state and coordinated leases. Configure cloud workload identity externally;
the controller manifest intentionally does not include static cloud credentials
or Kubernetes API permissions.

## Graceful shutdown

The runtime sends `SIGTERM` and allows a termination grace period. Production
readiness requires the controller to stop accepting new work, finish or safely
cancel in-flight orchestration within a bounded deadline, preserve durable
state, and leave reaper/reconciliation work recoverable after restart. The
deployment must not use a grace period shorter than the configured provider
operation timeout.

Before promotion, verify signal handling with an in-flight provisioning test,
restart recovery, and a subsequent reconciliation pass. A container exit that
leaves an untracked instance, lease, or secret is a release blocker.

## CI gates

The workflow runs unit tests, race detection, `go vet`, and a complete Go build
on every pull request and push to `main`. The build job also builds the image
and verifies that its configured runtime user is non-root. Promotion additionally
requires dependency and image scanning, secret scanning, artifact provenance,
and the controlled AWS/GitHub or GCP/GitHub validation described in the phase
plans.
