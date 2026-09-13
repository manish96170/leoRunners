# Final Audit

`validate.sh` is a read-only Phase 54 architecture and production-readiness
audit. It does not initialize Terraform, call cloud APIs, build images, apply
Kubernetes resources, create credentials, or mutate the repository.

The audit checks required documentation, phase tracking, security boundaries,
no-live defaults, offline test/validator entrypoints, and recorded environment
gaps. It emits only bounded messages and a versioned report with `PASS`,
`WARN`, or `BLOCKED` status. `WARN` is reserved for environment-gated work;
missing contracts or contradictory tracking are `BLOCKED`.

```sh
./tools/final-audit/validate.sh --report /tmp/leo-final-audit.json
./tools/final-audit/test.sh
```

Reports are written atomically with owner-only `0600` permissions. The report
contains no raw command output, credentials, account identifiers, provider
responses, or local paths. Exit codes are `0` for `PASS`, `2` for `WARN`, and
`1` for `BLOCKED`.
