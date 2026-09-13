# Final Readiness Pack

Phase 45 is a read-only release audit over the Phase 36–44 validators. Run it
from the repository root:

```sh
./tools/final-readiness/validate.sh --report /tmp/leo-final-readiness.json
./tools/final-readiness/test.sh
```

The report is versioned as `final-readiness.leorunners.io/v1` and uses three
categories:

- `PASS`: the local validator completed successfully.
- `WARN`: a read-only check was unavailable or an optional environment was not initialized.
- `BLOCKED`: a validator failed, a required offline contract was unavailable, or a mutation-looking command was observed.

The aggregator never supplies live, apply, build, init, destroy, or confirmation
arguments. It does not invoke Docker image builds, Packer, Terraform provider
initialization, Kubernetes apply, or cloud provisioning. The cloud preflight
may perform read-only AWS, GCP, and GitHub identity/API checks when the required
environment is present and `FINAL_READINESS_RUN_CLOUD_PREFLIGHT=true` is set.
The report explicitly inventories those gates and the
Docker, `kubectl`, Packer, and Terraform-provider gates that require an approved
operator run.

Reports contain bounded redacted messages, exclude raw validator output, are
written atomically with owner-only permissions, and assert
`live_mutation_invoked: false`. A `BLOCKED` result requires remediation or an
approved environment-gated check before release sign-off.
