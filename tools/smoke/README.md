# Offline Smoke Harness

This harness validates the local controller's minimum HTTP contract without
contacting AWS, GCP, GitHub, a model endpoint, or any other external service.
It builds and starts `controller/cmd/runner-controller` with a scrubbed
environment. Because no cloud configuration is present, the controller uses
its fake provider.

## Checks

The harness asserts:

- `GET /healthz` returns `200` and `ok` is reachable.
- A webhook without a signature returns `401`.
- A correctly signed malformed payload returns `400`.
- A correctly signed `workflow_job` queued fixture returns `202`.
- The controller process is terminated with `SIGTERM`; a bounded fallback
  prevents a hung test process, and temporary state/log files are removed.

The queued event is accepted asynchronously. This test deliberately verifies
intake and validation, not a real runner registration or cloud launch.

## Usage

From the repository root:

```sh
tools/smoke/smoke.sh
```

Run shell validation plus the full smoke test:

```sh
tools/smoke/test.sh
```

Set `SMOKE_PORT` when the automatically selected local port is unavailable:

```sh
SMOKE_PORT=19080 tools/smoke/smoke.sh
```

The harness has no external mode. Real AWS/GCP/GitHub validation belongs to a
separate explicitly authorized integration procedure.
