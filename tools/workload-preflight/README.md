# Workload preflight checker

This standalone Go tool checks whether a runner can execute a workload before
provisioning it. It reads a JSON manifest and emits one structured result per
requirement: `pass`, `warn`, or `fail`.

It checks the requested architecture, free disk and available memory, command
availability and minimum versions, Docker CLI plus local daemon availability,
and browser binaries. Optional requirements use `"required": false` and
produce warnings when unavailable. A failed check exits with status `1`;
manifest or invocation errors exit with status `2`.

## Usage

```sh
go run . --manifest fixtures/minimal.json --root /tmp
go test ./...
```

`--root` is used as the disk volume to inspect and as the working directory for
version probes. The checker only reads local system state and executes the
listed version probes. It never installs packages, starts containers, contacts
cloud services, or contacts registries by default. Docker probing uses
`docker info` only to test the local daemon.

## Manifest

```json
{
  "name": "node-browser-workload",
  "architecture": "amd64",
  "min_disk_bytes": 10737418240,
  "min_memory_bytes": 4294967296,
  "commands": [{"name": "git", "min_version": "2.40"}],
  "docker": {"required": true, "min_version": "24.0"},
  "browsers": [{"name": "chromium", "required": false}]
}
```

Command and browser entries default to required. `version_args` can override
the default `--version` probe for tools with a different interface. Version
comparison uses numeric components found in the probe output, so common
prefixes such as `v` and `Docker version` are supported.

## JSON output

The top-level object includes timestamps, the resolved root, summary counts,
and a `checks` array. Each check contains an id, category, status, message,
and, where meaningful, required and observed values. Output is deterministic
in shape and check order follows the manifest order.
