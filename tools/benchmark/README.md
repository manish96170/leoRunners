# Phase 5 startup benchmark

This harness measures the ephemeral runner path as seven ordered phases:

`queue -> launch -> boot -> registration -> ready -> job_start -> cleanup`

Each phase is recorded as an absolute UTC timestamp and a duration from the
previous milestone. JSON preserves timestamps and Go durations; CSV emits
stable `<phase>_ms` columns for spreadsheet or plotting workflows.

## Local replay (default)

The command makes no AWS, GitHub, network, or credential calls:

```sh
go run . --mode fake --count 3 --format csv --output benchmark.csv
go run . --mode fake --format json
go test ./...
```

`FakeTimeline.Replay` is deterministic and is the contract test for consumers
that need repeatable startup data.

## Real AWS runs (explicit opt-in)

Real collection must be driven by the controller/job integration so that each
mark reflects an observed event, rather than a guessed sleep. Review IAM,
cleanup, budget limits, and the test repository before enabling it. The guard
below intentionally refuses accidental cloud calls:

```sh
go run . --mode real-aws --allow-real-aws
```

The command currently acts as a guarded integration hook. A real collector
should create `NewCollector(start)`, call `Mark` at queue, launch, boot,
registration, ready, job-start, and cleanup callbacks, then pass the result to
`WriteCSV` or `WriteJSON`. Set `AWS_PROFILE`, `AWS_REGION`, and a disposable
runner configuration in the controller separately; this harness never embeds
credentials or silently provisions infrastructure.
