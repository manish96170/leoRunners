# AMI Startup Benchmark

This tool measures the immutable runner image startup timeline using stable
checkpoints:

`launch_requested`, `instance_running`, `bootstrap_started`,
`bootstrap_completed`, `runner_registered`, `runner_ready`, and `job_started`.

## Offline deterministic mode

The default mode performs no AWS, Packer, network, or wall-clock measurement:

```sh
GO_BIN=/tmp/leo-go1.27.1/bin/go ./tools/ami-benchmark/test.sh
GO_BIN=/tmp/leo-go1.27.1/bin/go go run ./tools/ami-benchmark -count 20 \
  -output /tmp/ami-startup-report.json
```

It calculates nearest-rank p50, p95, and p99 values for every startup segment,
applies timeout and optional baseline-regression gates, and writes reports with
owner-only permissions using an atomic rename. The report links `image_id`,
`source_ami`, profile version, and the SHA-256 AMI artifact digest from the
contract and manifest. Account IDs, instance IDs, addresses, user data, JIT
configuration, and credentials are excluded.

Use `-baseline PATH` to compare p95 total startup against a prior report. A
failed gate exits non-zero and the report still records the failed gate.

## Real EC2 procedure

Real measurement is deliberately separate from the fake harness. The checked-in
guard does not invoke AWS:

```sh
./tools/ami-benchmark/real-ec2.sh I_UNDERSTAND_EPHEMERAL_EC2
```

After approval, an operator must launch exactly the approved AMI in an isolated
account/subnet, record the seven checkpoints from the controller/bootstrap
logs, verify cleanup, and produce a redacted report using the same contract and
manifest. Do not place user data, JIT configuration, instance IDs, account IDs,
or IP addresses in the report. The AMI digest and profile version are required
for comparability. No Packer build is part of this measurement.

## Gates

The default fake thresholds are p99 total <= 30,000 ms, p95 <= 26,000 ms, and
p99 <= 30,000 ms. Override them for an approved profile; keep timeout and
regression thresholds in the report. A baseline regression is measured as
`(current_p95 - baseline_p95) / baseline_p95 * 100` and must not exceed the
configured allowance.
