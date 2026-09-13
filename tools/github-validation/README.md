# GitHub JIT production-scope validation

This offline gate validates the Phase 48 contract for ephemeral GitHub Actions
runners. It does not use GitHub credentials, `gh`, `curl`, or live endpoints.

```sh
./tools/github-validation/test.sh
```

The gate runs the repository's `httptest`-based GitHub client tests, runner
assignment fakes, security policy tests, and bootstrap shell tests. It also
checks the source-level boundaries for:

- mandatory JIT labels and runner-group identity;
- bounded retry and context cancellation;
- exact online runner ID/name verification;
- webhook delivery identity and workflow-job event normalization;
- organization/repository and runner-label admission boundaries;
- fork denial by default and explicit fork-isolation coverage;
- runner-group/label propagation into JIT requests;
- single-use assignment protection for concurrent duplicate jobs;
- registration cleanup after a runner may have registered before failure;
- absence of the encoded JIT value from durable registration events;
- one-time bootstrap input and cleanup on success, failure, and signals;
- no persistent registration or credential handling in bootstrap; and
- fail-closed fork/untrusted capability policy through the existing security
  contract.

No validator output contains a JIT payload or credential.

The scope fixtures under `fixtures/` are intentionally declarative and
credential-free: `scope.safe` is admitted, while `scope.fork`,
`scope.repository`, and `scope.label` are denied. They document the expected
production boundary without making any GitHub request.
