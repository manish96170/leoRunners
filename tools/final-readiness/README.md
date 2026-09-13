# Final Readiness Gate

`validate.sh` runs the platform's offline validators and emits a redacted,
owner-only readiness report when `FINAL_READINESS_REPORT` is set. It never
provisions cloud resources or applies Terraform. A blocked result means an
environment-gated check still needs Docker, kubectl, Packer, credentials, or
an initialized provider plugin.

```sh
tools/final-readiness/test.sh
FINAL_READINESS_REPORT=/tmp/leo-readiness.json tools/final-readiness/validate.sh
```

The report is not approval for a live run. Use the approved scope file,
confirmation gate, and controlled-validation evidence workflow separately.
