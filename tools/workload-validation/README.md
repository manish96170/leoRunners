# Phase 6 workload validation

This harness produces repeatable evidence for comparing runner images. It
records the platform startup path and workload stages in one JSON or Markdown
report, bound to the reviewed workload manifest, commit, and command digest.
Logs captured from command mode are redacted before being written and reports
attest that raw payloads were excluded.

## Offline fake mode

Fake mode is the default and makes no repository, network, AWS, or GitHub calls:

```sh
go run . --mode fake --image base-linux-x64 --format markdown --output report.md
go test ./...
```

Its timestamps and durations are deterministic so reports can be used as a
schema and pipeline smoke test. They are marked `provenance: synthetic` and
must not be presented as real performance measurements.

## Explicit command mode

Command mode executes the supplied command in an existing local checkout. It
is opt-in, uses `sh -c`, captures combined stdout/stderr, records the exit code,
status, timeout, and wall duration, and exits 2 when the workload fails:

```sh
go run . --mode command \
  --repository /path/to/checkout \
  --workflow .github/workflows/ci.yml \
  --image node-linux-x64 \
  --command 'npm ci && npm run build && npm test' \
  --format markdown --output node-report.md
```

The command is intentionally not inferred from a GitHub workflow and the
harness does not provision cloud resources. Review the command and checkout
before running it; do not put credentials in arguments or output. A command
can still print unknown secret formats, so reports must be handled as
sensitive evidence even after redaction. Command reports are marked
`provenance: real`; pass the fixed workload identity, manifest digest, and
commit when collecting controlled evidence.

## Comparison fields

Use the same repository, workflow, commit, command digest, manifest digest,
cache mode, toolchain digest, resource profile, and measurement policy for each
image. Compare `platform_timings`, `workload_timings`, `duration`, `status`,
and `exit_code`. Never compare synthetic evidence with real evidence.
