# Performance

No performance claims are made yet because the workspace has no real workflow or cloud measurements. The first benchmark records queue time, webhook handling, scheduling, provisioning, boot, JIT registration, runner-ready time, job start, checkout, dependency/cache operations, build, tests, artifacts, cleanup, total duration, and cost.

The likely first bottleneck is cold-start and runner registration, not Go controller throughput. Use immutable images and measure before introducing warm pools, Spot, EC2 Fleet, prefetching, or predictive caching.

Phase 1 should provide deterministic fake-provider timing tests and controller metrics. Phase 5 adds real AWS timing and AMI comparison.
